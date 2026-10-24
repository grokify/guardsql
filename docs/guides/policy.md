# Policy

`guardsql.Policy` applies AST-level safety rules. It is designed for structural
query controls and for compiled authorization decisions.

For new authorization integrations, call `Analyze` first and authorize the
resulting `RequirementSet`. `CheckPolicy` remains the compatibility API for
enforcing the current AST-level allowlist.

## Policy Shape

```go
policy := guardsql.Policy{
    AllowedOps:       []guardsql.Operation{guardsql.OperationRead},
    AllowedEntities:  []string{"roadmap_items"},
    AllowedJoinTypes: []guardsql.JoinType{guardsql.JoinInner, guardsql.JoinLeft},
    AllowedFunctions: []guardsql.AggregateFunc{guardsql.AggCount, guardsql.AggSum},
    Fields: map[string]map[string]guardsql.FieldPolicy{
        "roadmap_items": {
            "name":   {Selectable: true, Filterable: true, Sortable: true, Groupable: true},
            "score":  {Selectable: true, Filterable: true, Sortable: true, Groupable: true, Aggregatable: true},
            "status": {Selectable: true, Filterable: true, Sortable: true, Groupable: true},
        },
    },
    RequireLimit:      true,
    MaxLimit:          1000,
    MaxDepth:          8,
    MaxNodes:          80,
    MaxInValues:       100,
    MaxSelectItems:    50,
    MaxOrderFields:    5,
    MaxGroupFields:    5,
    MaxJoins:          3,
    MaxSubqueryDepth:  0,
    AllowCTEs:         false,
    AllowNestedSources: false,
}
```

## Empty Allowlists

By default an empty `AllowedEntities` or `Fields` adds no restriction, so a zero
`Policy` (for example `Config.Policy` left unset) applies only the limits that
are set. Set `StrictAllowlists` when the policy is an authorization result:
`AllowedEntities` and `Fields` then become strict allowlists even when empty, so
an unlisted entity is refused and an entity without a `Fields` entry has no
usable fields. `authz.PolicyFromDecision` always sets it.

`AllowedEntities` applies to joined entities as well as the `FROM` entity.

For a generated deny-by-default starting point:

```go
policy := guardsql.SafeAnalyticsPolicy(schema)
```

## Read-Only Analytics

Analytics endpoints should normally set:

```go
AllowedOps: []guardsql.Operation{guardsql.OperationRead}
```

Future mutation operations should remain behind separate APIs, explicit
policies, permission checks, and audit logs.

## Field Policy

When `Policy.Fields` is configured for an entity, field usage becomes
deny-by-default for that entity:

- selected fields require `Selectable`
- aggregate input fields require `Aggregatable` or an allowed
  `AggregateFuncs` entry
- `WHERE` fields require `Filterable`
- `ORDER BY` fields require `Sortable`
- `GROUP BY` fields require `Groupable`, with `Sortable` accepted for backward
  compatibility
- join predicate fields require `Joinable` when using `CheckAnalysisPolicy`

`SELECT *` is rejected when field policy is configured unless `AllowStar` is
true.

## Structural Policy

Policy can deny or limit:

- aggregate functions with `AllowedFunctions`
- join types with `AllowedJoinTypes`
- CTEs with `AllowCTEs`
- nested query sources with `AllowNestedSources`
- select item count with `MaxSelectItems`
- order fields with `MaxOrderFields`
- group fields with `MaxGroupFields`
- joins with `MaxJoins`
- CTEs with `MaxCTEs`
- nested query depth with `MaxSubqueryDepth`

## Limits

Policy can enforce:

- `RequireLimit`
- `MaxLimit`
- `MaxDepth`
- `MaxNodes`
- `MaxInValues`
- `MaxSelectItems`
- `MaxOrderFields`
- `MaxGroupFields`
- `MaxJoins`
- `MaxCTEs`
- `MaxSubqueryDepth`

`MaxLimit` currently rejects queries with no `LIMIT`. If a host application
wants to allow omitted limits while still applying service-side row caps, it
should leave `MaxLimit` unset and enforce the cap in the backend execution
layer.

## Persisted ASTs

Saving a parsed AST or compiled query metadata can help with audit, query
fingerprinting, and repeat execution. It does not replace policy checks.

Always re-check current policy before execution because user roles,
relationships, field permissions, and tenant configuration can change after a
query is saved.

## Analysis Requirements

`Analyze` separates semantic requirement extraction from policy enforcement.
This is the first step toward distinct parsed, resolved, authorized, and
executable query states.

```go
analysis, err := guardsql.Analyze(q, schema)
if err != nil {
    return err
}

for _, field := range analysis.Requirements().Fields {
    // Authorize field.Entity, field.Field, and field.Usage.
}
```

Use this layer when mapping GuardSQL to external authorization systems because
it records how fields are used, not just whether they appear in a projection.

```go
issues := guardsql.CheckAnalysisPolicy(analysis, policy)
```

## Example Policies

See `examples/customer_analytics_policy.yaml` for a human-readable fail-closed
analytics profile. The YAML is documentation-oriented; Go callers should build
equivalent `Policy` values directly.
