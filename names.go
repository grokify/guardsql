package guardsql

import (
	"fmt"
	"strings"
)

// OutputName returns the result column name for a SELECT item.
func OutputName(item SelectItem) string {
	return outputName(item)
}

func outputName(item SelectItem) string {
	if item.Alias != "" {
		return item.Alias
	}
	if item.Aggregate != nil {
		if item.Aggregate.Star {
			return strings.ToLower(string(item.Aggregate.Func)) + "_all"
		}
		return strings.ToLower(fmt.Sprintf("%s_%s", item.Aggregate.Func, item.Aggregate.Field))
	}
	return item.Field
}
