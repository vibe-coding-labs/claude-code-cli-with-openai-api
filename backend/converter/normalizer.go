package converter

import (
	"strings"
	"unicode"
)

// Tool call ID prefixes from different providers
const (
	IDPrefixClaude  = "toolu_" // Anthropic Claude native prefix
	IDPrefixOpenAI  = "call_"  // OpenAI native prefix
	IDPrefixGeneric = "fc_"    // Generic function call prefix
)

// NormalizeToolCallID standardizes tool call IDs to use the Claude prefix.
// Also sanitizes IDs to match Anthropic's ^[a-zA-Z0-9_-]+\$ pattern (litellm pattern).
func NormalizeToolCallID(id string) string {
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, IDPrefixClaude) {
		return sanitizeToolCallID(id)
	}
	for _, prefix := range []string{IDPrefixOpenAI, IDPrefixGeneric} {
		if strings.HasPrefix(id, prefix) {
			return sanitizeToolCallID(IDPrefixClaude + strings.TrimPrefix(id, prefix))
		}
	}
	return sanitizeToolCallID(IDPrefixClaude + id)
}

// sanitizeToolCallID removes characters not allowed in Anthropic tool call IDs.
// Anthropic requires ^[a-zA-Z0-9_-]+\$ — any other character is replaced with underscore.
func sanitizeToolCallID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// CamelToSnake converts camelCase or PascalCase to snake_case.
func CamelToSnake(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// SnakeToCamel converts snake_case to camelCase.
func SnakeToCamel(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// NormalizeToolParameters normalizes parameter names to match expected casing.
func NormalizeToolParameters(toolName string, params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	knownSnakeParams := map[string]bool{
		"command": true, "file_path": true, "old_string": true, "new_string": true,
		"search_text": true, "replace_text": true, "pattern": true, "path": true,
		"content": true, "offset": true, "limit": true, "name": true,
		"description": true, "query": true, "glob": true, "timeout": true,
	}

	normalized := make(map[string]interface{}, len(params))
	for key, value := range params {
		if knownSnakeParams[key] {
			normalized[key] = value
			continue
		}
		snakeKey := CamelToSnake(key)
		if knownSnakeParams[snakeKey] {
			normalized[snakeKey] = value
		} else {
			normalized[key] = value
		}
	}

	return normalized
}
