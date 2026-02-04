/**
 * Run command - Execute a single prompt non-interactively
 *
 * Usage:
 *   multica run [options] <prompt>
 *   echo "prompt" | multica run
 */

import { Agent } from "../../runner.js";
import type { AgentOptions } from "../../types.js";
import type { ToolsConfig } from "../../tools/policy.js";
import { cyan, yellow, dim } from "../colors.js";

type RunOptions = {
  profile?: string;
  provider?: string;
  model?: string;
  apiKey?: string;
  baseUrl?: string;
  system?: string;
  thinking?: string;
  reasoning?: string;
  cwd?: string;
  session?: string;
  debug?: boolean;
  toolsProfile?: string;
  toolsAllow?: string[];
  toolsDeny?: string[];
  help?: boolean;
};

function printHelp() {
  console.log(`
${cyan("Usage:")} multica run [options] <prompt>
       echo "prompt" | multica run

${cyan("Options:")}
  ${yellow("--profile")} ID        Load agent profile
  ${yellow("--provider")} NAME     LLM provider (openai, anthropic, kimi, etc.)
  ${yellow("--model")} NAME        Model name
  ${yellow("--api-key")} KEY       API key (overrides environment)
  ${yellow("--base-url")} URL      Custom base URL for provider
  ${yellow("--system")} TEXT       System prompt (ignored if --profile set)
  ${yellow("--thinking")} LEVEL    Thinking level
  ${yellow("--reasoning")} MODE   Reasoning display mode (off, on, stream)
  ${yellow("--cwd")} DIR           Working directory
  ${yellow("--session")} ID        Session ID for persistence
  ${yellow("--debug")}             Enable debug logging
  ${yellow("--help")}, -h          Show this help

${cyan("Tools Configuration:")}
  ${yellow("--tools-profile")} P   Tool profile (minimal, coding, web, full)
  ${yellow("--tools-allow")} T     Allow specific tools (comma-separated)
  ${yellow("--tools-deny")} T      Deny specific tools (comma-separated)

${cyan("Examples:")}
  ${dim("# Run with default settings")}
  multica run "What is 2+2?"

  ${dim("# Use a specific profile")}
  multica run --profile coder "List files in this directory"

  ${dim("# Pipe input")}
  echo "Explain this code" | multica run

  ${dim("# Resume a session")}
  multica run --session abc123 "Continue from where we left off"
`);
}

function parseArgs(argv: string[]): { opts: RunOptions; prompt: string } {
  const args = [...argv];
  const opts: RunOptions = {};
  const promptParts: string[] = [];

  while (args.length > 0) {
    const arg = args.shift();
    if (!arg) break;

    if (arg === "--help" || arg === "-h") {
      opts.help = true;
      break;
    }
    if (arg === "--profile") {
      opts.profile = args.shift();
      continue;
    }
    if (arg === "--provider") {
      opts.provider = args.shift();
      continue;
    }
    if (arg === "--model") {
      opts.model = args.shift();
      continue;
    }
    if (arg === "--api-key") {
      opts.apiKey = args.shift();
      continue;
    }
    if (arg === "--base-url") {
      opts.baseUrl = args.shift();
      continue;
    }
    if (arg === "--system") {
      opts.system = args.shift();
      continue;
    }
    if (arg === "--thinking") {
      opts.thinking = args.shift();
      continue;
    }
    if (arg === "--reasoning") {
      opts.reasoning = args.shift();
      continue;
    }
    if (arg === "--cwd") {
      opts.cwd = args.shift();
      continue;
    }
    if (arg === "--session") {
      opts.session = args.shift();
      continue;
    }
    if (arg === "--debug") {
      opts.debug = true;
      continue;
    }
    if (arg === "--tools-profile") {
      opts.toolsProfile = args.shift();
      continue;
    }
    if (arg === "--tools-allow") {
      const value = args.shift();
      opts.toolsAllow = value?.split(",").map((s) => s.trim()) ?? [];
      continue;
    }
    if (arg === "--tools-deny") {
      const value = args.shift();
      opts.toolsDeny = value?.split(",").map((s) => s.trim()) ?? [];
      continue;
    }
    if (arg === "--") {
      promptParts.push(...args);
      break;
    }
    promptParts.push(arg);
  }

  return { opts, prompt: promptParts.join(" ") };
}

async function readStdin(): Promise<string> {
  if (process.stdin.isTTY) return "";
  return new Promise((resolve, reject) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => (data += chunk));
    process.stdin.on("end", () => resolve(data.trim()));
    process.stdin.on("error", reject);
  });
}

export async function runCommand(args: string[]): Promise<void> {
  const { opts, prompt } = parseArgs(args);

  if (opts.help) {
    printHelp();
    return;
  }

  const stdinPrompt = await readStdin();
  const finalPrompt = prompt || stdinPrompt;

  if (!finalPrompt) {
    printHelp();
    process.exit(1);
  }

  // Build tools config if any tools options are set
  let toolsConfig: ToolsConfig | undefined;
  if (opts.toolsProfile || opts.toolsAllow || opts.toolsDeny) {
    toolsConfig = {};
    if (opts.toolsProfile) {
      toolsConfig.profile = opts.toolsProfile as ToolsConfig["profile"];
    }
    if (opts.toolsAllow) {
      toolsConfig.allow = opts.toolsAllow;
    }
    if (opts.toolsDeny) {
      toolsConfig.deny = opts.toolsDeny;
    }
  }

  const agent = new Agent({
    profileId: opts.profile,
    provider: opts.provider,
    model: opts.model,
    apiKey: opts.apiKey,
    baseUrl: opts.baseUrl,
    systemPrompt: opts.system,
    thinkingLevel: opts.thinking as any,
    reasoningMode: opts.reasoning as AgentOptions["reasoningMode"],
    cwd: opts.cwd,
    sessionId: opts.session,
    debug: opts.debug,
    tools: toolsConfig,
  });

  // If it's a newly created session, notify user of sessionId
  if (!opts.session) {
    console.error(`[session: ${agent.sessionId}]`);
  }

  const result = await agent.run(finalPrompt);
  if (result.error) {
    console.error(`Error: ${result.error}`);
    process.exitCode = 1;
  }
}
