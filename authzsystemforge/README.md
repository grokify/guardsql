# GuardSQL SystemForge Authorization

`authzsystemforge` adapts SystemForge authorization decisions into
`guardsql.Policy`.

The root `github.com/grokify/guardsql` package does not import SystemForge.
Applications opt into this package when they want to use SystemForge providers,
including the SystemForge SpiceDB provider.

Example flow:

```go
provider := spicedb.NewProvider(client) // implements systemforge/authz.Authorizer

policy, err := authzsystemforge.PolicyBuilder{
    Authorizer:   provider,
    Principal:    authz.NewUserPrincipal(userID),
    Schema:       schema,
    RequireLimit: true,
    MaxLimit:     500,
    MaxDepth:     8,
    MaxNodes:     80,
    MaxInValues:  100,
}.Build(ctx)
if err != nil {
    return err
}

q, err := guardsql.Parse(input)
if err != nil {
    return err
}
if issues := guardsql.CheckPolicy(q, policy); len(issues) > 0 {
    return fmt.Errorf("query not allowed: %s", issues[0].Message)
}
```

The default resource builder is intended for examples and simple/mock
authorizers. It checks:

- entity access as resource type `guardsql_entity`, action `read`
- field select access as resource type `guardsql_field`, action `read`
- field filter access as resource type `guardsql_field`, action `list`
- field sort/group access as resource type `guardsql_field`, action `sort`

Applications can provide a custom `ResourceBuilder` to map entities and fields
onto product-specific SystemForge/SpiceDB resources. Production SpiceDB
deployments should normally provide one, because SystemForge's SpiceDB provider
checks concrete resource type/id/permission tuples.

For example, UIForge maps analytics datasets and fields to concrete SpiceDB
resources:

- `analytics_dataset:<deterministic_dataset_uuid>`
- `analytics_field:<deterministic_field_uuid>`

The resource IDs are derived from the analytics source ID, dataset query name,
and field query name. This keeps GuardSQL policy independent of UIForge while
still letting SpiceDB evaluate real relationship tuples.
