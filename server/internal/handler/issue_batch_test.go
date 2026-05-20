package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestBatchUpdateNoMutationReturnsZero — regression for #1660.
//
// When the request payload has valid issue_ids but the "updates" field
// is empty, missing, or doesn't decode any known mutation field, the
// handler used to walk every issue, run a no-op UPDATE, and increment
// `updated` for each one — returning {"updated": N} despite changing
// nothing. Reporters saw 200 + a positive count and assumed the call
// worked, then chased a phantom persistence bug.
//
// The fix is "tell the truth": when no mutation field is present, return
// {"updated": 0} immediately so the count matches reality.
func TestBatchUpdateNoMutationReturnsZero(t *testing.T) {
	// Two fresh issues so we can also assert no fields actually changed.
	a := createTestIssue(t, "BU-no-mut A", "todo", "low")
	b := createTestIssue(t, "BU-no-mut B", "todo", "low")
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	cases := []struct {
		desc string
		body map[string]any
	}{
		{
			desc: "updates_missing",
			// Most common reporter pattern: status at top level.
			body: map[string]any{"issue_ids": []string{a, b}, "status": "in_progress"},
		},
		{
			desc: "updates_empty_object",
			body: map[string]any{"issue_ids": []string{a, b}, "updates": map[string]any{}},
		},
		{
			desc: "updates_misnamed",
			// Singular "update" instead of plural "updates".
			body: map[string]any{"issue_ids": []string{a, b}, "update": map[string]any{"status": "done"}},
		},
		{
			desc: "updates_unknown_field_only",
			// Payload IS nested correctly, but every key inside `updates` is
			// unknown to the handler — same class of caller mistake as the
			// shapes above. hasMutation must stay false; behavior is already
			// correct, this case locks it in against future regressions.
			body: map[string]any{"issue_ids": []string{a, b}, "updates": map[string]any{"foo": "bar"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequest("POST", "/api/issues/batch-update", tc.body)
			testHandler.BatchUpdateIssues(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Updated int `json:"updated"`
			}
			json.NewDecoder(w.Body).Decode(&resp)
			if resp.Updated != 0 {
				t.Errorf("expected updated=0 when no mutation field present, got %d", resp.Updated)
			}

			// Belt and braces: confirm the issues weren't touched.
			for _, id := range []string{a, b} {
				gw := httptest.NewRecorder()
				gr := newRequest("GET", "/api/issues/"+id, nil)
				gr = withURLParam(gr, "id", id)
				testHandler.GetIssue(gw, gr)
				var got IssueResponse
				json.NewDecoder(gw.Body).Decode(&got)
				if got.Status != "todo" {
					t.Errorf("issue %s: status changed to %q despite no-mutation request", id, got.Status)
				}
			}
		})
	}
}

// TestBatchUpdateValidUpdatesPersistAndCount — positive case to lock in
// happy-path behavior alongside the regression test above.
func TestBatchUpdateValidUpdatesPersistAndCount(t *testing.T) {
	a := createTestIssue(t, "BU-ok A", "todo", "low")
	b := createTestIssue(t, "BU-ok B", "todo", "low")
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{a, b},
		"updates":   map[string]any{"status": "in_progress"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Updated != 2 {
		t.Errorf("expected updated=2, got %d", resp.Updated)
	}
	for _, id := range []string{a, b} {
		gw := httptest.NewRecorder()
		gr := newRequest("GET", "/api/issues/"+id, nil)
		gr = withURLParam(gr, "id", id)
		testHandler.GetIssue(gw, gr)
		var got IssueResponse
		json.NewDecoder(gw.Body).Decode(&got)
		if got.Status != "in_progress" {
			t.Errorf("issue %s: expected status=in_progress, got %q", id, got.Status)
		}
	}
}

func TestBatchUpdateInReviewTransitionReassignsToCreator(t *testing.T) {
	ctx := context.Background()
	var agentID string
	err := testPool.QueryRow(ctx,
		`SELECT id FROM agent WHERE workspace_id = $1 AND name = $2`,
		testWorkspaceID, "Handler Test Agent",
	).Scan(&agentID)
	if err != nil {
		t.Fatalf("failed to find test agent: %v", err)
	}

	a := createTestIssueWithAssignee(t, "BU-in-review A", "in_progress", "low", "agent", agentID)
	b := createTestIssueWithAssignee(t, "BU-in-review B", "in_progress", "low", "agent", agentID)
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{a, b},
		"updates":   map[string]any{"status": "in_review"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Updated != 2 {
		t.Fatalf("expected updated=2, got %d", resp.Updated)
	}

	for _, id := range []string{a, b} {
		gw := httptest.NewRecorder()
		gr := newRequest("GET", "/api/issues/"+id, nil)
		gr = withURLParam(gr, "id", id)
		testHandler.GetIssue(gw, gr)
		if gw.Code != http.StatusOK {
			t.Fatalf("GetIssue(%s): expected 200, got %d: %s", id, gw.Code, gw.Body.String())
		}
		var got IssueResponse
		json.NewDecoder(gw.Body).Decode(&got)
		if got.Status != "in_review" {
			t.Fatalf("issue %s: expected status=in_review, got %q", id, got.Status)
		}
		if got.AssigneeType == nil || *got.AssigneeType != "member" {
			t.Fatalf("issue %s: expected assignee_type=member (creator), got %v", id, got.AssigneeType)
		}
		if got.AssigneeID == nil || *got.AssigneeID != testUserID {
			t.Fatalf("issue %s: expected assignee_id=%s (creator), got %v", id, testUserID, got.AssigneeID)
		}
	}
}

func TestBatchUpdateInReviewTransitionKeepsHumanAssignee(t *testing.T) {
	ctx := context.Background()
	var memberID string
	if err := testPool.QueryRow(ctx, `
			INSERT INTO "user" (name, email)
			VALUES ($1, $2)
			RETURNING id
		`, "Batch Test Member", fmt.Sprintf("batch-test-member-%d@multica.ai", time.Now().UnixNano())).Scan(&memberID); err != nil {
		t.Fatalf("failed to create workspace member user: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
			INSERT INTO member (workspace_id, user_id, role)
			VALUES ($1, $2, 'member')
		`, testWorkspaceID, memberID); err != nil {
		t.Fatalf("failed to add workspace member: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, memberID)
	})

	issueID := createTestIssueWithAssignee(t, "BU-keep-human-assignee", "in_progress", "low", "member", memberID)
	t.Cleanup(func() { deleteTestIssue(t, issueID) })

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID},
		"updates":   map[string]any{"status": "in_review"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	gw := httptest.NewRecorder()
	gr := newRequest("GET", "/api/issues/"+issueID, nil)
	gr = withURLParam(gr, "id", issueID)
	testHandler.GetIssue(gw, gr)
	if gw.Code != http.StatusOK {
		t.Fatalf("GetIssue(%s): expected 200, got %d: %s", issueID, gw.Code, gw.Body.String())
	}
	var got IssueResponse
	json.NewDecoder(gw.Body).Decode(&got)
	if got.Status != "in_review" {
		t.Fatalf("issue %s: expected status=in_review, got %q", issueID, got.Status)
	}
	if got.AssigneeType == nil || *got.AssigneeType != "member" {
		t.Fatalf("issue %s: expected assignee_type=member, got %v", issueID, got.AssigneeType)
	}
	if got.AssigneeID == nil || *got.AssigneeID != memberID {
		t.Fatalf("issue %s: expected assignee_id=%s (unchanged), got %v", issueID, memberID, got.AssigneeID)
	}
}

func TestBatchUpdateToInReviewFromTodoKeepsAgentAssignee(t *testing.T) {
	ctx := context.Background()
	var agentID string
	err := testPool.QueryRow(ctx,
		`SELECT id FROM agent WHERE workspace_id = $1 AND name = $2`,
		testWorkspaceID, "Handler Test Agent",
	).Scan(&agentID)
	if err != nil {
		t.Fatalf("failed to find test agent: %v", err)
	}

	issueID := createTestIssueWithAssignee(t, "BU-no-handoff-from-todo", "todo", "low", "agent", agentID)
	t.Cleanup(func() { deleteTestIssue(t, issueID) })

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID},
		"updates":   map[string]any{"status": "in_review"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	gw := httptest.NewRecorder()
	gr := newRequest("GET", "/api/issues/"+issueID, nil)
	gr = withURLParam(gr, "id", issueID)
	testHandler.GetIssue(gw, gr)
	if gw.Code != http.StatusOK {
		t.Fatalf("GetIssue(%s): expected 200, got %d: %s", issueID, gw.Code, gw.Body.String())
	}
	var got IssueResponse
	json.NewDecoder(gw.Body).Decode(&got)
	if got.Status != "in_review" {
		t.Fatalf("issue %s: expected status=in_review, got %q", issueID, got.Status)
	}
	if got.AssigneeType == nil || *got.AssigneeType != "agent" {
		t.Fatalf("issue %s: expected assignee_type=agent, got %v", issueID, got.AssigneeType)
	}
	if got.AssigneeID == nil || *got.AssigneeID != agentID {
		t.Fatalf("issue %s: expected assignee_id=%s (unchanged), got %v", issueID, agentID, got.AssigneeID)
	}
}

// createTestIssue is a small helper to keep the table-driven cases clean.
// Returns the new issue's id; caller is responsible for cleanup.
func createTestIssue(t *testing.T, title, status, priority string) string {
	return createTestIssueWithAssignee(t, title, status, priority, "", "")
}

func createTestIssueWithAssignee(t *testing.T, title, status, priority, assigneeType, assigneeID string) string {
	t.Helper()
	body := map[string]any{
		"title":    title,
		"status":   status,
		"priority": priority,
	}
	if assigneeType != "" {
		body["assignee_type"] = assigneeType
	}
	if assigneeID != "" {
		body["assignee_id"] = assigneeID
	}
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body)
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue %q: expected 201, got %d: %s", title, w.Code, w.Body.String())
	}
	var issue IssueResponse
	json.NewDecoder(w.Body).Decode(&issue)
	return issue.ID
}

func deleteTestIssue(t *testing.T, id string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/issues/"+id, nil)
	req = withURLParam(req, "id", id)
	testHandler.DeleteIssue(w, req)
}
