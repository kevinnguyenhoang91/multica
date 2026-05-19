import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PageHeader } from "./page-header";

const sidebarState = vi.hoisted(() => ({
  current: null as null | { isMobile: boolean; state: "expanded" | "collapsed" },
}));

vi.mock("@multica/ui/components/ui/sidebar", () => ({
  SidebarTrigger: ({ className }: { className?: string }) => (
    <button data-testid="sidebar-trigger" className={className} type="button" />
  ),
  useOptionalSidebar: () => sidebarState.current,
}));

describe("PageHeader", () => {
  it("hides trigger on desktop when sidebar is expanded", () => {
    sidebarState.current = { isMobile: false, state: "expanded" };
    render(
      <PageHeader>
        <h1>Inbox</h1>
      </PageHeader>,
    );

    expect(screen.queryByTestId("sidebar-trigger")).not.toBeInTheDocument();
  });

  it("shows trigger on desktop when sidebar is collapsed", () => {
    sidebarState.current = { isMobile: false, state: "collapsed" };
    render(
      <PageHeader>
        <h1>Inbox</h1>
      </PageHeader>,
    );

    expect(screen.getByTestId("sidebar-trigger")).toBeInTheDocument();
  });

  it("shows trigger on mobile", () => {
    sidebarState.current = { isMobile: true, state: "expanded" };
    render(
      <PageHeader>
        <h1>Inbox</h1>
      </PageHeader>,
    );

    expect(screen.getByTestId("sidebar-trigger")).toBeInTheDocument();
  });
});
