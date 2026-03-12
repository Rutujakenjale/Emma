# Coupon Import (Minimal)

This is a minimal runnable Go service that implements an async CSV import for promotion codes using SQLite.

Run:

```bash
go mod tidy
go run ./cmd/api
```

Endpoints:
- `POST /api/v1/imports` - multipart form upload `file` (CSV)
- `GET /api/v1/imports/:id` - get import job status
- `GET /api/v1/imports/:id/errors` - list errors

OpenAPI:
- The API OpenAPI (Swagger) spec is available at `/openapi.yaml` when the server runs.
	You can load this file into Swagger UI or other tools to view and interact with
	the full API documentation (includes extra endpoints such as validation and analytics).

Interactive docs:
- Visit `/docs` while the server is running to open an embedded Swagger UI that
	loads the local OpenAPI spec at `/openapi.yaml`.

The service stores data in `data.db` and uses an on-start migration file in `migrations/001_init.sql`.
