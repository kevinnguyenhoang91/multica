import { describe, expect, it } from "vitest";
import {
  getScopeValues,
  normalizeActorScope,
  toActorIssuesFilter,
} from "./actor-issues-panel";

describe("actor issues scopes", () => {
  it("uses participated scope for agents and created scope for members", () => {
    expect(getScopeValues("agent")).toEqual(["assigned", "participated"]);
    expect(getScopeValues("member")).toEqual(["assigned", "created"]);
  });

  it("falls back to assigned when persisted scope is invalid for actor type", () => {
    expect(normalizeActorScope("created", "agent")).toBe("assigned");
    expect(normalizeActorScope("participated", "member")).toBe("assigned");
  });

  it("maps participated scope to participated_agent_id filter", () => {
    expect(toActorIssuesFilter("participated", "agent-1")).toEqual({
      participated_agent_id: "agent-1",
    });
  });
});
