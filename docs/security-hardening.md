# Security Hardening

GuardSQL treats customer-authored query text as an untrusted source language.
The engine should accept only the subset it can parse, validate, analyze,
authorize, and compile from trusted AST nodes.

## Why This Is Secure

GuardSQL's safety rests on three principles, not on filtering dangerous input.

**Regeneration, not passthrough.** The SQL that runs is *emitted from the
validated AST*, never the user's original text. Only constructs the AST can
represent — and the compiler knows how to write — can reach the database. There
is no grammar for stacked statements, comments, arbitrary functions, arithmetic,
DDL, or file/network access, so they cannot be emitted at all. This is
default-deny by construction, the same principle as sanitizing HTML by
re-serializing it rather than regex-filtering.

**Everything is an allowlist.** Entities, fields, per-field read/write
capabilities, aggregate functions, join types, and identifier characters are all
deny-by-default allowlists. A query can only name what the schema explicitly
permits; anything else fails validation or compilation.

**Values are parameters; identifiers are locked.** Every literal is a bind
parameter, so value-level injection is impossible. Identifiers — which cannot be
parameterized — are resolved against the schema, restricted to a safe charset,
and re-quoted, so identifier-level injection is impossible too.

These guarantees hold on the compiled path (`Compile` / `Config.Compile`). Using
GuardSQL only as a parse/policy gate and then executing the original text
verbatim gives up regeneration and weakens the injection guarantees to whatever
the parser happens to reject — always compile.

## Attack Coverage

Strength ratings assume the compiled path (`Compile` / `Config.Compile`). Using
GuardSQL only as a parse/policy gate and then executing the original text
verbatim weakens the injection guarantees to whatever the parser rejects.

- **Strong** — prevented by construction.
- **Partial** — reduced at the language layer; close fully with host/database controls.
- **Shared** — GuardSQL enforces a decision the application must make.
- **App-layer** — outside GuardSQL's scope.

