# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository

**cabrero** — CC auto-improvement system (MIT licensed). Go binary that observes Claude Code sessions, extracts behavioral signals, and proposes improvements to SKILL.md files with human approval. See DESIGN.md for full architecture.

## Documentation

- **DESIGN.md** is the living architecture document. Keep it updated whenever adding, changing, or removing features, commands, pipeline stages, or architectural decisions. If a commit changes behavior documented in DESIGN.md, update DESIGN.md in the same commit.
- **CHANGELOG.md** follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Add an entry under an `[Unreleased]` section for every user-visible change (new feature, fix, removal, deprecation). When a version is tagged, move unreleased entries under the new version heading with the release date.

## Git

- Remote: `git@github.com:vladolaru/cabrero.git`
- Main branch: `main`
