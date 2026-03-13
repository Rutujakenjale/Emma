# RUNBOOK — Coupon Import Service

This runbook contains common operational tasks: start/stop, upgrades, rollbacks, and troubleshooting steps.

## Start (local / prod)
- Ensure environment variables are set:
  - `USE_DB=1` (to enable DB-backed service) or unset for in-memory
  - `DB_PATH` path to database (default `data.db`)
  - `API_KEY` (production only)
  - `PORT` (optional)
- Apply migrations (if using a fresh DB):

```bash
sqlite3 $DB_PATH < migrations/001_init.sql
```

- Start service:

```bash
go run ./cmd/api
```

Or build and run a binary:

```bash
go build -o coupon-import ./cmd/api
./coupon-import
```

## Stop / Graceful shutdown
- The service listens for SIGINT/SIGTERM and will wait up to 30s for workers to finish.
- To stop immediately: send SIGKILL (not graceful) or kill the process.

## Deploy / Upgrade
1. Ensure CI runs `go test ./...` and `govulncheck` on PRs.
2. Deploy the new binary or container to staging; set `API_KEY` and DB connection.
3. Run smoke tests: upload a small CSV, check job status and error listing.
4. Monitor logs and metrics for errors and queue growth.

## Rollback
1. If DB schema migrations are non-destructive, deploy previous binary and monitor.
2. If schema changes are destructive, restore DB from backup before starting previous binary.
3. Always test rollback in staging before production.

## Troubleshooting
- Job stuck in `processing`: check worker logs, queue depth, and whether `ProcessFile` is blocked on I/O.
- High failure rate: inspect `import_errors` table for common validation failures; consider improving CSV validation or notifying client.
- Duplicate code constraint errors: check `promotion_codes` uniqueness and consider providing better error messages to clients.
- Disk space: ensure temp directory and DB path have sufficient free space.
- Long running imports: consider increasing worker timeout or moving to external queue and scalable workers.

## Recovery
- To reprocess failed rows: use the `POST /api/v1/imports/{id}/retry` endpoint which creates a new job for failed records.
- To rebuild DB from scratch (destructive): stop service, remove DB file, re-run migrations, and re-import data.

## Maintenance
- Periodically vacuum SQLite (if used) and back up DB file.
- Rotate `API_KEY` and other secrets regularly.

## Contact & Escalation
- Primary: Dev on-call
- Secondary: Team lead
