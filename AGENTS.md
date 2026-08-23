# AGENTS.md - GuardSQL

This repo implements GuardSQL, a SaaS-safe query language and AST policy
toolkit for analytics/reporting.

## Rules for Agents

- Keep the root module lightweight. Do not add database drivers, Ent,
  SystemForge, SpiceDB, Cedar, Rego, Casbin, or other heavy dependencies to the
  root `go.mod`.
- Put heavyweight integrations in optional subpackages or nested modules.
  `authzsystemforge/` is intentionally a nested module.
- Keep this repository focused on the Go engine. Non-Go runtime integrations,
  including Hibernate, belong in separate repositories such as
  `github.com/grokify/guardsql-java` or
  `github.com/grokify/guardsql-hibernate`.
- Never suggest executing GuardSQL text as SQL. The safe path is parse, validate,
  check policy, compile trusted AST nodes, then execute with service-owned
  limits.
- Treat the AST as the trust boundary.
- Keep tenant scope, authorization, rate limits, throttling, row caps, and audit
  logging outside user-authored query text.
- Persisted ASTs are useful, but saved queries must be checked against current
  policy before execution.

## Common Commands

Root module:

```bash
go test ./...
go vet ./...
```

Optional SystemForge adapter:

```bash
cd authzsystemforge
go test ./...
go vet ./...
```

Docs:

```bash
mkdocs build --strict
```

## Current Execution Boundary

- The in-memory evaluator supports single-source filters, grouping, aggregates,
  `HAVING`, ordering, limits, and projection.
- Joins, CTEs, and nested sources are AST features for backend-specific
  compilers.
- Self joins require aliases on both sources.

## Related Repos

- `../omniroadmap`: product roadmapping analytics provider.
- `../../plexusone/uiforge`: analytics dashboard builder using GuardSQL
  Questions and SystemForge/SpiceDB policy.
- `../systemforge`: authorization bridge and SpiceDB provider.
- Future Java/Hibernate integration should be external to this repo.