| Attack class | Strength | GuardSQL posture |
|--------------|----------|------------------|
| SQL injection — values | Strong | Every literal is a bind parameter; values are never concatenated into SQL. |
| SQL injection — identifiers | Strong | Table/column names must resolve to a schema field and pass `IsSafeIdentifier`, then are quoted per dialect. |
| Blind / boolean / timing injection | Strong | Parameterized values plus no scalar functions or arithmetic in the grammar leave no oracle to build. |
| Stacked queries, DDL, procedures, file/network access | Strong | Not representable in the AST, so the compiler cannot emit them. |
| Unicode / homoglyph identifier spoofing | Strong | `IsSafeIdentifier` is ASCII-only and rejects non-identifier characters. |
| Second-order injection | App-layer | Values are stored safely as parameters; if other code later concatenates a stored value into SQL, that path is outside GuardSQL. |
| Query-complexity denial of service | Partial | Policy bounds depth, node count, `IN` size, `LIMIT`, and select/join/group/CTE counts; expensive-but-legal queries still need DB timeouts, row caps, and a least-privileged role. |
| Column-level data exposure (secrets / PII) | Strong | Entity/field allowlist plus `ClassSecret`/`ClassInternal`/`ClassPII` stripping; denied columns read as "unknown field" without disclosing they exist. |
| Mass assignment via mutations | Strong | Assignment targets must be columns explicitly marked `Writable` in the schema (deny-by-default), so a caller cannot write a column such as `role` or `is_admin` unless permitted; mutations are also trusted-only and reject unknown columns. |
| Full-table `UPDATE` / `DELETE` | Strong | A `WHERE` clause is required by default; unfiltered mutations are refused unless explicitly allowed. |
| Error-based information disclosure | Strong | Errors are structural; denied/secret columns surface as unknown fields rather than confirming existence. |
| Broken object-level authorization (IDOR) | Strong&nbsp;* | The app supplies the trusted scope (session principal → row filter); GuardSQL injects it as a mandatory top-level `AND` on every SELECT/UPDATE/DELETE via `RowScope` (and stamps it onto INSERTs, while refusing assignment to scope columns), so no user `WHERE` can widen it. `RequireRowScope` makes unscoped access refuse to compile (fail-closed). See [Row Scope](trust-modes.md#row-scope-tenant--idor-isolation). |
| Multi-tenant data leakage | Strong&nbsp;* | Same mechanism: a forced `tenant_id` predicate is ANDed into every statement, and per-tenant schemas add column-level isolation. |
| Authorization bypass | Shared | External authz engines compile decisions into `guardsql.Policy` (`authz.PolicyFromDecision`); GuardSQL enforces the compiled decision against the AST. |

\* Strong **when the application configures `RowScope`** from trusted session
context — GuardSQL enforces the scope but cannot know the session tenant/owner
on its own. Without it, row-level isolation is not enforced.

The database account remains the final boundary for resource and RCE risk: run
untrusted queries against a read-only, least-privileged identity with row-level
security or approved tenant-scoped views.

## Enforced in GuardSQL

The Go engine can enforce these controls before backend execution:

- one parsed query, with complete input consumption
- read-only `OperationRead`
- allowlisted entities
- field usage by operation: select, filter, sort, group, aggregate, join, and
  having
- aggregate function allowlists
- join type allowlists
- `SELECT *` denial
- required `LIMIT` and maximum `LIMIT`
- expression depth and node count limits
- maximum `IN` list size
- maximum select, sort, group, join, CTE, and subquery counts
- CTE and nested-source denial by default
- resolved requirement extraction with `Analyze`

## Enforced by the Compiler

When you compile with the built-in [SQL compiler](compiler.md) (directly or via
[Config](trust-modes.md)), GuardSQL also enforces at emit time:

- **Value parameterization** — every literal is a bind parameter, never
  concatenated. Value-level injection is impossible.
- **Identifier lockdown** — table and column names are re-resolved against the
  schema, re-quoted per dialect, and (by default) required to match the
  unquoted identifier charset via `IsSafeIdentifier`. Because identifiers cannot
  be parameterized, this makes identifier-based injection structurally
  impossible. See [Identifier Safety](trust-modes.md#identifier-safety).
- **Data-classification stripping** — fields classed `ClassSecret`, `ClassInternal`, or `ClassPII`
  are removed from the effective schema unless explicitly allowed, so untrusted
  queries cannot reference them (and are not told they exist). See
  [Data Classification](trust-modes.md#data-classification).
- **Regeneration, not passthrough** — the executed SQL is emitted from the
  validated AST, so only supported constructs can reach the database.

## Trusted vs Untrusted

GuardSQL serves both customer-supplied SQL and internal application SQL from one
engine. `NewUntrustedConfig` is read-only, strips secret/internal/PII columns, requires a
`LIMIT`, and locks identifiers. `NewTrustedConfig` additionally allows access to
classified columns and trusted [mutations](mutations.md) — still parameterized
and identifier-locked. Never use the trusted profile for SQL that originates
outside the application.

## Enforced with Analysis

Use the resolved analysis path for operational systems:

```go
q, err := guardsql.Parse(input)
if err != nil {
    return err
}
if err := guardsql.Validate(q, schema); err != nil {
    return err
}
analysis, err := guardsql.Analyze(q, schema)
if err != nil {
    return err
}
policy := guardsql.SafeAnalyticsPolicy(schema)
if issues := guardsql.CheckPolicy(q, policy); len(issues) > 0 {
    return fmt.Errorf("query shape not allowed: %s", issues[0].Message)
}
if issues := guardsql.CheckAnalysisPolicy(analysis, policy); len(issues) > 0 {
    return fmt.Errorf("query requirements not allowed: %s", issues[0].Message)
}
```

`SafeAnalyticsPolicy` is a starting point, not a deployment policy. Host
applications should lower or raise limits deliberately and compile current
authorization decisions into the policy.

## Example Policy

See `examples/customer_analytics_policy.yaml` for a human-readable profile that
matches the intended fail-closed analytics posture.

## Host Requirements

Some controls cannot be proven by the AST engine alone. Production systems
should also enforce:

- trusted tenant scope outside user-authored text
- parameterized backend rendering only
- read-only database role
- no DDL, DML, procedure, file, extension, network, or session privileges
- database row-level security or approved tenant-scoped views
- statement, transaction, lock, memory, row, and returned-byte limits
- per-tenant rate limits and concurrency limits
- cancellation on client disconnect
- audit logs for original query, normalized AST, analysis requirements, policy
  issues, authorization decision, and rendered query fingerprint

The database account remains the final RCE boundary. GuardSQL reduces the
accepted query language to a policy-approved subset, but it should still run
against a least-privileged backend identity.

