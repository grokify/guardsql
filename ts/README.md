# @grokify/guardsql

Browser-side GuardSQL parser, AST types, schema helpers, and policy checks.

This package is intended for UI feedback: syntax errors, catalog validation,
field warnings, autocomplete metadata, and pre-save checks. The Go parser and
backend policy checks remain authoritative for server-side execution.

```ts
import { parse, checkPolicy } from '@grokify/guardsql'

const query = parse('SELECT id, name FROM roadmap_items LIMIT 25')
const issues = checkPolicy(query, {
  allowedOps: ['read'],
  maxDepth: 8,
  maxNodes: 80,
})
```

Format user-authored query text before display or persistence:

```ts
import { format } from '@grokify/guardsql'

format('select id,name from items limit 10')
// SELECT id, name
// FROM items
// LIMIT 10
```
