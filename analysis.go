package grokifyql

import (
	"fmt"
	"sort"
	"strings"
)

// FieldUsage describes how a query uses a field.
type FieldUsage string

const (
	FieldUsageSelect    FieldUsage = "select"
	FieldUsageFilter    FieldUsage = "filter"
	FieldUsageSort      FieldUsage = "sort"
	FieldUsageGroup     FieldUsage = "group"
	FieldUsageAggregate FieldUsage = "aggregate"
	FieldUsageJoin      FieldUsage = "join"
	FieldUsageHaving    FieldUsage = "having"
)

// Analysis is the resolved, read-only summary of a parsed query.
type Analysis struct {
	Operation     Operation
	Sources       []ResolvedSource
	Fields        []FieldRequirement
	Joins         []JoinRequirement
	Functions     []AggregateFunc
	CTEs          []Analysis
	NestedSources []Analysis
	HasAggregate  bool
}

// RequirementSet is a compact authorization and policy input derived from
// Analysis.
type RequirementSet struct {
	Operations []Operation
	Entities   []string
	Fields     []FieldRequirement
	Joins      []JoinRequirement
	Functions  []AggregateFunc
}

// Requirements returns a de-duplicated requirement set for authorization and
// policy adapters.
func (a Analysis) Requirements() RequirementSet {
	entitySet := map[string]struct{}{}
	fieldSet := map[string]FieldRequirement{}
	joinSet := map[string]JoinRequirement{}
	funcSet := map[AggregateFunc]struct{}{}
	for _, source := range a.Sources {
		entitySet[source.Entity] = struct{}{}
	}
	for _, field := range a.Fields {
		key := strings.Join([]string{
			field.Entity,
			field.Field,
			string(field.Usage),
			string(field.Aggregate),
			field.OutputName,
		}, "\x00")
		fieldSet[key] = field
	}
	for _, join := range a.Joins {
		key := strings.Join([]string{string(join.Type), join.LeftEntity, join.RightEntity}, "\x00")
		joinSet[key] = join
	}
	for _, fn := range a.Functions {
		funcSet[fn] = struct{}{}
	}
	for _, child := range append(append([]Analysis{}, a.CTEs...), a.NestedSources...) {
		reqs := child.Requirements()
		for _, entity := range reqs.Entities {
			entitySet[entity] = struct{}{}
		}
		for _, field := range reqs.Fields {
			key := strings.Join([]string{
				field.Entity,
				field.Field,
				string(field.Usage),
				string(field.Aggregate),
				field.OutputName,
			}, "\x00")
			fieldSet[key] = field
		}
		for _, join := range reqs.Joins {
			key := strings.Join([]string{string(join.Type), join.LeftEntity, join.RightEntity}, "\x00")
			joinSet[key] = join
		}
		for _, fn := range reqs.Functions {
			funcSet[fn] = struct{}{}
		}
	}
	return RequirementSet{
		Operations: []Operation{a.Operation},
		Entities:   sortedKeys(entitySet),
		Fields:     sortedValues(fieldSet),
		Joins:      sortedJoinValues(joinSet),
		Functions:  sortedAggregateFuncs(funcSet),
	}
}

// ResolvedSource identifies a query source after schema and CTE resolution.
type ResolvedSource struct {
	Name     string
	Alias    string
	Entity   string
	Nested   bool
	CTE      bool
	JoinType JoinType
}

// FieldRequirement describes a concrete field operation in a query.
type FieldRequirement struct {
	Entity     string
	Field      string
	Source     string
	Usage      FieldUsage
	Aggregate  AggregateFunc
	OutputName string
}

// JoinRequirement describes a source-to-source join used by a query.
type JoinRequirement struct {
	Type        JoinType
	LeftEntity  string
	RightEntity string
	RightSource string
}

// Analyze resolves a parsed query against a schema and returns the resources,
// fields, functions, and joins the query requires. It does not mutate the query.
func Analyze(q *Query, schema Schema) (Analysis, error) {
	if q == nil {
		return Analysis{}, fmt.Errorf("query is nil")
	}
	ctx := analysisContext{schema: schema.Normalize()}
	return ctx.analyze(q, nil)
}

type analysisContext struct {
	schema Schema
}

