# Cabrero

For a good deal of time now, I've had this idea on my mind: every Claude Code session generates signal — what worked, what didn't, which skills helped, which got in the way. And without something collecting that signal, it just vanishes when the session ends.

That's the problem Cabrero solves. It captures that signal, finds the patterns, and proposes concrete improvements to your skills and configuration. You approve before anything changes. Nothing lands without your say-so.

The result? **Compound engineering** — each session makes the next one better. Skills sharpen from real usage, not guesswork. Over weeks and months, the system quietly accumulates hard-won knowledge that would otherwise be lost.

Named after the Spanish word for *goatherd* — the one who tends the flock, keeping insights from scattering.

## Why this exists

If you use Claude Code seriously, you've probably noticed a few things:

- You keep correcting the same behavior across sessions
- Skills you wrote months ago no longer match how you actually work
- You know *something* is slowing sessions down, but can't pinpoint what
- You've built up a library of skills and CLAUDE.md rules, but have no feedback loop telling you which ones are helping and which are getting in the way

I don't know about you, but I dislike losing hard-won knowledge to session boundaries. Every correction you make, every workaround you apply — that's signal about what your skills should say but don't. It dissipates the moment the session closes.

Cabrero closes that loop. It watches your sessions in the background, finds the recurring patterns — the retry storms, the late skill reads, the workarounds you keep applying — and turns them into actionable proposals. Complete with evidence traced back to the exact session turns that triggered them.

## How it works

Let me try and make the case for the approach before the mechanics.

The key insight: **you don't need to manually audit your sessions.** The pipeline does the investigation for you and shows its work — you just review what it found and decide whether the proposed change makes sense. Full traceability from proposal back to raw session turns at every layer.

```
Session ends  →  Capture  →  Parse  →  Classify  →  Evaluate  →  You approve  →  Apply
```

1. **Capture** — Hook scripts preserve your CC transcripts before compaction erases them. This is the foundation — if the raw data is gone, everything downstream is impossible.
2. **Parse** — A fast code-only pass extracts structural signals: tool retries, error patterns, friction indicators, skill usage timing. No LLM calls here, just deterministic extraction.
3. **Classify** — A lightweight model infers session goals and flags signals worth investigating, including patterns that recur across sessions.
4. **Evaluate** — A capable model assesses skill performance and generates proposed improvements with full citation chains.
5. **Approve** — You review every proposal, trace it back to the raw evidence, and decide.
6. **Apply** — Approved changes are blended into your skill files naturally, preserving tone and structure.

All AI calls go through the `claude` CLI — no separate API keys, no extra accounts. CC's existing auth is reused throughout.

## What it improves

- **Skills** (SKILL.md files) — the main target; iterative improvement from real usage
- **Commands** — custom slash commands
- **Agents** — sub-agent definitions
- **CLAUDE.md** — flags stale or counterproductive rules; proposes additions when it sees you correcting the same thing repeatedly

For third-party plugins you don't own, Cabrero switches to *evaluation mode* — it won't propose changes, but it'll tell you whether each plugin is helping or creating friction. Think of it as a fitness report, not a diff.

## Install

Two lines and you're set:

```bash
curl -fsSL https://raw.githubusercontent.com/vladolaru/cabrero/main/install.sh | bash
cabrero setup
```

The install script downloads a pre-built binary for your Mac (Apple Silicon or Intel). The setup wizard walks you through connecting everything — hooks, background daemon, PATH configuration. It shows what it'll do at each step and asks before making changes.

Or build from source if that's more your speed:

```bash
git clone https://github.com/vladolaru/cabrero.git
cd cabrero
make install
cabrero setup
```

### Requirements

- macOS (Apple Silicon or Intel)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI installed and authenticated

## Commands

Enough intro. Here's a rundown of what the CLI gives you:

```
cabrero setup       Set everything up (hooks, daemon, configuration)
cabrero status      Check health: store, hooks, daemon, sessions
cabrero sessions    Browse captured sessions
cabrero run         Run the analysis pipeline on a specific session
cabrero proposals   See what Cabrero is suggesting
cabrero inspect     Drill into a proposal with full evidence chain
cabrero update      Self-update to the latest release
cabrero daemon      Background processor (managed by launchd)
```

Run `cabrero help` for the full list.

## Project status

Active development. The capture layer, analysis pipeline, background daemon, and self-packaging are functional. The interactive review interface (approve/reject with TUI) and prompt iteration tooling are next.

If you are in a hurry, the [design document](DESIGN.md) has the full architecture and roadmap — all the layers, the LLM stack, cross-session pattern detection, the planned macOS menu bar app, and the prompt iteration system.

## Inspirations and acknowledgments

Cabrero builds on ideas and tools from across the ecosystem. Without wanting to sound prescriptive or definitive — here's what shaped the thinking:

- **[Compound Engineering](https://every.to/source-code/compound-engineering-the-definitive-guide)** by Kieran Klaassen and the team at [Every](https://every.to/) — this is the big one. Their thesis: *each unit of engineering work should make subsequent units easier, not harder.* Where most codebases accumulate complexity over time, compound engineering flips the dynamic — features teach the system new capabilities, bug fixes eliminate entire categories of future bugs, patterns become tools. Every proved this works at scale, running multiple products with single-person engineering teams. Cabrero applies the same principle to the AI layer: every Claude Code session feeds lessons back into the skills that guide the next one. Their [compound engineering plugin](https://github.com/EveryInc/compound-engineering-plugin) for Claude Code is a related effort worth exploring.
- **[Claude Code](https://docs.anthropic.com/en/docs/claude-code)** by Anthropic — the AI coding agent that Cabrero observes and improves. Its hook system (`PreCompact`, `SessionEnd`) makes non-invasive capture possible without modifying CC itself. This is no small feat — the hooks are what make the entire approach viable.
- **[SKILL.md convention](https://www.anthropic.com/engineering/claude-code-best-practices)** — Anthropic's approach to reusable, structured instructions for Claude. Cabrero treats these as the primary artifact to iterate on.
- **Feedback loops in developer tooling** — inspired by how linters, type checkers, and test suites create tight loops between action and learning. Cabrero extends that pattern to the AI layer: your skills get the same continuous-improvement treatment your code does.
- **[GoReleaser](https://goreleaser.com/)** — powers the cross-compilation and release automation.
- **[Keep a Changelog](https://keepachangelog.com/)** and **[Conventional Commits](https://www.conventionalcommits.org/)** — the documentation and commit conventions this project follows.

## License

[MIT](LICENSE)
