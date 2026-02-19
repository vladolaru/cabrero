# Cabrero

Cabrero is a macOS tool that watches your [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
sessions, extracts behavioral signals, and proposes improvements to your SKILL.md files — with
human approval before any change lands.

Named after the Spanish word for goatherd — the one who tends the flock, keeping insights from
scattering. Every CC session generates signal about what worked and what didn't. Without something
collecting and acting on that signal, it dissipates. Cabrero makes it compound: each session
feeds improvements into the next.

## How it works

```
Observe  →  Extract  →  Propose  →  Human approves  →  Apply
```

1. **Observe** — Hook scripts capture CC session transcripts before compaction erases them
2. **Extract** — A fast pre-parser pulls structural signals (tool retries, late skill reads,
   error patterns) without any LLM calls
3. **Classify** — Haiku infers session goals and classifies signals worth investigating
4. **Evaluate** — Opus assesses skill performance and generates proposed improvements
5. **Approve** — You review every proposal with full traceability back to raw session turns
6. **Apply** — Approved changes are blended into SKILL.md files via the `claude` CLI,
   preserving tone and structure

All LLM calls go through the `claude` CLI — no separate API keys, no extra auth.

## Requirements

- macOS
- Go 1.22+ (to build from source)
- [`claude`](https://docs.anthropic.com/en/docs/claude-code) CLI installed and authenticated

## Build and install

```bash
git clone https://github.com/vladolaru/cabrero.git
cd cabrero
go build -o cabrero .
```

Move the binary somewhere in your PATH, or:

```bash
mkdir -p ~/.cabrero/bin
cp cabrero ~/.cabrero/bin/
# Add ~/.cabrero/bin to your PATH
```

## Usage

```
cabrero help          Show all subcommands
cabrero import        Seed the store from existing CC session files
cabrero status        Pipeline health and store overview
cabrero sessions      List captured sessions
cabrero run           Run the analysis pipeline on a session
cabrero proposals     List pending proposals
cabrero inspect       Show a proposal with full citation chain
cabrero approve       Approve and apply a proposal
cabrero reject        Reject a proposal with optional reason
cabrero replay        Re-run pipeline with a different prompt
cabrero prompts       List prompt files with versions
```

## Project status

Early development. The capture layer and CLI skeleton are functional. The analysis pipeline
and review interface are not yet implemented.

## Architecture

See [DESIGN.md](DESIGN.md) for the full architecture document covering the capture layer,
analysis pipeline, LLM stack, human approval gate, prompt iteration system, and planned
macOS menu bar app.
