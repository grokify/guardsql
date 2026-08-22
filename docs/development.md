# Development

GrokifyQL should stay small and dependency-light.

## Repository Layout

```text
.
├── ast.go
├── lexer.go
├── parser.go
├── schema.go
├── validate.go
├── policy.go
├── lint.go
├── eval.go
├── authz/
└── authzsystemforge/
```

## Module Boundaries

- Root module: `github.com/grokify/grokifyql`
- Optional SystemForge module: `github.com/grokify/grokifyql/authzsystemforge`

Do not add SystemForge, SpiceDB, Cedar, Rego, SQL drivers, Ent, or storage
dependencies to the root module.

## Testing

Run root tests:

```bash
go test ./...
go vet ./...
```

Run browser package tests:

```bash
cd ts
npm test
```

Run optional SystemForge adapter tests separately:

```bash
cd authzsystemforge
go test ./...
go vet ./...
```

## Documentation

Build the docs locally:

```bash
mkdocs build --strict
```

The generated `site/` directory should not be committed.
