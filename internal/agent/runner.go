// Package agent wires Rook's autonomous security agent on top of the
// ChatBotKit Go SDK. It loads the embedded skill library, registers the
// default file and shell tools, and drives the agent loop until it exits.
package agent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"

	rook "github.com/chatbotkit/rook"
	"github.com/chatbotkit/rook/internal/config"

	sdkagent "github.com/chatbotkit/go-sdk/agent"
	"github.com/chatbotkit/go-sdk/sdk"
	"github.com/chatbotkit/go-sdk/types"
)

// Config controls a single autonomous run.
type Config struct {
	// APISecret is the credential for the selected backend.
	APISecret string
	// BaseURL is the API endpoint for the selected backend. Empty uses the SDK
	// default (the ChatBotKit API).
	BaseURL string
	// Model is the model the agent reasons with (e.g. "gpt-4o").
	Model string
	// MaxIterations bounds how many tool-using turns the agent may take
	// before it is forced to stop.
	MaxIterations int
	// Task is the objective the operator hands to the agent.
	Task string
	// Scope is the explicit authorization boundary (hosts, repos, paths) the
	// agent must stay within. It is injected into the backstory.
	Scope string
	// Verbose prints each token as it streams in addition to tool activity.
	Verbose bool
}

// Run loads the embedded skills, builds the agent, and streams its execution
// to stdout. It returns the agent's exit code.
func Run(ctx context.Context, cfg Config) (int, error) {
	subFS, err := fs.Sub(rook.SkillsFS, "skills")
	if err != nil {
		return 1, fmt.Errorf("open embedded skills: %w", err)
	}

	skillsResult, err := sdkagent.LoadSkillsFromFS(subFS)
	if err != nil {
		return 1, fmt.Errorf("load embedded skills: %w", err)
	}

	skills := skillsResult.GetSkills()

	fmt.Fprintf(os.Stderr, "Loaded %d embedded skill(s):\n", len(skills))
	for _, s := range skills {
		fmt.Fprintf(os.Stderr, "  • %s - %s\n", s.Name, s.Description)
	}
	fmt.Fprintln(os.Stderr)

	scope := strings.TrimSpace(cfg.Scope)
	if scope == "" {
		scope = "Authorized scope: not specified. Treat the current working " +
			"directory as the only target and do not reach out to remote systems."
	} else {
		scope = "Authorized scope:\n" + scope
	}

	backstory := fmt.Sprintf(config.Backstory, scope)

	client := sdk.New(sdk.Options{Secret: cfg.APISecret, BaseURL: cfg.BaseURL})

	tools := sdkagent.DefaultTools()

	skillsFeature := sdkagent.CreateSkillsFeature(skills)

	messages := []sdkagent.Message{
		{Type: "user", Text: cfg.Task},
	}

	events, errs := sdkagent.ExecuteWithTools(ctx, client, sdkagent.ExecuteWithToolsOptions{
		Model:         cfg.Model,
		Messages:      messages,
		Backstory:     backstory,
		Tools:         tools,
		MaxIterations: cfg.MaxIterations,
		Extensions: &types.ConversationCompleteRequestExtensions{
			Features: []types.CompleteFeature{
				{
					Name:    skillsFeature["name"].(string),
					Options: skillsFeature["options"].(map[string]interface{}),
				},
			},
		},
	})

	exitCode := 0

	for event := range events {
		switch e := event.(type) {
		case sdkagent.TokenAgentEvent:
			if cfg.Verbose {
				fmt.Print(e.Token)
			}
		case sdkagent.IterationEvent:
			fmt.Fprintf(os.Stderr, "\n--- Iteration %d ---\n", e.Iteration)
		case sdkagent.ToolCallStartEvent:
			fmt.Fprintf(os.Stderr, "\n[%s] %v\n", e.Name, e.Args)
		case sdkagent.ToolCallEndEvent:
			fmt.Fprintf(os.Stderr, "[%s] → %v\n", e.Name, truncate(e.Result))
		case sdkagent.ToolCallErrorEvent:
			fmt.Fprintf(os.Stderr, "[%s] error: %s\n", e.Name, e.Error)
		case sdkagent.AgentExitEvent:
			fmt.Fprintf(os.Stderr, "\n\n=== Agent exited with code %d ===\n", e.Code)
			if e.Message != "" {
				fmt.Fprintf(os.Stderr, "Message: %s\n", e.Message)
			}
			exitCode = e.Code
		}
	}

	if err := <-errs; err != nil {
		return 1, err
	}

	return exitCode, nil
}

// truncate shortens long string fields in a tool result for terminal display.
func truncate(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	const limit = 200
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok && len(s) > limit {
			out[k] = s[:limit] + "… (truncated)"
			continue
		}
		out[k] = v
	}
	return out
}
