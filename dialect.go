package guardsql

import (
	"fmt"
	"strconv"
	"strings"
)

// dialect abstracts the SQL-emission differences between backends. It is
// internal: callers select a dialect via CompileOptions.Dialect.
type dialect interface {
	// quoteIdent quotes an identifier, escaping the quote character.
	quoteIdent(s string) string
	// placeholder returns the bind placeholder for the n-th (1-based) parameter.
	placeholder(n int) string
	// jsonExtractText returns an SQL expression extracting the text value at
	// path from the already-quoted JSON column expression col, binding any path
	// parameters via bind.
	jsonExtractText(col string, path []string, bind func(any) string) string
	// castNumber and castBool wrap a JSON-extracted text expression so it
	// compares and aggregates as a number or boolean. Dialects whose extraction
	// already yields a coercible value return expr unchanged.
	castNumber(expr string) string
	castBool(expr string) string
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectSQLite:
		return sqliteDialect{}, nil
	case DialectPostgres:
		return postgresDialect{}, nil
	case DialectMySQL:
		return mysqlDialect{}, nil
	case "":
		return nil, fmt.Errorf("guardsql: dialect is required")
	default:
		return nil, fmt.Errorf("guardsql: unsupported dialect %q", d)
	}
}

// jsonPathDollar builds a JSONPath expression ($.a.b) from validated segments.
func jsonPathDollar(path []string) string {
	var b strings.Builder
	b.WriteByte('$')
	for _, seg := range path {
		b.WriteByte('.')
		b.WriteString(seg)
	}
	return b.String()
}

type sqliteDialect struct{}

func (sqliteDialect) quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
func (sqliteDialect) placeholder(int) string        { return "?" }
func (sqliteDialect) castNumber(expr string) string { return expr }
func (sqliteDialect) castBool(expr string) string   { return expr }
func (sqliteDialect) jsonExtractText(col string, path []string, bind func(any) string) string {
	return "json_extract(" + col + ", " + bind(jsonPathDollar(path)) + ")"
}

type mysqlDialect struct{}

func (mysqlDialect) quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}
func (mysqlDialect) placeholder(int) string        { return "?" }
func (mysqlDialect) castNumber(expr string) string { return expr }
func (mysqlDialect) castBool(expr string) string   { return expr }
func (mysqlDialect) jsonExtractText(col string, path []string, bind func(any) string) string {
	return "JSON_UNQUOTE(JSON_EXTRACT(" + col + ", " + bind(jsonPathDollar(path)) + "))"
}

type postgresDialect struct{}

func (postgresDialect) quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
func (postgresDialect) placeholder(n int) string { return "$" + strconv.Itoa(n) }

// jsonb_extract_path_text yields text, which Postgres will not compare or sum
// against numbers or booleans without an explicit cast. A JSON value that is
// not convertible makes the query fail at execution time.
func (postgresDialect) castNumber(expr string) string { return "(" + expr + ")::numeric" }
func (postgresDialect) castBool(expr string) string   { return "(" + expr + ")::boolean" }
func (postgresDialect) jsonExtractText(col string, path []string, bind func(any) string) string {
	// jsonb_extract_path_text(col::jsonb, seg, seg, ...) binds each segment and
	// works whether the column is json or jsonb.
	var b strings.Builder
	b.WriteString("jsonb_extract_path_text(")
	b.WriteString(col)
	b.WriteString("::jsonb")
	for _, seg := range path {
		b.WriteString(", ")
		b.WriteString(bind(seg))
	}
	b.WriteString(")")
	return b.String()
}
