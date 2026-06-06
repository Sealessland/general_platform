---
name: postgresql-database
description: Use when working with PostgreSQL or PGSQL SQL, migrations, schemas, indexes, transactions, locks, isolation, query plans, seed data, psql/docker-compose database checks, or debugging PostgreSQL-backed repository behavior.
---

# PostgreSQL Database

Use this skill for PostgreSQL schema changes, SQL review, migrations, data fixes, query tuning, lock/concurrency bugs, and repository behavior that depends on PostgreSQL semantics.

## First Checks

1. Identify the active database entrypoint: migration file, repository adapter, SQL script, Docker Compose service, or psql command.
2. Read the schema before reasoning about behavior. In this workspace start with:
   - `backend/migrations/`
   - `backend/internal/redcart/infrastructure/postgres/`
   - `docker-compose.yml`
   - `docs/architecture/inventory-design.md` for inventory-related work
3. Separate PostgreSQL behavior from in-memory adapter behavior. A passing memory test does not prove PostgreSQL transaction safety.
4. If the task changes behavior, add or request a PostgreSQL-backed test, not only a unit test with the memory repository.

## SQL Safety Rules

- Prefer additive migrations. Do not edit already-applied migrations unless the user explicitly says the DB is disposable.
- Wrap multi-step data writes that must be atomic in a transaction.
- For inventory, money, orders, idempotency, and permissions, avoid read-then-write races. Prefer atomic conditional updates or row locks.
- Use `SELECT ... FOR UPDATE` only inside a transaction and only when the locked rows are indexed and bounded.
- For stock reservation, prefer patterns like:

```sql
UPDATE product_skus
SET locked_stock = locked_stock + $1
WHERE id = $2
  AND status = 'active'
  AND stock - locked_stock >= $1
RETURNING id, stock, locked_stock;
```

- Check `RowsAffected` or `RETURNING`; no returned row means business conflict, not success.
- Make uniqueness explicit with constraints or unique indexes, especially for idempotency keys, external IDs, and natural keys.
- Add `CHECK` constraints for local invariants when possible, such as non-negative money, stock, quantity, and locked stock.
- Avoid unbounded `UPDATE`/`DELETE`; require a selective `WHERE` and, for manual fixes, preview with `SELECT` first.

## Review Checklist

When reviewing PostgreSQL code, look for:

- Missing transaction around order + items + inventory + event writes.
- Read-modify-write updates without row lock or atomic condition.
- `ON CONFLICT DO NOTHING` hiding seed or migration drift.
- Seed data that creates inconsistent derived state, such as stock not matching locks.
- Repository methods returning empty slices on SQL errors, hiding database failures.
- Tests that skip integration by default and therefore do not exercise PostgreSQL.
- OpenAPI or docs claiming behavior that is only true for the memory adapter.

## Debug Workflow

1. Reproduce with PostgreSQL, not only the memory repository.
2. Capture exact command, DSN target, and whether Docker Compose or local PostgreSQL was used.
3. Inspect current rows before and after the operation.
4. For concurrency bugs, run parallel requests or transactions against a fresh fixture row.
5. If a race is found, identify the first non-atomic boundary: read, insert, update, lock, or commit.
6. Verify fixes with a PostgreSQL integration test and the workspace validation script.

## Useful Commands

Use the workspace `rtk` prefix when running commands here.

```bash
rtk docker compose ps
rtk docker compose exec postgres psql -h 127.0.0.1 -p 15432 -U postgres -d redcart -c '\dt'
rtk docker compose exec postgres psql -h 127.0.0.1 -p 15432 -U postgres -d redcart -c 'SELECT version();'
rtk bash ci/scripts/backend-ci.sh
```

For local backend integration tests:

```bash
cd backend
POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres -v
```

## Completion

Before claiming done:

- State whether the behavior was verified against PostgreSQL.
- State whether migration/schema changes were made.
- Run relevant PostgreSQL integration tests when possible.
- In this workspace, also run `rtk bash scripts/validate-workspace.sh`.
