"use client";

import { useCallback } from "react";
import { toast } from "sonner";
import { useWSEvent } from "@multica/core/realtime";
import { useQuickCreateStore } from "@multica/core/issues/stores/quick-create-store";
import { useWorkspacePaths } from "@multica/core/paths";
import { useNavigation } from "../navigation";
import { stripQuickCreatePrefix } from "../inbox/components/inbox-display";
import type { InboxNewPayload } from "@multica/core/types";

/**
 * Invisible component that watches for quick-create completion/failure via
 * WebSocket `inbox:new` events and updates the persistent loading toast
 * shown after a Quick Capture submit.
 *
 * Mount once inside `DashboardLayout` — it renders nothing.
 */
export function QuickCreateToasts() {
  const removePendingTask = useQuickCreateStore((s) => s.removePendingTask);
  const paths = useWorkspacePaths();
  const navigation = useNavigation();

  const handler = useCallback(
    (payload: unknown) => {
      const { item } = payload as InboxNewPayload;
      if (!item) return;

      const taskId = item.details?.task_id;
      if (!taskId) return;

      // Only handle items that match a pending quick-create we initiated.
      const pending = useQuickCreateStore.getState().pendingTasks[taskId];
      if (!pending) return;

      if (item.type === "quick_create_done") {
        const identifier = item.details?.identifier ?? "";
        const title = stripQuickCreatePrefix(item.title, identifier);
        const issueId = item.issue_id;

        toast.success(title || "Issue created", {
          id: taskId,
          description: identifier || undefined,
          duration: 5000,
          action: issueId
            ? {
                label: "View",
                onClick: () => navigation.push(paths.issueDetail(issueId)),
              }
            : undefined,
        });
        removePendingTask(taskId);
      } else if (item.type === "quick_create_failed") {
        const error =
          item.details?.error || item.body || "Quick create did not finish";

        toast.error("Failed to create issue", {
          id: taskId,
          description: error,
          duration: 8000,
        });
        removePendingTask(taskId);
      }
    },
    [removePendingTask, paths, navigation],
  );

  useWSEvent("inbox:new", handler);

  return null;
}
