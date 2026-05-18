package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// issueRow implements pgx.Row and returns a scanned db.Issue on success or an
// error when the caller wants to simulate ErrNoRows (guardrail blocked).
type issueRow struct {
	issue *db.Issue
	err   error
}

func (r *issueRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	i := r.issue
	ptrs := []any{
		&i.ID, &i.WorkspaceID, &i.Title, &i.Description,
		&i.Status, &i.Priority, &i.AssigneeType, &i.AssigneeID,
		&i.CreatorType, &i.CreatorID, &i.ParentIssueID,
		&i.AcceptanceCriteria, &i.ContextRefs, &i.Position,
		&i.DueDate, &i.CreatedAt, &i.UpdatedAt, &i.Number,
		&i.ProjectID, &i.OriginType, &i.OriginID, &i.FirstExecutedAt,
	}
	for idx, p := range ptrs {
		if idx >= len(dest) {
			break
		}
		switch d := dest[idx].(type) {
		case *pgtype.UUID:
			*d = *(p.(*pgtype.UUID))
		case *string:
			*d = *(p.(*string))
		case *pgtype.Text:
			*d = *(p.(*pgtype.Text))
		case *pgtype.Timestamptz:
			*d = *(p.(*pgtype.Timestamptz))
		case *[]byte:
			*d = *(p.(*[]byte))
		case *float64:
			*d = *(p.(*float64))
		case *int32:
			*d = *(p.(*int32))
		}
	}
	return nil
}

// trackingDBTX records whether AdvanceIssueToInReviewOnTaskCompletion was
// called and controls its return value: advanceReturn != nil → return issue;
// nil → return ErrNoRows (simulating any guardrail blocking the transition).
type trackingDBTX struct {
	task          db.AgentTaskQueue
	advanceCalled bool
	advanceReturn *db.Issue // nil means ErrNoRows (no-op)
}

func (m *trackingDBTX) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (m *trackingDBTX) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}

func (m *trackingDBTX) QueryRow(_ context.Context, sql string, _ ...interface{}) pgx.Row {
	// AdvanceIssueToInReviewOnTaskCompletion is uniquely identified by the
	// literal 'in_review' in the SET clause of the UPDATE.
	if strings.Contains(sql, "SET status = 'in_review'") {
		m.advanceCalled = true
		if m.advanceReturn != nil {
			return &issueRow{issue: m.advanceReturn}
		}
		return &issueRow{err: pgx.ErrNoRows}
	}
	// CompleteAgentTask and similar UPDATEs contain "SET status ="
	if strings.Contains(sql, "SET status =") {
		return &mockRow{err: pgx.ErrNoRows}
	}
	// getIssuePrefix calls GetWorkspace — return ErrNoRows so prefix is "".
	if strings.Contains(sql, "FROM workspace") {
		return &issueRow{err: pgx.ErrNoRows}
	}
	// GetAgentTask — return the stored task
	return &mockRow{task: &m.task}
}

func newTrackingSvc(task db.AgentTaskQueue, advanceReturn *db.Issue) (*TaskService, *trackingDBTX) {
	mock := &trackingDBTX{task: task, advanceReturn: advanceReturn}
	svc := &TaskService{
		Queries: db.New(mock),
		Bus:     events.New(),
	}
	return svc, mock
}

// collectIssueUpdatedEvents subscribes to EventIssueUpdated on the service bus
// and returns a pointer to a slice that accumulates received events.
func collectIssueUpdatedEvents(svc *TaskService) *[]events.Event {
	var received []events.Event
	svc.Bus.Subscribe(protocol.EventIssueUpdated, func(e events.Event) {
		received = append(received, e)
	})
	return &received
}

// TestCompleteTask_AutoMovesIssueToInReviewForAgentAndSquad verifies that when
// the DB returns a row (transition fired), an issue:updated event is published
// with status_changed=true.
func TestCompleteTask_AutoMovesIssueToInReviewForAgentAndSquad(t *testing.T) {
	issueID := testUUID(0xA0)
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)
	wsID := testUUID(0x10)

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: issueID,
		Status:  "completed",
	}
	returnedIssue := &db.Issue{
		ID:          issueID,
		WorkspaceID: wsID,
		Status:      "in_review",
	}

	svc, mock := newTrackingSvc(task, returnedIssue)
	received := collectIssueUpdatedEvents(svc)

	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if !mock.advanceCalled {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called, but it was not")
	}
	if len(*received) != 1 {
		t.Fatalf("expected 1 issue:updated event, got %d", len(*received))
	}
	payload, ok := (*received)[0].Payload.(map[string]any)
	if !ok {
		t.Fatal("event payload is not map[string]any")
	}
	if payload["status_changed"] != true {
		t.Errorf("expected status_changed=true, got %v", payload["status_changed"])
	}
	if payload["prev_status"] != "in_progress" {
		t.Errorf("expected prev_status=in_progress, got %v", payload["prev_status"])
	}
}

