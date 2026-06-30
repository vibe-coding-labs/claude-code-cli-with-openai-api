package database

import "testing"

func TestResolveModelAlias(t *testing.T) {
	mappings := map[string]string{
		"glm-5.1":      "astron-code-latest",
		"glm-4.6":      "astron-code-latest",
		"empty-target": "",
	}

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"alias hit", "glm-5.1", "astron-code-latest"},
		{"second alias hit", "glm-4.6", "astron-code-latest"},
		{"unmapped name passes through", "gpt-4o", "gpt-4o"},
		{"alias with empty target ignored", "empty-target", "empty-target"},
		{"empty model passes through", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveModelAlias(tc.model, mappings); got != tc.want {
				t.Errorf("ResolveModelAlias(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}

func TestResolveModelAliasNilAndEmptyMap(t *testing.T) {
	if got := ResolveModelAlias("glm-5.1", nil); got != "glm-5.1" {
		t.Errorf("nil mappings should pass through, got %q", got)
	}
	if got := ResolveModelAlias("glm-5.1", map[string]string{}); got != "glm-5.1" {
		t.Errorf("empty mappings should pass through, got %q", got)
	}
}
