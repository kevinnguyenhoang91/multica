import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

// Helper: stub fetch with a single JSON response. Status defaults to 200.
function stubFetchJson(body: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(typeof body === "string" ? body : JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

// These tests cover the five failure modes that white-screened the desktop
// app in past incidents. The contract is: a malformed response degrades to
// an empty/safe shape, never throws into React.
describe("ApiClient schema fallback", () => {
  describe("listTimeline", () => {
    it("falls back to an empty page when required fields are missing", async () => {
      stubFetchJson({});
      const client = new ApiClient("https://api.example.test");
      const page = await client.listTimeline("issue-1");
      expect(page).toEqual({
        entries: [],
        next_cursor: null,
        prev_cursor: null,
        has_more_before: false,
        has_more_after: false,
      });
    });

    it("falls back when a field has the wrong type", async () => {
      stubFetchJson({
        entries: "not-an-array",
        next_cursor: null,
        prev_cursor: null,
        has_more_before: false,
        has_more_after: false,
      });
      const client = new ApiClient("https://api.example.test");
      const page = await client.listTimeline("issue-1");
      expect(page.entries).toEqual([]);
      expect(page.has_more_after).toBe(false);
    });

    it("accepts a new entry type rather than crashing on enum drift", async () => {
      stubFetchJson({
        entries: [
          {
            type: "future_kind", // not in TS union
            id: "e-1",
            actor_type: "member",
            actor_id: "u-1",
            created_at: "2026-01-01T00:00:00Z",
          },
        ],
        next_cursor: null,
        prev_cursor: null,
        has_more_before: false,
        has_more_after: false,
      });
      const client = new ApiClient("https://api.example.test");
      const page = await client.listTimeline("issue-1");
      expect(page.entries).toHaveLength(1);
      expect(page.entries[0]?.type).toBe("future_kind");
    });

    it("returns an empty page when the body is null", async () => {
      stubFetchJson(null);
      const client = new ApiClient("https://api.example.test");
      const page = await client.listTimeline("issue-1");
      expect(page.entries).toEqual([]);
    });

    it("treats null arrays as empty arrays", async () => {
      stubFetchJson({
        entries: null,
        next_cursor: null,
        prev_cursor: null,
        has_more_before: false,
        has_more_after: false,
      });
      const client = new ApiClient("https://api.example.test");
      const page = await client.listTimeline("issue-1");
      expect(page.entries).toEqual([]);
    });
  });

  describe("listIssues", () => {
    it("falls back to an empty list when the response is malformed", async () => {
      stubFetchJson({ unexpected: true });
      const client = new ApiClient("https://api.example.test");
      const res = await client.listIssues();
      expect(res).toEqual({ issues: [], total: 0 });
    });
  });

  describe("listComments", () => {
    it("returns [] when the response is not an array", async () => {
      stubFetchJson({ wrong: "shape" });
      const client = new ApiClient("https://api.example.test");
      const comments = await client.listComments("issue-1");
      expect(comments).toEqual([]);
    });
  });

  describe("listIssueSubscribers", () => {
    it("returns [] when the response is null", async () => {
      stubFetchJson(null);
      const client = new ApiClient("https://api.example.test");
      const subs = await client.listIssueSubscribers("issue-1");
      expect(subs).toEqual([]);
    });
  });

  describe("listChildIssues", () => {
    it("returns { issues: [] } when the issues field is missing", async () => {
      stubFetchJson({});
      const client = new ApiClient("https://api.example.test");
      const res = await client.listChildIssues("issue-1");
      expect(res).toEqual({ issues: [] });
    });
  });
});
