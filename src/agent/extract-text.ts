import type { AgentMessage } from "@mariozechner/pi-agent-core";

/** Extract plain text content from an AgentMessage */
export function extractText(message: AgentMessage | undefined): string {
  if (!message || typeof message !== "object" || !("content" in message)) return "";
  const content = (message as { content?: Array<{ type: string; text?: string }> }).content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((c) => c.type === "text")
    .map((c) => c.text ?? "")
    .join("");
}

/** Extract thinking/reasoning content from an AgentMessage */
export function extractThinking(message: AgentMessage | undefined): string {
  if (!message || typeof message !== "object" || !("content" in message)) return "";
  const content = (message as { content?: Array<{ type: string; thinking?: string }> }).content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((c) => c.type === "thinking")
    .map((c) => c.thinking ?? "")
    .join("");
}
