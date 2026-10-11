# Getting Started

This walkthrough shows the recommended way to accept untrusted queries and run
trusted mutations end to end. The high-level `Config` runs the full pipeline —
parse, validate, policy, compile — and returns parameterized SQL you hand
straight to `database/sql`.

```bash
go get github.com/grokify/guardsql
```

## 1. Describe the database as a schema

The schema is an allowlist: it defines what may be queried, each column's read
and write capabilities, and its data classification. Capabilities are
deny-by-default — a column grants only what you set.

```go
schema := guardsql.Schema{
    Entities: map[string]guardsql.Entity{
        "events": {Name: "events", Fields: map[string]guardsql.Field{
            "id":        {Type: guardsql.FieldNumber, Selectable: true, Filterable: true, Sortable: true},
            "status":    {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true, Writable: true},
            "tenant_id": {Type: guardsql.FieldNumber, Filterable: true}, // used for scoping, not selectable
            "payload":   {Type: guardsql.FieldJSON, Selectable: true, Sortable: true}, // JSON, addressable by path
            "owner_email": {
                Type: guardsql.FieldString, Selectable: true,
                Class: guardsql.ClassPII, // hidden unless AllowPII
            },
        }},
    },
}
```

## 2. Accept an untrusted (customer) query

`NewUntrustedConfig` is read-only, strips secret/PII columns, locks identifiers,
and requires a `LIMIT`. Add a `RowScope` from trusted session context so the
query can only see the caller's tenant.

```go
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres)
cfg.RowScope = guardsql.ScopeEquals("tenant_id", sessionTenantID) // trusted value
cfg.RequireRowScope = true                                        // fail closed

out, err := cfg.Compile(userQuery)
if err != nil {
    return err // syntax error, disallowed field, missing scope, etc.
}

rows, err := db.Query(out.SQL, out.Args...)
```

Given:

```sql
SELECT payload.type AS type, count(*) AS n FROM events GROUP BY payload.type LIMIT 100
```

`out.SQL` is dialect-specific, parameterized, and tenant-scoped, and
`out.Args` is `["type", 7, "type"]`:

```sql
SELECT jsonb_extract_path_text("payload"::jsonb, $1) AS "type", COUNT(*) AS "n"
FROM "events" WHERE "tenant_id" = $2
GROUP BY jsonb_extract_path_text("payload"::jsonb, $3) LIMIT 100
```

The customer cannot reference `owner_email` (PII), cannot widen past their
tenant, cannot inject via values (bound) or identifiers (allowlisted +
charset-locked), and cannot call arbitrary SQL (only the AST is emitted).

## 3. Run a trusted mutation

`NewTrustedConfig` permits reads and mutations and full column access.
Assignment targets must be `Writable`; UPDATE/DELETE require a `WHERE`; and
`RowScope` still applies, so a mutation cannot cross tenants.

```go
tcfg := guardsql.NewTrustedConfig(schema, guardsql.DialectPostgres)
tcfg.RowScope = guardsql.ScopeEquals("tenant_id", sessionTenantID)

out, err := guardsql.Update("events").
    Set("status", "resolved").
    Where(guardsql.Eq("id", eventID)).
    Compile(tcfg)
// UPDATE "events" SET "status" = $1 WHERE ("id" = $2 AND "tenant_id" = $3)

_, err = db.Exec(out.SQL, out.Args...)
```

## 4. Layer host controls

GuardSQL secures the query language; the deployment still owns the runtime
boundary:

- a read-only, least-privileged database role for untrusted queries
- statement timeouts and server-side row limits
- database row-level security or tenant-scoped views as a final backstop
- audit logging and rate limits

See [Security Hardening](security-hardening.md) for the full checklist and the
[attack coverage table](security-hardening.md#attack-coverage).

## Next Steps

- [Config & Trust Modes](trust-modes.md) — profiles, classification, row scope,
  identifier safety
- [SQL Compiler](compiler.md) — dialects, JSON extraction, guarantees
- [Mutations](mutations.md) — the trusted builder and condition helpers
- [Security Hardening](security-hardening.md) — why it is secure, and what the
  host must still do
