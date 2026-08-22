# Request Pipeline

Hosted applications should process customer-authored GrokifyQL in a fixed order.

```text
Parse -> Validate -> Analyze -> Policy -> Authorize -> Compile -> Execute
```

## Parse

`Parse` converts text into a `Query` AST. Anything that does not parse is a
syntax error, not an injection event.

```go
q, err := grokifyql.Parse(input)
```

## Validate

`Validate` checks the AST against an allowlisted schema of entities and fields.
Validation is structural and schema-oriented. It is not a replacement for user
authorization.

```go
err := grokifyql.Validate(q, schema)
```

## Analyze

`Analyze` resolves query sources and fields against the schema and returns the
resources, field usages, aggregate functions, and joins needed by the query.
This is the preferred input for authorization adapters because it describes what
the query actually attempts to use.

```go
analysis, err := grokifyql.Analyze(q, schema)
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
issues := grokifyql.CheckPolicy(q, policy)
```

## Authorize

Authorization decisions should come from trusted application context, such as
tenant membership, RBAC/ReBAC roles, SpiceDB relationships, or service account
scopes.

Authorization may be enforced outside GrokifyQL, or compiled into
`grokifyql.Policy` before `CheckPolicy`.

For relationship-aware systems such as SpiceDB, prefer authorizing the
`RequirementSet` from `Analyze` instead of asking one broad question such as
"can this principal use analytics?"

## Compile

Compile only validated and policy-approved AST nodes. Backend compilers may
target:

- parameterized SQL
- Ent predicates
- API filters
- in-memory filters
- domain-specific query providers

Never compile by concatenating user-authored text.

## Execute

Execution should use service-owned controls:

- tenant scope
- timeouts
- row limits
- rate limits
- audit logs
- resource controls
