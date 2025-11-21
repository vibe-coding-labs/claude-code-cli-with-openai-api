package main

import (
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/cmd"
)

func main() {
	// Inject frontend filesystem functions
	cmd.SetFrontendFunctions(GetFrontendFS, IsFrontendEmbedded)

	// Execute CLI
	cmd.Execute()
}
