# RateCart feature roadmap

This is a discovery-led roadmap, not a promise that every feature belongs in the product. Prioritize only after buyer interviews. Effort is relative for this repository.

| # | Feature | User problem / competitor gap to test | Impact | Effort |
|---:|---|---|---|---|
| 1 | Tenant-scoped API keys with rotation/revocation | Manual env-only provisioning is hard to operate; validate against gateway setup overhead. | High | High |
| 2 | Usage dashboard with quota, errors, and latency | Teams cannot see which tenant is near its limit. | High | High |
| 3 | Self-serve tenant onboarding | Manual operator setup blocks evaluation. | High | High |
| 4 | OpenAPI explorer and copyable curl examples | New API consumers need a quick first successful call. | High | Medium |
| 5 | Per-plan quotas and hard spend ceilings | Buyers need predictable use and no surprise overages (reported in marketplace reviews). | High | High |
| 6 | Webhook notifications for quota thresholds | Operators need timely usage alerts. | High | Medium |
| 7 | Bulk CSV catalog import with validation preview | Small sellers need to migrate data without hand-written scripts. | High | High |
| 8 | Idempotent upsert/bulk sync endpoint | Integrations need safe retries and incremental updates. | High | Medium |
| 9 | Audit log for key and catalog changes | Teams need to explain access and changes after incidents. | High | Medium |
| 10 | Per-key scopes and least-privilege access | One leaked key should not expose every operation. | High | Medium |
| 11 | Catalog schema validation/versioning | Producers need safer schema changes and compatibility feedback. | Medium | High |
| 12 | Search/filter/sort with bounded query cost | Consumers need usable catalogs without downloading all records. | Medium | Medium |
| 13 | SDK/client snippets generated from OpenAPI | Reduce integration effort across common languages. | Medium | Medium |
| 14 | Usage export and invoice-ready CSV | Buyers need internal cost allocation and reconciliation. | Medium | Medium |
| 15 | Workspace/team roles | Multiple operators need safe shared access. | Medium | High |
| 16 | Regional deployment and data-retention controls | Buyers need locality and deletion controls before production data. | Medium | High |
| 17 | Cache with explicit freshness/invalidation | Repeated catalog reads can increase cost/latency. | Medium | High |
| 18 | Integration recipes for Shopify/WooCommerce/webhooks | Sellers need to sync from the source they already use. | Medium | High |
| 19 | Status page and incident notices | API consumers need trustworthy service status. | Medium | Medium |
| 20 | Hosted pilot onboarding checklist and sample dataset | Buyers need a safe evaluation without importing real customer data. | High | Low |

## Build gates

1. Validate buyer and current workaround before adding hosted accounts or billing.
2. First product milestone: safe key lifecycle, OpenAPI quickstart, import preview, and measurable health/usage. Each needs tests, UI states, documentation, and a local synthetic demo.
3. Before any hosted pilot: managed database, backups and restore proof, abuse controls, monitoring, key revocation, privacy/security review, and an explicit support boundary.
4. Billing, AI, collaboration, and integrations remain deferred until the paid use case and support cost are evidenced. No feature should imply a production SLA before operations exist.
