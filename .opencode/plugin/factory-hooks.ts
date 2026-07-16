import type { Plugin } from "@opencode-ai/plugin"
import { execFile, execFileSync } from "node:child_process"
import { join } from "node:path"
import { existsSync, unlinkSync, writeFileSync } from "node:fs"

export default (async ({ client, project, directory, $ }) => {
  // Session → agent name map, populated by the chat.message hook.
  // tool.execute.before has sessionID but not agent; we derive the agent
  // from the most recent chat.message for that session.
  // Verified against opencode plugin types (packages/plugin/src/index.ts):
  //   tool.execute.before input: { tool: string; sessionID: string; callID: string }
  //   chat.message input: { sessionID: string; agent?: string }
  const sessionAgents = new Map<string, string>()

  // Record the session-start HEAD for the loop-close check (dispose hook).
  const flagPath = join(directory, "memory", ".pending-lesson-reminder")
  let startHead = ""
  try {
    startHead = execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: directory,
      encoding: "utf-8",
      timeout: 3000,
    }).trim()
  } catch {
    // Not a git repo or git unavailable — loop-close check will skip silently.
  }

  return {
    // Capture the agent name for each session from chat messages.
    "chat.message": async (input, output) => {
      if (input.agent) {
        sessionAgents.set(input.sessionID, input.agent)
      }
    },

    "tool.execute.before": async (input, output) => {
      const toolName = input.tool ?? ""

      // Test-edit denial: call the shared shell script.
      // per ADR-0004 Decision 2: no inline enforcement logic in the plugin.
      // The script (scripts/hooks/test-edit-denial.sh) is the single source of the rule.
      if (toolName === "edit" || toolName === "write") {
        const filePath = output?.args?.filePath ?? output?.args?.path ?? ""

        // Derive the agent role from the session's agent name.
        const agentName = sessionAgents.get(input.sessionID) ?? ""
        const role = agentNameToRole(agentName)

        // Call the shared script via execFile (non-promisified to avoid stdin deadlock).
        // The script reads JSON from stdin, so we must write to stdin and close it.
        // The promisified execFile does not support the `input` option (that's execFileSync only).
        const scriptPath = join(directory, "scripts", "hooks", "test-edit-denial.sh")
        const payload = JSON.stringify({
          tool_name: toolName,
          tool_input: { file_path: filePath },
        })

        const exitCode = await new Promise<number>((resolve) => {
          const child = execFile(scriptPath, [], {
            env: {
              ...process.env,
              FACTORY_AGENT_ROLE: role,
            },
            timeout: 5000,
          }, (err) => {
            // err is non-null if the child exited with non-zero code or timed out.
            // Exit code 2 = intentional denial (not an error in our protocol).
            if (err && "code" in err && typeof err.code === "number") {
              resolve(err.code)
            } else if (err) {
              // Script missing, timeout, or other error — fail open (see tradeoff in commit message).
              resolve(0)
            } else {
              resolve(0)
            }
          })

          // Write the JSON payload to the child's stdin and close it.
          // This unblocks the script's `INPUT=$(cat)` line.
          child.stdin?.end(payload)
        })

        if (exitCode === 2) {
          throw new Error("DENIED: implementer role cannot edit test files (*_test.go). Generator/evaluator separation.")
        }
      }
    },

    "tool.execute.after": async (input, output) => {
      // PostToolUse: run gofmt on Go files after edit.
      // Uses execFile (not exec) to prevent command injection.
      const toolName = input.tool ?? ""
      if (toolName === "edit" || toolName === "write") {
        const filePath = output?.args?.filePath ?? output?.args?.path ?? input?.args?.filePath ?? ""
        if (/\.go$/.test(filePath)) {
          try {
            const { promisify } = await import("node:util")
            const execFileAsync = promisify(execFile)
            await execFileAsync("gofmt", ["-w", filePath])
          } catch {
            // Best-effort; non-blocking
          }
        }
      }
    },

    // Second-brain loop-close nudge: on session.idle, write a flag file
    // that reminds the agent to reflect on whether the previous turn
    // produced a lesson worth writing to memory/lessons/.
    // Per AGENTS.md "Second-brain loop-close" rule + Karpathy pattern.
    event: async ({ event }) => {
      if (event.type === "session.idle") {
        const sessionID = event.properties.sessionID
        try {
          writeFileSync(flagPath,
            `sessionID: ${sessionID}\n` +
            `turn ended: ${new Date().toISOString()}\n\n` +
            `If this turn revealed a non-obvious fact (gotcha, version mismatch,\n` +
            `API shape, bug fix that cost time), write memory/lessons/NNN-*.md\n` +
            `with provenance. Then delete this file.\n` +
            `See AGENTS.md "Second-brain loop-close" rule.\n`
          )
        } catch {
          // Best-effort; non-blocking
        }
      }
    },

    // Best-effort loop-close check at process exit.
    // Calls the shared script which checks: files changed since startHead
    // but no new lesson files -> writes memory/PENDING-LESSONS.md.
    // Uses execFileSync (synchronous) so it completes before the process exits.
    // NOT VERIFIED: whether dispose fires on TUI quit (requires live testing).
    dispose: async () => {
      if (!startHead) return
      try {
        // Delete the per-turn flag file — session is ending, the nudge is stale.
        if (existsSync(flagPath)) {
          unlinkSync(flagPath)
        }
        const scriptPath = join(directory, "scripts", "hooks", "loop-close-check.sh")
        execFileSync(scriptPath, [], {
          cwd: directory,
          env: {
            ...process.env,
            FACTORY_SESSION_START_HEAD: startHead,
          },
          timeout: 5000,
          stdio: "pipe",
        })
      } catch {
        // Best-effort; non-blocking. The script exits 1 when it writes
        // a reminder (changes exist, no lessons) — that's expected, not an error.
      }
    },
  }
}) satisfies Plugin

// agentNameToRole maps an opencode agent name to a FACTORY_AGENT_ROLE value.
// The mapping is: implementer → "implementer" (denied test edits); all others → "" (allowed).
// This is the only place role derivation happens — the shell script does the enforcement.
function agentNameToRole(agentName: string): string {
  if (agentName === "implementer") {
    return "implementer"
  }
  return ""
}
