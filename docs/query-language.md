# Query Language

GrokifyQL is SQL-like, but it is not SQL. It parses to a purpose-built AST that
applications validate and compile into backend-specific execution plans.

## Basic Query

```sql
SELECT id, name, priority
FROM roadmap_items
WHERE status IN ("planned", "in_progress") AND score >= 50
ORDER BY score DESC
LIMIT 25
```

`SELECT` is optional. A query without `SELECT` returns all selectable fields
allowed by the backend and policy:

```sql
FROM roadmap_items WHERE name CONTAINS "roadmap" LIMIT 10
```

## Clauses

Supported clauses:

- `SELECT ... FROM ...`
- `WHERE`
- `GROUP BY`
- `HAVING`
- `ORDER BY`
- `LIMIT`
- `WITH` common table expressions
- joins
- nested query sources in `FROM (...)`

## WHERE Operators

Supported `WHERE` operators:

- `=`
- `!=`
- `<`
- `<=`
- `>`
- `>=`
- `IN`
- `CONTAINS`
- `IS NULL`
- `IS NOT NULL`

Boolean composition supports `AND`, `OR`, `NOT`, and parentheses.

## Aggregates

Supported aggregate functions:

- `COUNT(*)`
- `COUNT(field)`
- `SUM(field)`
- `AVG(field)`
- `MIN(field)`
- `MAX(field)`

Example:

```sql
SELECT status, COUNT(*) AS count, AVG(score) AS avg_score
FROM roadmap_items
GROUP BY status
HAVING count > 1
ORDER BY avg_score DESC
LIMIT 10
```

In aggregate queries, non-aggregate selected fields must appear in `GROUP BY`.

## Joins

Supported join syntax:

- `JOIN`
- `INNER JOIN`
- `LEFT JOIN`
- `LEFT OUTER JOIN`
- `RIGHT JOIN`
- `RIGHT OUTER JOIN`
- `FULL JOIN`
- `FULL OUTER JOIN`
- `CROSS JOIN`

Example:

```sql
SELECT parent.id, child.id
FROM roadmap_items AS parent
JOIN roadmap_items AS child ON parent.id = child.parent_id
LIMIT 25
```

Self joins require aliases on both sources. Duplicate aliases are rejected.

## CTEs and Nested Sources

GrokifyQL can represent CTEs and nested sources in the AST:

```sql
WITH active AS (
  SELECT id, status FROM roadmap_items WHERE status = "active" LIMIT 100
)
SELECT active.id
FROM (SELECT id, status FROM active LIMIT 50) AS active
JOIN roadmap_items AS base ON active.id = base.id
LIMIT 10
```

Nested sources require aliases.

## Execution Boundary

The in-memory evaluator supports single-source filters, grouping, aggregates,
`HAVING`, ordering, limits, and projection.

Joins, CTEs, and nested sources are parsed and validated as AST features, but
they should be executed by backend-specific compilers.

## Formatting

Use `grokifyql.Format` or `grokifyqlfmt` to normalize user-authored query text.
The formatter parses the query first, then emits canonical GrokifyQL from the
AST.

Multiline output:

```sql
SELECT workspace_ref, name, custom.product
FROM initiatives
WHERE provider = "aha-studio" AND workspace_ref = "SAVIN"
PRIORITIZE BY moscow_rice
LIMIT 100
```

Single-line output:

```sql
SELECT workspace_ref, name, custom.product FROM initiatives WHERE provider = "aha-studio" AND workspace_ref = "SAVIN" PRIORITIZE BY moscow_rice LIMIT 100
```

CLI examples:

```bash
grokifyqlfmt query.gql
grokifyqlfmt --style=singleline query.gql
grokifyqlfmt --highlight=ansi query.gql
```

The Go and TypeScript APIs also support optional ANSI syntax highlighting for
terminal display. Browser editors should generally use token spans/classes from
their editor component rather than ANSI escape codes.
