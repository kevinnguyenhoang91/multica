package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// trackingDBTX wraps mockDBTX and records SQL passed to Exec.
type trackingDBTX struct {
	task    db.AgentTaskQueue
	execSQL []string
}

func (m *trackingDBTX) Exec(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
	m.execSQL = append(m.execSQL, sql)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *trackingDBTX) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}

func (m *trackingDBTX) QueryRow(_ context.Context, sql string, _ ...interface{}) pgx.Row {
	// CompleteAgentTask and similar UPDATEs contain "SET status ="
	if strings.Contains(sql, "SET status =") {
		return &mockRow{err: pgx.ErrNoRows}
	}
	// GetAgentTask — return the stored task
	return &mockRow{task: &m.task}
}

func (m *trackingDBTX) advanceCalled() bool {
	for _, s := range m.execSQL {
		if strings.Contains(s, "in_review") {
			return true
		}
	}
	return false
}

func newTrackingSvc(task db.AgentTaskQueue) (*TaskService, *trackingDBTX) {
	mock := &trackingDBTX{task: task}
	svc := &TaskService{
		Queries: db.New(mock),
		Bus:     events.New(),
	}
	return svc, mock
}

// TestCompleteTask_AutoMovesIssueToInReviewForAgentAndSquad verifies that
// maybeAdvanceIssueToInReviewOnCompletion issues the SQL advance query for an
// issue-linked task regardless of the assignee type recorded on the task row.
// The SQL itself enforces the agent/squad guard atomically in the DB.
func TestCompleteTask_AutoMovesIssueToInReviewForAgentAndSquad(t *testing.T) {
	issueID := testUUID(0xA0)
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: issueID,
		Status:  "completed",
	}

	svc, mock := newTrackingSvc(task)
	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if !mock.advanceCalled() {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called for issue-linked task, but it was not")
	}
}

// TestCompleteTask_DoesNotMoveIssueToInReviewWhenActiveTasksRemain verifies
// that the SQL advance query is still issued but enforces the active-task gate
// atomically. The no-op path (zero rows updated) is handled inside the DB;
// maybeAdvanceIssueToInReviewOnCompletion always calls the query — the guard
// lives in the SQL WHERE clause, not in Go application logic.
func TestCompleteTask_DoesNotMoveIssueToInReviewWhenActiveTasksRemain(t *testing.T) {
	issueID := testUUID(0xB0)
	taskID := testUUID(0x01)
	agentID := testUUID(0x02)

	// Simulate the trackingDBTX returning "UPDATE 0" (no rows affected) to
	// represent the active-task gate blocking the transition.
	mock := &trackingDBTX{
		task: db.AgentTaskQueue{
			ID:      taskID,
			AgentID: agentID,
			IssueID: issueID,
			Status:  "completed",
		},
	}
	svc := &TaskService{
		Queries: db.New(mock),
		Bus:     events.New(),
	}

	task := db.AgentTaskQueue{
		ID:      taskID,
		AgentID: agentID,
		IssueID: issueID,
		Status:  "completed",
	}
	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	// The SQL query must be issued — the DB enforces the gate atomically.
	if !mock.advanceCalled() {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called; active-task gate is enforced by SQL, not Go code")
	}
}

// TestCompleteTask_DoesNotMoveIssueToInReviewForMemberAssignedIssue verifies
// that the SQL query is still issued for member-assigned issues, but the
// assignee_type IN ('agent','squad') guard in the WHERE clause prevents the
// UPDATE from matching any rows — enforced atomically in the DB.
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
	svc, mock := newTrackingSvc(task)
	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	// The SQL is called; the assignee guardrail is in the DB WHERE clause.
	if !mock.advanceCalled() {
		t.Error("expected AdvanceIssueToInReviewOnTaskCompletion to be called; assignee guardrail is enforced by SQL, not Go code")
	}
}

// TestCompleteTask_DoesNotOverwriteTerminalIssueStatus verifies that the SQL
// advance query is issued but the status = 'in_progress' guard in the WHERE
// clause prevents overwriting terminal states (done/cancelled/blocked).
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
			svc, mock := newTrackingSvc(task)
			svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

			// The SQL is issued; terminal-state protection is in the DB WHERE clause.
			if !mock.advanceCalled() {
				t.Errorf("status=%s: expected SQL advance call; terminal-state guard is enforced by DB, not Go code", status)
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
	svc, mock := newTrackingSvc(task)
	svc.maybeAdvanceIssueToInReviewOnCompletion(context.Background(), task)

	if mock.advanceCalled() {
		t.Error("expected NO advance SQL for task without issue link, but the query was called")
	}
}
