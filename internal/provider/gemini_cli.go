package provider

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nex-crm/wuphf/internal/agent"
)

const GeminiCLIDefaultModel = "gemini-2.5-pro"

var geminiCLICandidates = []string{
	"gemini",
	"gemini.exe",
	"gemini.cmd",
	"gemini.bat",
}

func resolveGeminiCLIPath(lookPath func(string) (string, error)) (string, string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	for _, candidate := range geminiCLICandidates {
		if path, err := lookPath(candidate); err == nil {
			return path, candidate, nil
		}
	}
	return "", "", exec.ErrNotFound
}

// ResolveGeminiCLIPath is the exported resolver, used by diagnostics and tests.
func ResolveGeminiCLIPath(lookPath func(string) (string, error)) (string, string, error) {
	return resolveGeminiCLIPath(lookPath)
}

var (
	geminiCLILookPath = exec.LookPath
	geminiCLICommand  = exec.Command
)

// CreateGeminiCLIStreamFn returns a StreamFn that runs the Gemini CLI non-interactively.
// WUPHF owns conversation history; each invocation is stateless from the CLI's perspective.
func CreateGeminiCLIStreamFn(_ string) agent.StreamFn {
	return func(msgs []agent.Message, _ []agent.AgentTool) <-chan agent.StreamChunk {
		ch := make(chan agent.StreamChunk, 64)
		go func() {
			defer close(ch)

			_, binaryName, err := resolveGeminiCLIPath(geminiCLILookPath)
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: "Gemini CLI not found. Install with `npm i -g @google/gemini-cli` or use /provider to choose a different provider."}
				return
			}

			_, prompt := buildClaudePrompts(msgs)
			if prompt == "" {
				prompt = "Proceed with the task."
			}

			cmd := geminiCLICommand(binaryName, buildGeminiCLIArgs(prompt)...)
			cmd.Env = filteredEnv(nil)

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("attach gemini stdout: %v", err)}
				return
			}

			var stderr strings.Builder
			cmd.Stderr = &stderr

			if err := cmd.Start(); err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("gemini CLI failed to start: %v", err)}
				return
			}

			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if line := scanner.Text(); strings.TrimSpace(line) != "" {
					ch <- agent.StreamChunk{Type: "text", Content: line + "\n"}
				}
			}

			if err := cmd.Wait(); err != nil {
				detail := strings.TrimSpace(stderr.String())
				if detail != "" {
					ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini CLI error: %s", detail)}
				} else {
					ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini CLI exited with error: %v. Run `gemini auth login` or use /provider to choose a different provider.", err)}
				}
			}
		}()
		return ch
	}
}

func buildGeminiCLIArgs(prompt string) []string {
	return []string{"--model", GeminiCLIDefaultModel, "--yolo", "-p", prompt}
}