// TestCompleteTask_DoesNotMoveIssueToInReviewWhenActiveTasksRemain verifies
// that when the DB returns ErrNoRows (active-task gate blocked the transition),
// no issue:updated event is published. The SQL NOT EXISTS guard is the line of
// defence; this test ensures the service correctly treats ErrNoRows as a no-op.
func TestCompleteTask_DoesNotMoveIssueToInReviewWhenActiveTasksRemain(t *testing.T) {
	issueID := testUUID(0xB0)
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: issueID,
		Status:  "completed",
	}
	// advanceReturn=nil → DB returns ErrNoRows, simulating active-task gate.
	svc, mock := newTrackingSvc(task, nil)
	received := collectIssueUpdatedEvents(svc)

	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if !mock.advanceCalled {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called")
	}
	if len(*received) != 0 {
		t.Errorf("expected no issue:updated event when DB gate blocks transition, got %d", len(*received))
	}
}

// TestCompleteTask_DoesNotMoveIssueToInReviewForMemberAssignedIssue verifies
// that when the DB returns ErrNoRows (assignee guardrail blocked), no
// issue:updated event is published.
func TestCompleteTask_DoesNotMoveIssueToInReviewForMemberAssignedIssue(t *testing.T) {
	issueID := testUUID(0xC0)
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: issueID,
		Status:  "completed",
	}
	// advanceReturn=nil → DB returns ErrNoRows, simulating assignee guardrail.
	svc, mock := newTrackingSvc(task, nil)
	received := collectIssueUpdatedEvents(svc)

	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if !mock.advanceCalled {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called; assignee guardrail is enforced by SQL, not Go code")
	}
	if len(*received) != 0 {
		t.Errorf("expected no issue:updated event when assignee guard blocks, got %d", len(*received))
	}
}

// TestCompleteTask_DoesNotOverwriteTerminalIssueStatus verifies that when the
// DB returns ErrNoRows (terminal-state protection), no issue:updated event is
// published.
func TestCompleteTask_DoesNotOverwriteTerminalIssueStatus(t *testing.T) {
	terminalStatuses := []string{"done", "cancelled", "blocked", "in_review"}

	for _, status := range terminalStatuses {
		t.Run("issue_status_"+status, func(t *testing.T) {
			issueID := testUUID(0xD0)
			taskID := testUUID(0x01)
			agentID := testUUID(0x02)

			task := db.AgentTaskQueue{
				ID:      taskID,
				AgentID: agentID,
				IssueID: issueID,
				Status:  "completed",
			}
			// advanceReturn=nil → DB returns ErrNoRows (terminal-state guard).
			svc, mock := newTrackingSvc(task, nil)
			received := collectIssueUpdatedEvents(svc)

			svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

			if !mock.advanceCalled {
				t.Errorf("status=%s: expected SQL advance call; terminal-state guard is enforced by DB, not Go code", status)
			}
			if len(*received) != 0 {
				t.Errorf("status=%s: expected no event when terminal-state guard blocks, got %d", status, len(*received))
			}
		})
	}
}

// TestCompleteTask_NoIssueLink verifies that maybeAdvanceIssueToInReviewOnCompletion
// is a no-op for tasks not linked to any issue (chat tasks, quick-create tasks).
func TestCompleteTask_NoIssueLink(t *testing.T) {
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: pgtype.UUID{Valid: false}, // not linked to any issue
		Status:  "completed",
	}
	svc, mock := newTrackingSvc(task, nil)
	received := collectIssueUpdatedEvents(svc)

	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if mock.advanceCalled {
		t.Error("expected NO advance SQL for task without issue link, but the query was called")
	}
	if len(*received) != 0 {
		t.Errorf("expected no event for unlinked task, got %d", len(*received))
	}
}

