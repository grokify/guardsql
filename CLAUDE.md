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
- Let host applications compile the AST into backend-specific parameterized
  SQL, Ent predicates, API filters, or in-memory execution.
- Never execute the original query string as SQL.

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
- Joins, CTEs, and nested query sources are represented in the AST and should be
  executed by backend-specific compilers.
- Self joins require aliases on both sources.
