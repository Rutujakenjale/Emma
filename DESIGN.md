# Design Overview

This document briefly describes the architecture, data flow, and operational considerations for the Coupon Import service.

## Components
- `cmd/api` — HTTP server (Gin) that exposes API endpoints and serves docs/spec.
- `internal/handler` — HTTP handlers, request validation, file handling, and API-key middleware.
- `internal/service` — Business logic for creating import jobs, parsing CSVs, persisting progress, and recording errors. Two implementations:
  - `ImportService` — DB-backed (SQLite)
  - `InMemoryImportService` — ephemeral memory-backed for tests/dev
- `internal/worker` — bounded worker pool that runs background `ProcessFile` jobs.
- `migrations` — initial SQL schema for `promotion_codes`, `import_jobs`, `import_errors`.

## Data flow
1. Client uploads CSV to `POST /api/v1/imports` (multipart form `file`).
2. Handler validates MIME and CSV header, saves file to a temp path, creates an import job via `service.CreateJob`.
3. Job is enqueued to `internal/worker.Pool`. Workers call `service.ProcessFile(jobID, path)`.
4. `ProcessFile` streams the CSV, validates rows, writes promotion codes in batches, and records errors in `import_errors`.
5. Client polls `GET /api/v1/imports/:id` and `GET /api/v1/imports/:id/errors` for status and failures.

## Background job model
- Bounded queue with limited buffered capacity and a fixed number of workers.
- Enqueue returns quickly; jobs can fall back to a goroutine if the queue is full.
- Workers log processing errors; graceful shutdown waits for workers to complete with a timeout.

## Error handling
- Parse/validation errors for rows are recorded into `import_errors` (with raw row and reason).
- Batch insert errors (e.g., UNIQUE constraint) are parsed and recorded.
- Background errors are logged and the job state is updated to `failed/partial/completed`.

## Scalability and reliability
- For high throughput, replace the in-process worker with an external queue (e.g., Redis, RabbitMQ, cloud queue) and horizontally scale workers.
- Use a hardened RDBMS for concurrency and durability in production. SQLite is fine for small deployments and tests but not for heavy concurrent loads.
- Monitor job queue depth, job failure rates, and DB error rates.

## Observability
- Add structured logging and metrics (Prometheus): job accepted, started, completed, success_count, failure_count, queue length, worker errors.
- Add traces for long-running file processing (optional).

## Migrations and deployments
- `migrations/001_init.sql` contains the initial schema. Apply it on DB init.
- Use environment variables: `USE_DB`, `DB_PATH`, `PORT`, `API_KEY`.

## Next steps
- Harden input validation and edge cases for varied CSV encodings/locales.
- Add retry/backoff strategy for transient DB errors.
- Add e2e tests in CI with a real DB instance (or sqlite in temp file) and test large files.

## Architecture Diagram

```mermaid
flowchart LR
    Client[[Client]] -->|POST /imports (multipart)| API["API (Gin)"]
    API --> Handler["Import Handler"]
    Handler -->|save file & create job| DB[(SQLite / DB)]
    Handler -->|enqueue| Queue["Worker Pool (in-process) / Queue"]
    Queue --> Worker["Worker(s)"]
    Worker -->|ProcessFile| Service["ImportService"]
    Service --> DB
    Service --> Errors["import_errors table"]
    API -->|GET /imports/:id| DB
    API -->|GET /imports/:id/errors| Errors
    subgraph Observability
      Service --> Logs["Logs & Metrics"]
    end
```

The diagram shows the primary runtime components and data flow for imports.