type sourceBinding struct {
	name   string
	alias  string
	entity Entity
	nested bool
	cte    bool
	join   JoinType
}

func (ctx analysisContext) analyze(q *Query, inheritedCTEs map[string]Entity) (Analysis, error) {
	analysis := Analysis{Operation: q.Operation}
	cteEntities := map[string]Entity{}
	for name, ent := range inheritedCTEs {
		cteEntities[name] = ent
	}
	for _, cte := range q.With {
		if cte.Query == nil {
			return Analysis{}, fmt.Errorf("cte %q query is nil", cte.Name)
		}
		child, err := ctx.analyze(cte.Query, inheritedCTEs)
		if err != nil {
			return Analysis{}, fmt.Errorf("cte %q: %w", cte.Name, err)
		}
		analysis.CTEs = append(analysis.CTEs, child)
		cteEntities[strings.ToLower(cte.Name)] = outputEntity(cte.Query).NormalizeName(cte.Name)
	}

	scope := sourceScope{byName: map[string]sourceBinding{}}
	base, nestedAnalyses, err := ctx.resolveBaseSource(q, cteEntities)
	if err != nil {
		return Analysis{}, err
	}
	analysis.NestedSources = append(analysis.NestedSources, nestedAnalyses...)
	if err := scope.add(base); err != nil {
		return Analysis{}, err
	}
	analysis.Sources = append(analysis.Sources, resolvedSource(base))

	for _, join := range q.Joins {
		source, nested, err := ctx.resolveJoinSource(join, cteEntities)
		if err != nil {
			return Analysis{}, err
		}
		analysis.NestedSources = append(analysis.NestedSources, nested...)
		if err := scope.add(source); err != nil {
			return Analysis{}, err
		}
		analysis.Sources = append(analysis.Sources, resolvedSource(source))
		analysis.Joins = append(analysis.Joins, JoinRequirement{
			Type:        join.Type,
			LeftEntity:  base.entity.Name,
			RightEntity: source.entity.Name,
			RightSource: source.displayName(),
		})
		joinFields, err := analyzeExprFields(join.Condition, scope, FieldUsageJoin)
		if err != nil {
			return Analysis{}, fmt.Errorf("join %q: %w", source.displayName(), err)
		}
		analysis.Fields = append(analysis.Fields, joinFields...)
	}

	fields, funcs, hasAggregate, err := analyzeSelectFields(q.Select, scope)
	if err != nil {
		return Analysis{}, err
	}
	analysis.Fields = append(analysis.Fields, fields...)
	analysis.Functions = append(analysis.Functions, funcs...)
	analysis.HasAggregate = hasAggregate

	groupFields, err := analyzeNamedFields(q.GroupBy, scope, FieldUsageGroup)
	if err != nil {
		return Analysis{}, err
	}
	analysis.Fields = append(analysis.Fields, groupFields...)

	whereFields, err := analyzeExprFields(q.Where, scope, FieldUsageFilter)
	if err != nil {
		return Analysis{}, err
	}
	analysis.Fields = append(analysis.Fields, whereFields...)

	if q.Having != nil {
		havingScope := outputScope(q)
		havingFields, err := analyzeExprFields(q.Having, havingScope, FieldUsageHaving)
		if err != nil {
			return Analysis{}, fmt.Errorf("having: %w", err)
		}
		analysis.Fields = append(analysis.Fields, havingFields...)
	}

	orderScope := scope
	if hasAggregate || len(q.GroupBy) > 0 {
		orderScope = outputScope(q)
	}
	orderNames := make([]string, 0, len(q.OrderBy))
	for _, order := range q.OrderBy {
		orderNames = append(orderNames, order.Field)
	}
	orderFields, err := analyzeNamedFields(orderNames, orderScope, FieldUsageSort)
	if err != nil {
		return Analysis{}, err
	}
	analysis.Fields = append(analysis.Fields, orderFields...)

	return analysis, nil
}

