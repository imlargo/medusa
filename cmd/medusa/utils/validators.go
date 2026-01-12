package utils

import (
	"regexp"
	"strings"
	"unicode"
)

// IsValidProjectName checks if a project name is valid
func IsValidProjectName(name string) bool {
	// Must start with letter, contain only letters, numbers, dashes, underscores
	match, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_-]*$`, name)
	return match
}

// Capitalize capitalizes the first letter of a string
func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// ToSnakeCase converts a string to snake_case
func ToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ToCamelCase converts a string to CamelCase
func ToCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = Capitalize(parts[i])
	}
	return strings.Join(parts, "")
}

// Pluralize adds 's' to a word (simple pluralization)
func Pluralize(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}
