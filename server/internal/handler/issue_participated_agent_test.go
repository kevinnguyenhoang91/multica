package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func insertAgentComment(t *testing.T, issueID, agentID, body string) string {
	t.Helper()
	var commentID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO comment (workspace_id, issue_id, author_type, author_id, content, type)
		VALUES ($1, $2, 'agent', $3, $4, 'comment')
		RETURNING id
	`, testWorkspaceID, issueID, agentID, body).Scan(&commentID); err != nil {
		t.Fatalf("insert agent comment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM comment WHERE id = $1`, commentID)
	})
	return commentID
}

func listIssuesParticipatedByAgent(t *testing.T, agentID string) []string {
	t.Helper()
	path := fmt.Sprintf("/api/issues?workspace_id=%s&participated_agent_id=%s&limit=500",
		testWorkspaceID, agentID)
	w := httptest.NewRecorder()
	testHandler.ListIssues(w, newRequest("GET", path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListIssues: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Issues []IssueResponse `json:"issues"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	ids := make([]string, 0, len(resp.Issues))
	for _, iss := range resp.Issues {
		ids = append(ids, iss.ID)
	}
	return ids
}

func listGroupedIssuesParticipatedByAgent(t *testing.T, agentID string) []string {
	t.Helper()
	path := fmt.Sprintf(
		"/api/issues/grouped?workspace_id=%s&group_by=assignee&statuses=todo&participated_agent_id=%s&limit=100",
		testWorkspaceID, agentID,
	)
	w := httptest.NewRecorder()
	testHandler.ListGroupedIssues(w, newRequest("GET", path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListGroupedIssues: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp GroupedIssuesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode grouped response: %v", err)
	}
	ids := []string{}
	for _, g := range resp.Groups {
		for _, iss := range g.Issues {
			ids = append(ids, iss.ID)
		}
	}
	return ids
}

func TestListIssues_ParticipatedAgentID_MatchesOnlyThatAgentComments(t *testing.T) {
	ctx := context.Background()
	fx := setupInvolvesFixture(t)

	matchingIssueID := insertIssueTo(t, ctx, testWorkspaceID,
		"participated-agent should match this issue", "member", fx.userID)
	insertAgentComment(t, matchingIssueID, fx.ownedAgentID, "I participated here")

	otherIssueID := insertIssueTo(t, ctx, testWorkspaceID,
		"participated-agent should not match other agent comments", "member", fx.userID)
	insertAgentComment(t, otherIssueID, fx.otherAgentID, "different agent comment")

	got := listIssuesParticipatedByAgent(t, fx.ownedAgentID)
	if !containsIssueID(got, matchingIssueID) {
		t.Fatalf("expected matching issue %s in list, got %v", matchingIssueID, got)
	}
	if containsIssueID(got, otherIssueID) {
		t.Fatalf("unexpected issue from other agent comment surfaced: %s (full %v)", otherIssueID, got)
	}
}

func TestListGroupedIssues_ParticipatedAgentID_MatchesAgentComments(t *testing.T) {
	ctx := context.Background()
	fx := setupInvolvesFixture(t)

	matchingIssueID := insertIssueTo(t, ctx, testWorkspaceID,
		"grouped participated-agent should match this issue", "member", fx.userID)
	insertAgentComment(t, matchingIssueID, fx.ownedAgentID, "grouped path participation")

	got := listGroupedIssuesParticipatedByAgent(t, fx.ownedAgentID)
	if !containsIssueID(got, matchingIssueID) {
		t.Fatalf("expected matching issue %s in grouped list, got %v", matchingIssueID, got)
	}
}

func TestListIssues_ParticipatedAgentID_InvalidUUIDReturns400(t *testing.T) {
	path := fmt.Sprintf("/api/issues?workspace_id=%s&participated_agent_id=not-a-uuid", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.ListIssues(w, newRequest("GET", path, nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid UUID, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListGroupedIssues_ParticipatedAgentID_InvalidUUIDReturns400(t *testing.T) {
	path := fmt.Sprintf("/api/issues/grouped?workspace_id=%s&group_by=assignee&participated_agent_id=not-a-uuid", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.ListGroupedIssues(w, newRequest("GET", path, nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid UUID, got %d: %s", w.Code, w.Body.String())
	}
}
