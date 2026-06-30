package database

// ResolveModelAlias maps a client-supplied model name to the real upstream
// model name using a config's ModelMappings (alias -> real). This lets clients
// such as Codex send a model name their built-in catalog recognizes (e.g.
// "glm-5.1"), while the proxy forwards the real name the upstream provider
// requires (e.g. "astron-code-latest").
//
// If model is empty, mappings is nil/empty, or model is not present as an
// alias key (or maps to an empty string), the input is returned unchanged —
// i.e. unmapped names pass through verbatim. The function is side-effect free.
func ResolveModelAlias(model string, mappings map[string]string) string {
	if model == "" || len(mappings) == 0 {
		return model
	}
	if real, ok := mappings[model]; ok && real != "" {
		return real
	}
	return model
}
