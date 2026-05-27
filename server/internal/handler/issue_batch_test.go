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

// TestBatchUpdateInReviewTransitionSkipsArchivedCreatorAgentAssignee is the
// batch-path regression test for KHA-53: a status-only batch update to
// in_review should skip (updated=0) issues whose creator is an archived agent,
// because the handoff would derive an invalid assignee that bypasses validation.
func TestBatchUpdateInReviewTransitionSkipsArchivedCreatorAgentAssignee(t *testing.T) {
	ctx := context.Background()

	// Create a fresh agent to act as creator (will be archived before the update).
	creatorAgent := createHandlerTestAgent(t, "KHA53 Batch Creator Agent", []byte("[]"))

	a := createTestIssue(t, "BU-kha53-archived-creator A", "in_progress", "low")
	b := createTestIssue(t, "BU-kha53-archived-creator B", "in_progress", "low")
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	// Override creator for both issues to the fresh agent.
	for _, id := range []string{a, b} {
		if _, err := testPool.Exec(ctx,
			`UPDATE issue SET creator_type = 'agent', creator_id = $1 WHERE id = $2`,
			creatorAgent, id,
		); err != nil {
			t.Fatalf("override creator for issue %s: %v", id, err)
		}
	}

	// Archive the creator agent.
	if _, err := testPool.Exec(ctx,
		`UPDATE agent SET archived_at = now(), archived_by = $1 WHERE id = $2`,
		testUserID, creatorAgent,
	); err != nil {
		t.Fatalf("archive creator agent: %v", err)
	}

	// Status-only batch in_review update — both issues should be skipped
	// because the handoff would derive the archived agent as their assignee.
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{a, b},
		"updates":   map[string]any{"status": "in_review"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BatchUpdateIssues: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Updated != 0 {
		t.Fatalf("expected updated=0 (all skipped), got %d", resp.Updated)
	}

	// Confirm statuses were not changed.
	for _, id := range []string{a, b} {
		gw := httptest.NewRecorder()
		gr := newRequest("GET", "/api/issues/"+id, nil)
		gr = withURLParam(gr, "id", id)
		testHandler.GetIssue(gw, gr)
		var got IssueResponse
		json.NewDecoder(gw.Body).Decode(&got)
		if got.Status != "in_progress" {
			t.Fatalf("issue %s: expected status=in_progress (unchanged), got %q", id, got.Status)
		}
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
