export function schemaFromAnalyticsCatalog(catalog, sourceID) {
    const source = catalog.sources.find((candidate) => eq(candidate.id, sourceID));
    if (!source)
        return { entities: {} };
    const entities = {};
    for (const dataset of source.datasets) {
        const entityName = dataset.queryName || dataset.id;
        const fields = {};
        for (const field of dataset.fields) {
            const fieldName = field.queryName || field.id;
            fields[fieldName] = {
                name: fieldName,
                type: field.type === 'date' ? 'time' : field.type,
                selectable: field.selectable,
                filterable: field.filterable,
                sortable: field.sortable,
            };
        }
        entities[entityName] = { name: entityName, fields };
    }
    return { entities };
}
export function policyFieldsFromSchema(schema) {
    const out = {};
    for (const [entityName, entity] of Object.entries(schema.entities)) {
        out[entityName.toLowerCase()] = {};
        for (const [fieldName, field] of Object.entries(entity.fields)) {
            out[entityName.toLowerCase()][fieldName.toLowerCase()] = {
                selectable: field.selectable,
                filterable: field.filterable,
                sortable: field.sortable,
            };
        }
    }
    return out;
}
export function validate(query, schema) {
    const issues = [];
    const entity = schema.entities[query.from] ?? schema.entities[query.from.toLowerCase()];
    if (!entity)
        return [{ message: `unknown entity ${JSON.stringify(query.from)}` }];
    for (const field of referencedFields(query)) {
        if (!lookupField(entity.fields, field))
            issues.push({ message: `unknown field ${JSON.stringify(field)}` });
    }
    return issues;
}
export function referencedFields(query) {
    const fields = new Set();
    for (const item of query.select) {
        if (item.star || item.aggregate?.star)
            continue;
        fields.add(item.aggregate?.field ?? item.field);
    }
    for (const field of query.groupBy)
        fields.add(field);
    for (const order of query.orderBy)
        fields.add(order.field);
    walkQueryExpr(query, (field) => fields.add(field));
    return [...fields].filter(Boolean);
}
function walkQueryExpr(query, add) {
    const walk = (expr) => {
        if (!expr)
            return;
        if (expr.kind === 'compare')
            add(expr.field);
        else if (expr.kind === 'logical') {
            walk(expr.left);
            walk(expr.right);
        }
        else {
            walk(expr.expr);
        }
    };
    walk(query.where);
    walk(query.having);
}
function lookupField(fields, name) {
    return fields[name] ?? fields[name.toLowerCase()];
}
function eq(a, b) {
    return a.toLowerCase() === b.toLowerCase();
}
//# sourceMappingURL=schema.js.map