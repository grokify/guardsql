# CLAUDE.md - guardsql

GuardSQL is a small, SaaS-safe query language for analytics and reporting. The
core package should stay lightweight and should not import concrete
authorization engines or database drivers.

This repository is the Go engine. Non-Go runtime integrations should be kept in
separate repositories or modules. Hibernate support should live in a Java
project such as `github.com/grokify/guardsql-java` or
`github.com/grokify/guardsql-hibernate`.

## Core Contract

- Parse user-authored query text into a Go AST.
- Validate and check policy against the AST.
- Compile the validated AST into parameterized SQL. GuardSQL ships a built-in
  compiler (`Compile`, dialects SQLite/Postgres/MySQL, JSON extraction); host
  applications may also compile the AST into other backends (Ent predicates, API
  filters, in-memory execution).
- Never execute the original query string as SQL — always regenerate SQL from
  the AST so only supported constructs reach the database.

## High-Level Surfaces

- `Config` is the recommended entry point (parse → validate → policy → compile).
  `NewUntrustedConfig` (customer SQL: read-only, secret/PII stripped, identifier
  lockdown, LIMIT required) and `NewTrustedConfig` (internal SQL: reads +
  mutations, full column access) provide hardened defaults for the two trust
  levels.
- Field `Class` (`ClassPublic`/`ClassPII`/`ClassInternal`/`ClassSecret`) drives
  classification stripping; keep secrets, internal, and PII fields out of
  untrusted queries by default. `FromCatalog` builds a `Schema` from a
  `datacatalog` catalog.
- Field capabilities (`Selectable`/`Filterable`/`Sortable`/`Writable`) are all
  deny-by-default allowlists. `Writable` is the mass-assignment defense —
  mutation targets must be explicitly writable, even for trusted callers.
- Identifiers must match the unquoted charset by default (`IsSafeIdentifier`,
  `Schema.ValidateIdentifiers`) because identifiers cannot be parameterized. Do
  not weaken this default; `AllowUnsafeIdentifiers` is the explicit opt-out.
- `RowScope` (with optional `RequireRowScope`) enforces tenant/ownership
  isolation (IDOR): the app supplies a trusted predicate that is ANDed into
  every compiled SELECT/UPDATE/DELETE. GuardSQL enforces it; the app must decide
  it — never derive the scope from user-supplied query text.
- Trusted mutations use the `Insert`/`Update`/`Delete` builders (no mutation
  parser). Keep them deny-by-default and WHERE-required; never expose them to
  customer-supplied input.
- Compiler values are always bind parameters; never string-concatenate a value
  into emitted SQL.

## Query Safety

- Analytics endpoints should normally allow only `OperationRead`.
- Treat field policy as a deny-by-default allowlist when configured.
- Keep tenant scope, RBAC/ReBAC, rate limits, throttling, audit logging, and
  row limits outside user-authored query text.
- Persisted ASTs are useful for audit and caching, but authorization must be
  checked against current policy before execution.

## Optional Authorization Modules

- `authz` is lightweight and can define generic authorization decisions.
- `authzsystemforge` is a nested optional module for SystemForge and SpiceDB.
  Keep its dependencies out of the root module.
- Add future Cedar/Rego/SpiceDB examples as optional subpackages or nested
  modules when they require heavy dependencies.
- Do not add Java, Hibernate, JDBC, or JVM dependencies to this Go module.

## Current Boundaries

- The in-memory evaluator supports single-source filters, grouping, aggregates,
  `HAVING`, ordering, limits, and projection.
- The built-in SQL compiler supports single-source read queries (SELECT, WHERE,
  GROUP BY, HAVING, ORDER BY, LIMIT, aggregates, JSON paths). Joins, CTEs, and
  nested query sources are represented in the AST but not yet emitted by the
  built-in compiler; use a backend-specific compiler for those.
- Mutations are builder-only (no INSERT/UPDATE/DELETE parser yet). A mutation
  parser may be added later if trusted SQL-text adoption is needed.
- Self joins require aliases on both sources.
