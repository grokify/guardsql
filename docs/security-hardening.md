# Security Hardening

GrokifyQL treats customer-authored query text as an untrusted source language.
The engine should accept only the subset it can parse, validate, analyze,
authorize, and compile from trusted AST nodes.

## Enforced in GrokifyQL

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

Use the resolved analysis path for operational systems:

```go
q, err := grokifyql.Parse(input)
if err != nil {
    return err
}
if err := grokifyql.Validate(q, schema); err != nil {
    return err
}
analysis, err := grokifyql.Analyze(q, schema)
if err != nil {
    return err
}
policy := grokifyql.SafeAnalyticsPolicy(schema)
if issues := grokifyql.CheckPolicy(q, policy); len(issues) > 0 {
    return fmt.Errorf("query shape not allowed: %s", issues[0].Message)
}
if issues := grokifyql.CheckAnalysisPolicy(analysis, policy); len(issues) > 0 {
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

The database account remains the final RCE boundary. GrokifyQL reduces the
accepted query language to a policy-approved subset, but it should still run
against a least-privileged backend identity.

