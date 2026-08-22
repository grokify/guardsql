import type { Expr, Policy, PolicyIssue, Query } from './types.js'

export function checkPolicy(query: Query, policy: Policy): PolicyIssue[] {
  const issues: PolicyIssue[] = []
  if (policy.allowedOps?.length && !policy.allowedOps.includes(query.operation)) {
    issues.push({ message: `operation ${JSON.stringify(query.operation)} is not allowed` })
  }
  if (policy.allowedEntities?.length && !stringAllowed(query.from, policy.allowedEntities)) {
    issues.push({ message: `entity ${JSON.stringify(query.from)} is not allowed` })
  }
  if (policy.fields && Object.keys(policy.fields).length > 0) {
    issues.push(...checkFieldPolicy(query, policy))
  }
  if (policy.requireLimit && query.limit === undefined) {
    issues.push({ message: 'LIMIT is required' })
  }
  if (policy.maxLimit && (query.limit === undefined || query.limit > policy.maxLimit)) {
    issues.push({ message: `LIMIT exceeds maximum ${policy.maxLimit}` })
  }
  if (policy.maxDepth && exprDepth(query.where) > policy.maxDepth) {
    issues.push({ message: `WHERE expression depth exceeds maximum ${policy.maxDepth}` })
  }
  if (policy.maxNodes && exprNodes(query.where) > policy.maxNodes) {
    issues.push({ message: `WHERE expression node count exceeds maximum ${policy.maxNodes}` })
  }
  if (policy.maxInValues) {
    walkExpr(query.where, (expr) => {
      if (expr.kind === 'compare' && expr.op === 'IN' && (expr.values?.length ?? 0) > policy.maxInValues!) {
        issues.push({ message: `IN value count exceeds maximum ${policy.maxInValues}` })
      }
    })
  }
  return issues
}

function checkFieldPolicy(query: Query, policy: Policy): PolicyIssue[] {
  const fieldPolicies = policy.fields?.[query.from.toLowerCase()]
  if (!fieldPolicies) return []
  const issues: PolicyIssue[] = []
  for (const item of query.select) {
    if (item.star) {
      if (!policy.allowStar) issues.push({ message: 'SELECT * is not allowed when field policy is configured' })
      continue
    }
    const field = item.aggregate?.star ? undefined : item.aggregate?.field ?? item.field
    if (field && !fieldPolicies[field.toLowerCase()]?.selectable) {
      issues.push({ message: `field ${JSON.stringify(field)} is not selectable by policy` })
    }
  }
  for (const field of query.groupBy) {
    if (!fieldPolicies[field.toLowerCase()]?.sortable) {
      issues.push({ message: `field ${JSON.stringify(field)} is not groupable by policy` })
    }
  }
  walkExpr(query.where, (expr) => {
    if (expr.kind === 'compare' && !fieldPolicies[expr.field.toLowerCase()]?.filterable) {
      issues.push({ message: `field ${JSON.stringify(expr.field)} is not filterable by policy` })
    }
  })
  for (const order of query.orderBy) {
    if (!fieldPolicies[order.field.toLowerCase()]?.sortable) {
      issues.push({ message: `field ${JSON.stringify(order.field)} is not sortable by policy` })
    }
  }
  return issues
}

function stringAllowed(value: string, allowed: string[]) {
  return allowed.some((candidate) => candidate.toLowerCase() === value.toLowerCase())
}

function exprDepth(expr?: Expr): number {
  if (!expr) return 0
  if (expr.kind === 'logical') return 1 + Math.max(exprDepth(expr.left), exprDepth(expr.right))
  if (expr.kind === 'not') return 1 + exprDepth(expr.expr)
  return 1
}

function exprNodes(expr?: Expr): number {
  if (!expr) return 0
  if (expr.kind === 'logical') return 1 + exprNodes(expr.left) + exprNodes(expr.right)
  if (expr.kind === 'not') return 1 + exprNodes(expr.expr)
  return 1
}

function walkExpr(expr: Expr | undefined, fn: (expr: Expr) => void) {
  if (!expr) return
  fn(expr)
  if (expr.kind === 'logical') {
    walkExpr(expr.left, fn)
    walkExpr(expr.right, fn)
  } else if (expr.kind === 'not') {
    walkExpr(expr.expr, fn)
  }
}
