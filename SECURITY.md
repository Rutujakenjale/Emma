# Security Guidance

This file lists security controls, rationale, and recommended actions to harden the Coupon Import service.

## Secrets and credentials
- Do not store secrets in source control. Use environment variables or a secrets manager for `API_KEY`, DB credentials, and any cloud credentials.
- In CI, add secrets to the repository's encrypted secrets store and never print them in logs.

## Authentication & authorization
- The API supports a simple API key middleware (`API_KEY` env). In production, prefer stronger auth (JWT/OAuth2) or mutual TLS for client apps.
- Protect documentation and spec endpoints with the same middleware.

## Upload surface hardening
- Limit upload size (currently 10MB) and enforce a sensible upper bound based on expected file sizes.
- Sanitize filenames and save uploads to a secured temp directory with a random prefix (already implemented).
- Sniff MIME (using `http.DetectContentType`) and validate CSV header/schema before enqueuing.
- Validate every parsed field and record parse/validation failures instead of ignoring errors.

## Input validation & injection
- Treat all CSV fields as untrusted. Use parameterized SQL statements (prepared statements) to avoid SQL injection (current code uses `?` parameters).
- Validate and canonicalize timestamps, numbers, and text before persistence.

## Data protection
- If storing PII or sensitive promo metadata, encrypt data at rest and limit access to DB files.
- When storing uploaded files, use short TTLs and remove files after processing.

## Dependency & vulnerability management
- Run `govulncheck` in CI (added). Periodically run `go list -m -u all` and update dependencies after testing.
- Pin and audit transitive dependencies if you require strict supply-chain controls.

## Logging & privacy
- Avoid logging raw uploaded data or PII. Redact or omit sensitive fields in logs.
- Use structured logs for easier filtering and secure log storage with role-based access.

## Operational controls
- Run the service under a least-privileged user and restrict file system permissions for temp storage.
- Implement monitoring and alerting for job failure spikes and queue saturation.

## Incident response
- Define a runbook for failed imports: how to inspect `import_errors`, re-run retries, and restore DB from backups.

## Recommended immediate actions
1. Configure `API_KEY` and require it in non-development environments.
2. Add automated dependency updates and pause before merging: test upgrades in CI.
3. Rotate secrets and ensure backups for the DB.
