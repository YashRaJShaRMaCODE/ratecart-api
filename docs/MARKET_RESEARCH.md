# RateCart market research

Research snapshot: 2026-11-17. RateCart is a small Go prototype for tenant-scoped product records and API quotas. The products below solve adjacent API-management/marketplace jobs, not the same narrow product-catalog problem. Prices vary by region, usage, and contract; verify before making a buyer-facing comparison.

## Buyer hypothesis

Small commerce SaaS teams that need to expose a tenant-isolated catalog API may pay for a managed API plus simple, predictable quotas and onboarding. This is unvalidated. The first discovery question is whether catalog ingestion and tenant-safe API access are painful enough to buy separately from an existing gateway or commerce platform.

## Alternatives and gaps

| Alternative | Public pricing signal | Gaps from user reviews (not universal) |
| --- | --- | --- |
| [Amazon API Gateway](https://aws.amazon.com/api-gateway/pricing/) | Pay per request/data transfer; AWS lists a free tier for eligible new accounts. | G2 reviewers report costs can be hard to forecast at volume; configuration/debugging spans AWS services; REST/HTTP feature differences can complicate choices. [G2 reviews](https://www.g2.com/products/amazon-api-gateway/reviews) |
| [Kong Konnect](https://konghq.com/pricing) | API Management Plus starts at $25/month plus usage; limits and extra charges apply; Enterprise is custom. | G2 reviewers mention a steep learning curve, dense concepts, and costs/tiers that rise with usage. [G2 reviews](https://www.g2.com/products/kong-inc-kong-konnect/reviews) |
| [Google Apigee](https://cloud.google.com/apigee/pricing) | Evaluation, pay-as-you-go and subscription options; exact bill depends on environments, proxies, and usage. | G2 reviewers mention an immature developer portal and slow proxy deployments; minor policy changes can take time to propagate. [G2 reviews](https://www.g2.com/products/apigee-api-management/reviews) |
| [RapidAPI Hub](https://docs.rapidapi.com/do/docs/monetizing-your-api-on-rapidapicom) | Provider sets API plans; payout is reduced by marketplace and payment-processing fees. | G2 reviews report inconsistent API reliability/documentation, limited spend/log controls, and support/UX complaints. [G2 reviews](https://www.g2.com/products/rapidapi/reviews?qs=pros-and-cons) |
| [Gravitee](https://www.gravitee.io/pricing) | Public page lists Planet at $2,500/month; higher tiers are custom, with capacity/module limits. | G2 reviewers mention a learning curve before teams get useful results; a reviewer also reports gaps in event-management integrations and schema/catalog workflows. [G2 reviews](https://www.g2.com/products/gravitee-io/reviews) |

Review evidence is directional: public review sets are self-selected and some complaints concern broader gateway products, not catalog endpoints. Do not turn review excerpts into guaranteed competitor weaknesses.

## Positioning to test

“A small, self-hostable catalog API starter with tenant isolation, bounded usage, and a clear path from local evaluation to a paid pilot.” This is a hypothesis, not a claim of superiority. Differentiate only if interviews confirm a specific commerce workflow that general gateways leave awkward.

## Buyer validation

Interview 5–8 indie commerce/API teams. Ask for a recent catalog/API integration, current tools and bill, data/tenant failure risks, and who approves a pilot. Do not pitch until the problem is understood. Continue only after a buyer agrees to a bounded pilot with success criteria and price discussion.
