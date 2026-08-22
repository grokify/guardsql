# GrokifyQL

GrokifyQL is a small, application-neutral query language for SaaS analytics and
reporting. It gives users a SQL-like interface without passing customer-authored
SQL directly to a database.

The core contract is:

```text
query text -> parser -> AST -> schema validation -> policy -> backend compiler
```

Applications own tenant scope, authorization, execution limits, audit logging,
and backend compilation. GrokifyQL owns parsing, AST types, validation helpers,
policy checks, and a limited in-memory evaluator for simple cases.

This repository is the Go implementation of that engine. Language/runtime
integrations should be separate from the root module. For example, a Hibernate
adapter should live in a Java-oriented repository such as
`github.com/grokify/grokifyql-java` or `github.com/grokify/grokifyql-hibernate`
and consume the same conceptual AST/policy contract without adding Java
dependencies here.

## Design Goals

- Keep customer query text away from direct database execution.
- Make unsupported syntax fail as a normal parse error.
- Validate entities and fields against explicit allowlisted schemas.
- Enforce read-only and field-level policy against the AST.
- Keep the root Go module dependency-light.
- Put heavyweight authorization integrations in optional modules.
- Keep non-Go runtime integrations outside this repository.

## Core Packages

| Package | Purpose |
|---------|---------|
| `github.com/grokify/grokifyql` | Parser, AST, schema validation, policy, evaluator |
| `github.com/grokify/grokifyql/authz` | Lightweight authorization decision helper |
| `github.com/grokify/grokifyql/authzsystemforge` | Optional SystemForge/SpiceDB adapter module |

## Minimal Example

```go
q, err := grokifyql.Parse(`SELECT id, name FROM roadmap_items WHERE status = "planned" LIMIT 25`)
if err != nil {
    return err
}

if err := grokifyql.Validate(q, schema); err != nil {
    return err
}

issues := grokifyql.CheckPolicy(q, grokifyql.Policy{
    AllowedOps: []grokifyql.Operation{grokifyql.OperationRead},
    MaxDepth:   8,
    MaxNodes:   80,
})
if len(issues) > 0 {
    return fmt.Errorf("query not allowed: %s", issues[0].Message)
}
```
