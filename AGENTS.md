# Agent Instructions

This repository follows an agent-native workflow. Keep changes small, verifiable, and aligned with the documented layer boundaries.

## Read First

1. `README.md` for the public workspace surface.
2. `docs/index.md` for canonical documentation routing.
3. `docs/architecture.md` for layer boundaries and dependency direction.
4. A matching guide in `docs/workflows/` before implementing a recurring task.

## Operating Rules

- Use `rtk` before shell commands in this workspace.
- Prefer `rg` or `rg --files` for search.
- Keep agent routing docs short; put stable facts in `docs/`.
- Do not add product, framework, storage, or vendor assumptions without recording the boundary in `docs/architecture.md`.
- Add or update executable validation when adding a public surface, adapter, workflow, or generated artifact.
- Source files must not exceed 400 lines, excluding dependencies, build outputs, and generated code. If a file exceeds the limit, split it by business or view logic before proceeding. The frontend demo entry `frontend/src/app.ts` is intentionally exempt until it graduates from a single-file demo.

## Validation

Run:

```bash
rtk bash scripts/validate-workspace.sh
```

Completion means the command passes and the changed workflow has evidence listed in the relevant docs.