func (ctx analysisContext) resolveBaseSource(q *Query, ctes map[string]Entity) (sourceBinding, []Analysis, error) {
	if q.FromQuery != nil {
		if strings.TrimSpace(q.FromAlias) == "" {
			return sourceBinding{}, nil, fmt.Errorf("nested FROM query requires an alias")
		}
		nested, err := ctx.analyze(q.FromQuery, ctes)
		if err != nil {
			return sourceBinding{}, nil, fmt.Errorf("nested FROM query: %w", err)
		}
		ent := outputEntity(q.FromQuery).NormalizeName(q.FromAlias)
		return sourceBinding{name: q.FromAlias, alias: q.FromAlias, entity: ent, nested: true}, []Analysis{nested}, nil
	}
	ent, cte, ok := ctx.lookupEntity(q.From, ctes)
	if !ok {
		return sourceBinding{}, nil, fmt.Errorf("unknown entity %q", q.From)
	}
	return sourceBinding{name: q.From, alias: q.FromAlias, entity: ent, cte: cte}, nil, nil
}

func (ctx analysisContext) resolveJoinSource(join Join, ctes map[string]Entity) (sourceBinding, []Analysis, error) {
	if join.Query != nil {
		if strings.TrimSpace(join.Alias) == "" {
			return sourceBinding{}, nil, fmt.Errorf("joined nested query requires an alias")
		}
		nested, err := ctx.analyze(join.Query, ctes)
		if err != nil {
			return sourceBinding{}, nil, fmt.Errorf("join query: %w", err)
		}
		ent := outputEntity(join.Query).NormalizeName(join.Alias)
		return sourceBinding{name: join.Alias, alias: join.Alias, entity: ent, nested: true, join: join.Type}, []Analysis{nested}, nil
	}
	ent, cte, ok := ctx.lookupEntity(join.Entity, ctes)
	if !ok {
		return sourceBinding{}, nil, fmt.Errorf("unknown join entity %q", join.Entity)
	}
	return sourceBinding{name: join.Entity, alias: join.Alias, entity: ent, cte: cte, join: join.Type}, nil, nil
}

func (ctx analysisContext) lookupEntity(name string, ctes map[string]Entity) (Entity, bool, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if ent, ok := ctx.schema.Entities[key]; ok {
		if ent.Name == "" {
			ent.Name = key
		}
		return ent, false, true
	}
	if ent, ok := ctes[key]; ok {
		return ent, true, true
	}
	return Entity{}, false, false
}

type sourceScope struct {
	sources []sourceBinding
	byName  map[string]sourceBinding
}

func (s *sourceScope) add(source sourceBinding) error {
	if s.byName == nil {
		s.byName = map[string]sourceBinding{}
	}
	if source.alias != "" {
		if err := s.bindKey(source.alias, source); err != nil {
			return err
		}
		s.sources = append(s.sources, source)
		return nil
	}
	if err := s.bindKey(source.name, source); err != nil {
		return err
	}
	s.sources = append(s.sources, source)
	return nil
}

func (s *sourceScope) bindKey(key string, source sourceBinding) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return nil
	}
	if _, exists := s.byName[key]; exists {
		return fmt.Errorf("duplicate source alias %q", key)
	}
	s.byName[key] = source
	return nil
}

func (s sourceScope) resolveField(name string) (FieldRequirement, error) {
	raw := strings.ToLower(strings.TrimSpace(name))
	if raw == "" {
		return FieldRequirement{}, fmt.Errorf("empty field name")
	}
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		sourceName := raw[:idx]
		fieldName := raw[idx+1:]
		source, ok := s.byName[sourceName]
		if !ok {
			return FieldRequirement{}, fmt.Errorf("unknown source %q for field %q", sourceName, name)
		}
		field, ok := source.entity.Fields[fieldName]
		if !ok {
			return FieldRequirement{}, fmt.Errorf("unknown field %q on source %q", fieldName, sourceName)
		}
		return fieldRequirement(source, field), nil
	}
	var matches []FieldRequirement
	for _, source := range s.sources {
		if field, ok := source.entity.Fields[raw]; ok {
			matches = append(matches, fieldRequirement(source, field))
		}
	}
	switch len(matches) {
	case 0:
		return FieldRequirement{}, fmt.Errorf("unknown field %q", name)
	case 1:
		return matches[0], nil
	default:
		return FieldRequirement{}, fmt.Errorf("ambiguous field %q", name)
	}
}

