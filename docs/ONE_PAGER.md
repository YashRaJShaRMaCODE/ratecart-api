# RateCart — product one-pager

## What it is

An early Go API prototype for tenant-isolated product catalog records and simple request quotas.

## Who may need it

Indie commerce SaaS teams that expose catalog data to multiple customers and want a safer starter than writing tenancy and quota plumbing from scratch. This buyer is a hypothesis; no customer or revenue is verified.

## The problem to prove

Do small teams repeatedly spend engineering time building catalog APIs, tenant boundaries, and usage limits—and would they pay for a focused service or a supported implementation?

## Current proof

- Local Go API, tenant-scoped catalog handlers and quota behavior.
- OpenAPI 3.1 contract; optional PostgreSQL persistence.
- Local tests, vet, and build passed in the 2026-11-16 audit.

## Not ready to sell as hosted software

No self-serve accounts, managed key lifecycle, billing, verified hosted endpoint, tested database restore, production monitoring, or customer support process. PostgreSQL integration and race test evidence remain open. Do not enter real customer data.

## First commercial experiment

Run 5–8 discovery interviews. If a buyer confirms pain, offer one bounded integration/pilot with written scope, success measure, price, support window, and data boundaries. Collect payment only after owner-approved legal, tax, and provider setup. Decide continue/revise/park using cash collected, delivery hours, cost, and repeat demand—not page views alone.

## Success measure

One paid pilot with a successful tenant-isolation and restore drill, buyer-approved outcome, measured support effort, and no unresolved high-severity security issue. No revenue guarantee.
