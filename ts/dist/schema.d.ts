import type { AnalyticsCatalog, FieldPolicy, Query, Schema, ValidationIssue } from './types.js';
export declare function schemaFromAnalyticsCatalog(catalog: AnalyticsCatalog, sourceID: string): Schema;
export declare function policyFieldsFromSchema(schema: Schema): Record<string, Record<string, FieldPolicy>>;
export declare function validate(query: Query, schema: Schema): ValidationIssue[];
export declare function referencedFields(query: Query): string[];
//# sourceMappingURL=schema.d.ts.map