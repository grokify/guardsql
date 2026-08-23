import assert from 'node:assert/strict'
import test from 'node:test'
import {
  checkPolicy,
  format,
  highlightANSI,
  parse,
  policyFieldsFromSchema,
  schemaFromAnalyticsCatalog,
  validate,
} from '../dist/index.js'

test('parses and checks a basic query', () => {
  const query = parse('SELECT id, name FROM roadmap_items WHERE status = "planned" ORDER BY name LIMIT 25')
  assert.equal(query.from, 'roadmap_items')
  assert.equal(query.select.length, 2)
  assert.equal(query.orderBy[0]?.field, 'name')
  assert.equal(query.limit, 25)
  assert.deepEqual(checkPolicy(query, { allowedOps: ['read'], maxDepth: 8, maxNodes: 80 }), [])
})

test('formats GuardSQL queries', () => {
  assert.equal(
    format('select name,count(*) as count from initiatives where provider="aha-studio" and workspace_ref="SAVIN" group by name having count > 1 prioritize by moscow_rice order by count desc limit 10'),
    [
      'SELECT name, COUNT(*) AS count',
      'FROM initiatives',
      'WHERE provider = "aha-studio" AND workspace_ref = "SAVIN"',
      'GROUP BY name',
      'HAVING count > 1',
      'PRIORITIZE BY moscow_rice',
      'ORDER BY count DESC',
      'LIMIT 10',
    ].join('\n'),
  )
  assert.equal(
    format('SELECT id,name FROM items WHERE status IN ("planned","active") LIMIT 10', {
      style: 'singleline',
    }),
    'SELECT id, name FROM items WHERE status IN ("planned", "active") LIMIT 10',
  )
})

test('highlights GuardSQL keywords for ANSI terminals', () => {
  const highlighted = highlightANSI('SELECT name FROM items WHERE active = TRUE')
  assert.match(highlighted, /\u001b\[1;36mSELECT\u001b\[0m/)
  assert.match(highlighted, /\u001b\[35mTRUE\u001b\[0m/)
})

test('parses aggregate query', () => {
  const query = parse('SELECT status, COUNT(*) AS count FROM roadmap_items GROUP BY status HAVING count > 1')
  assert.equal(query.select[1]?.aggregate?.func, 'COUNT')
  assert.equal(query.groupBy[0], 'status')
})

test('catalog schema drives validation and policy', () => {
  const schema = schemaFromAnalyticsCatalog(
    {
      id: 'uiforge',
      name: 'UIForge',
      sources: [
        {
          id: 'omniroadmap',
          name: 'OmniRoadmap',
          type: 'application',
          datasets: [
            {
              id: 'items',
              name: 'Items',
              queryName: 'items',
              fields: [
                {
                  id: 'name',
                  name: 'Name',
                  queryName: 'name',
                  type: 'string',
                  selectable: true,
                  filterable: true,
                  sortable: true,
                },
                {
                  id: 'score',
                  name: 'Score',
                  queryName: 'score',
                  type: 'number',
                  selectable: false,
                  filterable: false,
                  sortable: false,
                },
              ],
            },
          ],
        },
      ],
    },
    'omniroadmap',
  )
  const query = parse('SELECT score FROM items')
  assert.deepEqual(validate(query, schema), [])
  assert.equal(
    checkPolicy(query, {
      allowedOps: ['read'],
      fields: policyFieldsFromSchema(schema),
    })[0]?.message,
    'field "score" is not selectable by policy',
  )
})
