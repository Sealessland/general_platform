---
name: agent-native-shop
description: Use when working in the agent-native-shop workspace to route through its architecture docs, workflows, validation script, and agent completion checklist.
---

# Agent Native Shop

Start with `README.md`, then read `docs/index.md` and `docs/architecture.md`.

For recurring tasks, load only the matching workflow under `docs/workflows/`:

- Feature work: `docs/workflows/add-feature.md`
- Integration work: `docs/workflows/add-integration.md`
- Debugging: `docs/workflows/debug.md`
- Handoff validation: `docs/workflows/validate.md`

Before completion, run:

```bash
rtk bash scripts/validate-workspace.sh
```

Project-local Codex hooks live in `.codex/config.toml` and `.codex/hooks/redcart_project_hook.py`.
For handoff validation, run the quick hook gate; for cross-layer or PostgreSQL-sensitive changes, run the full gate:

```bash
rtk python3 .codex/hooks/redcart_project_hook.py --mode quick
rtk python3 .codex/hooks/redcart_project_hook.py --mode full
```

Use `docs/checklists/agent-native-completion.md` to audit the result.
