# Authorization

The root GuardSQL module does not import concrete authorization engines. It
defines AST and policy primitives. Host applications decide how to authorize
principals and may compile those decisions into `guardsql.Policy`.

## Lightweight Decision Helper

The `authz` package defines a small decision shape:

```go
decision := authz.Decision{
    AllowedEntities: []string{"roadmap_items"},
    Fields: map[string]map[string]guardsql.FieldPolicy{
        "roadmap_items": {
            "name": {Selectable: true, Filterable: true, Sortable: true},
        },
    },
    MaxDepth:    8,
    MaxNodes:    80,
    MaxInValues: 100,
}

policy := authz.PolicyFromDecision(decision)
```

External engines should produce decisions from trusted application context, not
from user-authored query text.

A decision's allowlists are strict: `PolicyFromDecision` sets
`Policy.StrictAllowlists`, so a decision with no `AllowedEntities` denies every
query, and an allowed entity without a `Fields` entry exposes no fields. An
authorization result that grants nothing therefore denies rather than leaving
the query unrestricted.

## SystemForge and SpiceDB

SystemForge's adapter lives in SystemForge, which imports GuardSQL (GuardSQL
never imports SystemForge):

```bash
go get github.com/plexusone/systemforge/authzguardsql
```

Example:

```go
policy, err := authzguardsql.PolicyBuilder{
    Authorizer: provider, // systemforge/authz.Authorizer
    Principal:  authz.NewUserPrincipal(userID),
    Schema:     schema,
    ResourceBuilder: myResourceBuilder,
    MaxDepth:   8,
    MaxNodes:   80,
}.Build(ctx)
```

The adapter checks:

| GuardSQL use | SystemForge action |
|---------------|--------------------|
| Entity read | `read` |
| Select field | `read` |
| Filter field | `list` |
| Sort/group field | `sort` |

## Resource Mapping

Production SpiceDB deployments should provide a custom resource builder so
query entities and fields map to concrete product resources:

```text
analytics_dataset:<dataset_id>
analytics_field:<field_id>
```

Attribute-only resources are useful for tests and examples, but SystemForge's
SpiceDB provider checks concrete type/id/permission tuples.

## Other Engines

Cedar, Rego, SpiceDB, Casbin, or custom RBAC engines can all be used by
compiling their decisions into `guardsql.Policy`.

Keep heavyweight engine dependencies outside the root module. Use optional
subpackages or nested modules when an integration needs non-trivial dependency
graphs.