func analyzeSelectFields(items []SelectItem, scope sourceScope) ([]FieldRequirement, []AggregateFunc, bool, error) {
	var fields []FieldRequirement
	var funcs []AggregateFunc
	var hasAggregate bool
	for _, item := range items {
		if item.Star {
			for _, source := range scope.sources {
				for _, field := range source.entity.Fields {
					req := fieldRequirement(source, field)
					req.Usage = FieldUsageSelect
					req.OutputName = field.Name
					fields = append(fields, req)
				}
			}
			continue
		}
		if item.Aggregate != nil {
			hasAggregate = true
			funcs = append(funcs, item.Aggregate.Func)
			if item.Aggregate.Star {
				continue
			}
			req, err := scope.resolveField(item.Aggregate.Field)
			if err != nil {
				return nil, nil, false, fmt.Errorf("aggregate field %q: %w", item.Aggregate.Field, err)
			}
			req.Usage = FieldUsageAggregate
			req.Aggregate = item.Aggregate.Func
			req.OutputName = outputName(item)
			fields = append(fields, req)
			continue
		}
		req, err := scope.resolveField(item.Field)
		if err != nil {
			return nil, nil, false, fmt.Errorf("select field %q: %w", item.Field, err)
		}
		req.Usage = FieldUsageSelect
		req.OutputName = outputName(item)
		fields = append(fields, req)
	}
	return fields, funcs, hasAggregate, nil
}

func analyzeNamedFields(names []string, scope sourceScope, usage FieldUsage) ([]FieldRequirement, error) {
	fields := make([]FieldRequirement, 0, len(names))
	for _, name := range names {
		req, err := scope.resolveField(name)
		if err != nil {
			return nil, fmt.Errorf("%s field %q: %w", usage, name, err)
		}
		req.Usage = usage
		fields = append(fields, req)
	}
	return fields, nil
}

func analyzeExprFields(expr Expr, scope sourceScope, usage FieldUsage) ([]FieldRequirement, error) {
	var fields []FieldRequirement
	var firstErr error
	walkExpr(expr, func(expr Expr) {
		if firstErr != nil {
			return
		}
		cmp, ok := expr.(*CompareExpr)
		if !ok {
			return
		}
		req, err := scope.resolveField(cmp.Field)
		if err != nil {
			firstErr = err
			return
		}
		req.Usage = usage
		fields = append(fields, req)
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return fields, nil
}

func outputScope(q *Query) sourceScope {
	source := sourceBinding{name: "output", entity: outputEntity(q).WithName("output")}
	scope := sourceScope{byName: map[string]sourceBinding{}}
	_ = scope.add(source)
	return scope
}

func fieldRequirement(source sourceBinding, field Field) FieldRequirement {
	return FieldRequirement{
		Entity: source.entity.Name,
		Field:  field.Name,
		Source: source.displayName(),
	}
}

func resolvedSource(source sourceBinding) ResolvedSource {
	return ResolvedSource{
		Name:     source.name,
		Alias:    source.alias,
		Entity:   source.entity.Name,
		Nested:   source.nested,
		CTE:      source.cte,
		JoinType: source.join,
	}
}

func (s sourceBinding) displayName() string {
	if s.alias != "" {
		return s.alias
	}
	return s.name
}

func (e Entity) NormalizeName(fallback string) Entity {
	if e.Name == "" {
		e.Name = fallback
	}
	e.Name = strings.ToLower(e.Name)
	return e
}

func (e Entity) WithName(name string) Entity {
	e.Name = strings.ToLower(name)
	return e
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedValues(values map[string]FieldRequirement) []FieldRequirement {
	out := make([]FieldRequirement, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entity != out[j].Entity {
			return out[i].Entity < out[j].Entity
		}
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		if out[i].Usage != out[j].Usage {
			return out[i].Usage < out[j].Usage
		}
		return out[i].OutputName < out[j].OutputName
	})
	return out
}

func sortedJoinValues(values map[string]JoinRequirement) []JoinRequirement {
	out := make([]JoinRequirement, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LeftEntity != out[j].LeftEntity {
			return out[i].LeftEntity < out[j].LeftEntity
		}
		if out[i].RightEntity != out[j].RightEntity {
			return out[i].RightEntity < out[j].RightEntity
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func sortedAggregateFuncs(values map[AggregateFunc]struct{}) []AggregateFunc {
	out := make([]AggregateFunc, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
