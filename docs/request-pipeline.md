# Request Pipeline

Hosted applications should process customer-authored GuardSQL in a fixed order.

```text
Parse -> Validate -> Analyze -> Policy -> Authorize -> Compile -> Execute
```

## Parse

`Parse` converts text into a `Query` AST. Anything that does not parse is a
syntax error, not an injection event.

```go
q, err := guardsql.Parse(input)
```

## Validate

`Validate` checks the AST against an allowlisted schema of entities and fields.
Validation is structural and schema-oriented. It is not a replacement for user
authorization.

```go
err := guardsql.Validate(q, schema)
```

## Analyze

`Analyze` resolves query sources and fields against the schema and returns the
resources, field usages, aggregate functions, and joins needed by the query.
This is the preferred input for authorization adapters because it describes what
the query actually attempts to use.

```go
analysis, err := guardsql.Analyze(q, schema)
if err != nil {
    return err
}

requirements := analysis.Requirements()
```

Qualified names resolve through source aliases. Once multiple sources are
present, unqualified fields must resolve to exactly one source or analysis fails
with an ambiguity error.

## Policy

`CheckPolicy` enforces read-only mode, allowed entities, field capabilities,
required limits, maximum limits, expression complexity, and maximum `IN` list
size.

```go
issues := guardsql.CheckPolicy(q, policy)
```

## Authorize

Authorization decisions should come from trusted application context, such as
tenant membership, RBAC/ReBAC roles, SpiceDB relationships, or service account
scopes.

Authorization may be enforced outside GuardSQL, or compiled into
`guardsql.Policy` before `CheckPolicy`.

For relationship-aware systems such as SpiceDB, prefer authorizing the
`RequirementSet` from `Analyze` instead of asking one broad question such as
"can this principal use analytics?"

## Compile

Compile only validated and policy-approved AST nodes into parameterized SQL. The
built-in [compiler](compiler.md) targets SQLite, PostgreSQL, and MySQL and binds
every value and locks every identifier:

```go
out, err := guardsql.Compile(q, guardsql.CompileOptions{Dialect: guardsql.DialectPostgres, Schema: schema})
```

Other backends (Ent predicates, API filters, in-memory filters, domain-specific
providers) can compile the same AST. Never compile by concatenating
user-authored text.

## One-Step Config

`Config` runs Validate → Policy → Compile in a single call with secure defaults,
and is the recommended entry point for most applications:

```go
out, err := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres).Compile(input)
```

See [Config & Trust Modes](trust-modes.md).

## Execute

Execution should use service-owned controls:

- tenant scope
- timeouts
- row limits
- rate limits
- audit logs
- resource controls
