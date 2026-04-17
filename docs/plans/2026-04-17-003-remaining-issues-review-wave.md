---
title: feat: Remaining Anton issue review wave
type: feature
status: completed
date: 2026-04-17
origin: linear project Anton (PICA-232/233/234/235/236/237/239/240/241)
---

# feat: Remaining Anton issue review wave

## Overview

Implement the remaining Anton backlog in a bounded, test-backed wave and move all open issues to review-ready state.

## Target Issues

- PICA-234: doctor preflight + remediation guidance
- PICA-232: stronger doctor execution receipt
- PICA-235: task-state lifecycle commands and closure gates
- PICA-236: structured evidence/validation receipts
- PICA-233: structured handoff pack generation
- PICA-237: thin scoped thread brief and recipe surfaces
- PICA-239: contract drift audit across anton.yaml/docs
- PICA-240: scope and drift guardrails
- PICA-241: ergonomics hardening for agent-facing command outputs

## Scope Boundaries

- Keep Anton generic; no repo-specific adapters.
- Reuse current command groups where possible.
- Keep JSON contract explicit and stable through tests.

## Implementation Units

- [x] Unit 1: Extend `doctor` preflight + remediation and execution receipt fields
- [x] Unit 2: Add context/task drift guardrails and contract drift checks
- [x] Unit 3: Expand `task-state` lifecycle (`close`, `reopen`, `retarget`, `import`) and closure gates
- [x] Unit 4: Add structured evidence receipts to status and command payloads
- [x] Unit 5: Add `handoff` command surface for machine/human handoff packs
- [x] Unit 6: Add thin `threads` brief/recipe surfaces over `codex-threads`
- [x] Unit 7: Agent ergonomics pass (payload compactness, recovery hints, stable errors)
- [x] Unit 8: Tests, docs, Linear updates, and review transition
