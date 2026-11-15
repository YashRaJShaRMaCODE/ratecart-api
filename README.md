# RateCart API

RateCart is a Go API prototype for small commerce teams that need a tenant-scoped product catalog and a simple per-client request quota. API keys authenticate tenants; callers cannot choose their own rate-limit identity or access another tenant's products.

## Run locally

Requires Go 1.21 or newer. Set at least one API key before starting the server. Generate a long random key locally and keep it out of source control:

```powershell
$key = python -c "import secrets; print(secrets.token_urlsafe(32))"
$env:RATECART_API_KEYS = '[{"tenant_id":"demo","key":"' + $key + '"}]'
go run ./cmd/server
```

`RATECART_API_KEYS` is a JSON array of `{ "tenant_id", "key" }` entries. Use only letters, digits, `-` and `_` for tenant IDs; keys must be at least 32 characters. Multiple keys can map to one tenant for manual rotation. The authenticator compares SHA-256 digests in constant time and never logs keys; the configured secret still exists in the process environment, so use HTTPS and a secret manager in any hosted environment.

For persistent catalog data and a shared rate limit across server instances, set `DATABASE_URL` to a PostgreSQL connection string before starting. The service creates its tables and indexes on startup. Without it, local development uses memory and product/quota data disappears on restart. Set `APP_ENV=production` to make startup fail when `DATABASE_URL` is missing. Keep the connection string in the host's secret settings, never in source control.

## API

`GET /health` and `GET /openapi.json` are public. Health returns `503` if the configured datastore is unavailable, and `200` when ready. Every other route requires `Authorization: Bearer <api-key>`.

The public [OpenAPI 3.1 contract](/openapi.json) describes request/response schemas, bearer authentication, pagination, and error responses for client generation and API explorers.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/request` | Exercise the authenticated tenant's request quota. Body: `{"payload":{}}`. |
| `GET` | `/stats` | Read only this tenant's accepted and rejected counts; raw user IDs are not accepted or returned. This read-only route is not counted against the quota. |
| `POST` | `/products` | Create a product. SKU uniqueness is per tenant. |
| `PUT` | `/products/{id}` | Replace a product's name and SKU; SKU uniqueness and tenant ownership are enforced. |
| `DELETE` | `/products/{id}` | Permanently delete a product owned by this tenant; other tenants receive `404`. |
| `GET` | `/products?limit=20&offset=0` | List only this tenant's product summaries. Page size caps at 100. |
| `GET` | `/products/export?limit=25&offset=0` | Export complete records, including media URLs, in no-store pages capped at 25. |
| `GET` | `/products/{id}` | Read one of this tenant's products; another tenant's ID returns `404`. |
| `POST` | `/products/{id}/media` | Append image and video URLs to this tenant's product. At most 20 URLs per type. |

Create a product:

```sh
curl -X POST http://localhost:8080/products \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Desk lamp","sku":"LAMP-001","image_urls":["https://example.com/lamp.jpg"]}'
```

Update a product:

```sh
curl -X PUT http://localhost:8080/products/PRODUCT_ID \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Desk lamp","sku":"LAMP-002"}'
```

All authenticated request and catalog routes share a limit of five requests per tenant per rolling minute; `/stats` and `/health` are exempt. A `429` includes `X-RateLimit-Limit: 5`, `X-RateLimit-Remaining: 0`, and `Retry-After: 60` so clients can back off before retrying. Invalid JSON returns `400`, oversized bodies return `413`, missing or invalid credentials return `401`, and cross-tenant product lookups return `404`.

## Checks

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/server
```

GitHub Actions runs formatting, vet, race-enabled tests and a build. No API secrets are needed in CI.

## Product and hosting limits

This is an early API prototype, not a paid hosted service yet. PostgreSQL can persist product records and share the five-per-minute rolling quota across instances, but there is no backup/restore process. API-key configuration remains manual and process-local, with no self-serve onboarding, billing, key revocation or abuse monitoring. The in-memory local mode still resets data on restart. Do not use real customer catalogs until backups, tested restores, managed key rotation, abuse monitoring and privacy review are in place. Set `RATECART_TEST_DATABASE_URL` to a disposable PostgreSQL database to run the persistence and quota integration test.

The reasonable commercial path to validate is a developer API for indie commerce teams, with per-tenant plans after usage and hosting costs are measured. There are no verified customers or revenue claims. `free-for.dev` helps discover infrastructure for a demo; a free host is not suitable for persistent customer data or service guarantees.
