package templates

import (
	"fmt"
	"strings"
	"time"
)

// Field represents a model field
type Field struct {
	Name     string
	Type     string
	Unique   bool
	Required bool
	Index    bool
}

// Helper functions
func getCurrentYear() string {
	return fmt.Sprintf("%d", time.Now().Year())
}

func toGoType(t string) string {
	typeMap := map[string]string{
		"string":  "string",
		"int":     "int",
		"int64":   "int64",
		"uint":    "uint",
		"float":   "float64",
		"float64": "float64",
		"bool":    "bool",
		"time":    "time.Time",
		"json":    "json.RawMessage",
		"text":    "string",
	}

	if goType, ok := typeMap[t]; ok {
		return goType
	}
	return "string"
}

func toGormType(t string) string {
	typeMap := map[string]string{
		"string":  "varchar(255)",
		"text":    "text",
		"int":     "int",
		"int64":   "bigint",
		"uint":    "int",
		"float64": "decimal(10,2)",
		"bool":    "boolean",
		"time":    "timestamp",
		"json":    "jsonb",
	}

	if gormType, ok := typeMap[t]; ok {
		return gormType
	}
	return "varchar(255)"
}
