export type Operation = 'read';
export interface Query {
    operation: Operation;
    select: SelectItem[];
    from: string;
    fromAlias?: string;
    fromQuery?: Query;
    joins: Join[];
    where?: Expr;
    groupBy: string[];
    having?: Expr;
    prioritizeBy?: string;
    orderBy: Order[];
    limit?: number;
    ctes: CTE[];
}
export interface SelectItem {
    field: string;
    alias?: string;
    star?: boolean;
    aggregate?: Aggregate;
}
export interface Aggregate {
    func: AggregateFunction;
    field: string;
    star?: boolean;
}
export type AggregateFunction = 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX';
export interface CTE {
    name: string;
    query: Query;
}
export interface Join {
    type: JoinType;
    entity: string;
    alias?: string;
    query?: Query;
    on?: Expr;
}
export type JoinType = 'inner' | 'left' | 'right' | 'full' | 'cross';
export interface Order {
    field: string;
    desc?: boolean;
}
export type Expr = CompareExpr | LogicalExpr | NotExpr;
export interface CompareExpr {
    kind: 'compare';
    field: string;
    op: CompareOp;
    value?: Value;
    values?: Value[];
}
export interface LogicalExpr {
    kind: 'logical';
    op: 'AND' | 'OR';
    left: Expr;
    right: Expr;
}
export interface NotExpr {
    kind: 'not';
    expr: Expr;
}
export type CompareOp = '=' | '!=' | '<' | '<=' | '>' | '>=' | 'IN' | 'CONTAINS' | 'IS_NULL' | 'IS_NOT_NULL';
export type Value = string | number | boolean | null;
export interface Schema {
    entities: Record<string, Entity>;
    maxLimit?: number;
}
export interface Entity {
    name?: string;
    fields: Record<string, Field>;
}
export interface Field {
    name?: string;
    type: FieldType | string;
    selectable?: boolean;
    filterable?: boolean;
    sortable?: boolean;
}
export type FieldType = 'string' | 'number' | 'bool' | 'time';
export interface Policy {
    allowedOps?: Operation[];
    allowedEntities?: string[];
    fields?: Record<string, Record<string, FieldPolicy>>;
    allowStar?: boolean;
    requireLimit?: boolean;
    maxLimit?: number;
    maxDepth?: number;
    maxNodes?: number;
    maxInValues?: number;
}
export interface FieldPolicy {
    selectable?: boolean;
    filterable?: boolean;
    sortable?: boolean;
}
export interface PolicyIssue {
    message: string;
}
export interface ValidationIssue {
    message: string;
}
export interface AnalyticsCatalog {
    id: string;
    name: string;
    sources: AnalyticsSource[];
}
export interface AnalyticsSource {
    id: string;
    name: string;
    type: string;
    datasets: AnalyticsDataset[];
}
export interface AnalyticsDataset {
    id: string;
    name: string;
    queryName: string;
    fields: AnalyticsField[];
}
export interface AnalyticsField {
    id: string;
    name: string;
    queryName: string;
    type: string;
    selectable: boolean;
    filterable: boolean;
    sortable: boolean;
}
//# sourceMappingURL=types.d.ts.map