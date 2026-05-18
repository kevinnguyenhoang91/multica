package db

import (
	"strings"
	"testing"
)

// TestAdvanceIssueSQL_ContainsActiveTaskGuard asserts that the generated SQL
// for AdvanceIssueToInReviewOnTaskCompletion contains the NOT EXISTS guard
// against active tasks. This white-box test catches regressions where the guard
// is silently removed from the query — which would cause issues to advance to
// in_review even when agent tasks are still running.
func TestAdvanceIssueSQL_ContainsActiveTaskGuard(t *testing.T) {
	sql := advanceIssueToInReviewOnTaskCompletion

	if !strings.Contains(sql, "NOT EXISTS") {
		t.Error("SQL must contain NOT EXISTS active-task gate")
	}
	if !strings.Contains(sql, "agent_task_queue") {
		t.Error("SQL must reference agent_task_queue in the active-task gate")
	}
	for _, status := range []string{"'queued'", "'dispatched'", "'running'"} {
		if !strings.Contains(sql, status) {
			t.Errorf("SQL active-task gate must check status %s", status)
		}
	}
	if !strings.Contains(sql, "assignee_type IN") {
		t.Error("SQL must contain assignee_type IN guardrail")
	}
	if !strings.Contains(sql, "'agent'") || !strings.Contains(sql, "'squad'") {
		t.Error("SQL assignee guardrail must include 'agent' and 'squad'")
	}
	if !strings.Contains(sql, "status = 'in_progress'") {
		t.Error("SQL must require status = 'in_progress' (terminal-state protection)")
	}
}
