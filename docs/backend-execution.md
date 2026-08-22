# Backend Execution

GrokifyQL is not a database engine. It provides an AST and helpers that backend
providers can use safely.

## In-Memory Evaluator

`Eval` supports:

- single-source filters
- grouping
- aggregate functions
- `HAVING`
- ordering
- limits
- projection

It is useful for tests, local demos, small in-memory datasets, and reference
behavior.

## Backend Compilers

Production systems should compile validated and policy-approved ASTs into their
own backend calls:

- SQL with placeholders and separate arguments
- Ent predicates
- DoltDB/MySQL queries
- PostgreSQL queries
- API filter structs
- application-specific query providers

Joins, CTEs, and nested sources are AST-level features. They should be handled
by backend-specific compilers that understand the target storage and tenant
model.

## Service-Owned Controls

Backends should always apply:

- tenant scope outside user text
- query timeout
- read-only database roles
- database row-level security or approved tenant-scoped views
- service-side maximum rows
- rate limits and throttling
- audit logging
- resource accounting

Do not rely only on user-provided `LIMIT`.

The database account remains the final boundary for database-to-host RCE risk.
Do not grant customer-query execution roles permission to create functions,
install extensions, read server files, call network/file/procedure features, or
mutate session security state.

## Output Names

Use `OutputName(selectItem)` to determine projected column names for aggregate
and aliased selections. This keeps saved query metadata and dashboard widgets
stable across backends.
