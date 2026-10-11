package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
)

const mcpOutputLimit = 1 << 20

var mcpRedacted = []byte("[redacted]")

type mcpOutput struct {
	Text      string `json:"text" jsonschema:"Captured CLI bytes, represented as UTF-8 text or base64 according to encoding."`
	Encoding  string `json:"encoding" jsonschema:"utf-8 for valid UTF-8 bytes; base64 otherwise."`
	Truncated bool   `json:"truncated" jsonschema:"True when captured output was clipped to the 1 MiB limit."`
}

type mcpExecutionResult struct {
	Stdout      mcpOutput `json:"stdout"`
	Stderr      mcpOutput `json:"stderr"`
	ExitCode    *int      `json:"exit_code" jsonschema:"Measured local CLI exit code, or null when the process did not exit normally. This does not confirm remote termination."`
	PreviewOnly bool      `json:"preview_only" jsonschema:"True when a Compose mutation was requested without yes=true."`
	Cancelled   bool      `json:"cancelled" jsonschema:"True when local execution observed context cancellation. Remote operations may continue."`
}

type mcpExecutor struct {
	executable  string
	environment []string
	key         []byte
}

func newMCPExecutor(cfg *config.Config) (*mcpExecutor, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locating the CLI executable: %w", err)
	}
	environment := os.Environ()
	frozen := make([]string, 0, len(environment)+5)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "BERTH_MCP_CHILD", "BERTH_SERVER_URL", "BERTH_API_KEY", "BERTH_INSECURE", "NO_COLOR":
			continue
		}
		frozen = append(frozen, entry)
	}
	frozen = append(frozen,
		"BERTH_MCP_CHILD=1",
		"BERTH_SERVER_URL="+cfg.Server,
		"BERTH_API_KEY="+cfg.APIKey,
		"BERTH_INSECURE="+strconv.FormatBool(cfg.Insecure),
		"NO_COLOR=1",
	)
	return &mcpExecutor{executable: executable, environment: frozen, key: []byte(cfg.APIKey)}, nil
}

func (e *mcpExecutor) execute(ctx context.Context, command mcpCommand, input map[string]any) (mcpExecutionResult, error) {
	result := mcpExecutionResult{
		Stdout:      mcpOutput{Encoding: "utf-8"},
		Stderr:      mcpOutput{Encoding: "utf-8"},
		PreviewOnly: command.preview && input["yes"] != true,
	}
	arguments, err := command.arguments(input)
	if err != nil {
		result.Cancelled = ctx.Err() != nil
		return result, fmt.Errorf("invalid CLI arguments: %s", e.redact(err.Error()))
	}
	stdout := mcpCapture{key: e.key}
	stderr := mcpCapture{key: e.key}
	child := exec.CommandContext(ctx, e.executable, arguments...)
	child.Env = e.environment
	child.Stdout = &stdout
	child.Stderr = &stderr
	err = child.Run()
	result.Stdout = stdout.output()
	result.Stderr = stderr.output()
	result.Cancelled = ctx.Err() != nil
	if child.ProcessState != nil && child.ProcessState.Exited() {
		code := child.ProcessState.ExitCode()
		result.ExitCode = &code
	}
	if result.Cancelled {
		return result, fmt.Errorf("local CLI execution cancelled; remote operations may continue")
	}
	if err != nil {
		return result, fmt.Errorf("local CLI execution failed: %s", e.redact(err.Error()))
	}
	return result, nil
}

func (e *mcpExecutor) redact(text string) string {
	if len(e.key) == 0 {
		return text
	}
	return strings.ReplaceAll(text, string(e.key), string(mcpRedacted))
}

type mcpCapture struct {
	data      []byte
	key       []byte
	truncated bool
}

func (c *mcpCapture) Write(data []byte) (int, error) {
	length := len(data)
	if length > mcpOutputLimit-len(c.data) {
		c.truncated = true
	}
	limit := mcpOutputLimit + max(0, len(c.key)-1)
	data = data[:min(length, limit-len(c.data))]
	if len(data) == 0 {
		return length, nil
	}
	needed := len(c.data) + len(data)
	if needed > cap(c.data) {
		capacity := min(max(needed, 2*cap(c.data)), limit)
		grown := make([]byte, len(c.data), capacity)
		copy(grown, c.data)
		c.data = grown
	}
	c.data = append(c.data, data...)
	return length, nil
}

func (c *mcpCapture) output() mcpOutput {
	data := c.data
	if len(c.key) > 0 && bytes.Contains(data, c.key) {
		data = bytes.ReplaceAll(data, c.key, mcpRedacted)
	}
	if len(data) > mcpOutputLimit {
		data = data[:mcpOutputLimit]
		c.truncated = true
	}
	result := mcpOutput{Encoding: "utf-8", Truncated: c.truncated}
	if utf8.Valid(data) {
		result.Text = string(data)
	} else {
		result.Text = base64.StdEncoding.EncodeToString(data)
		result.Encoding = "base64"
	}
	return result
}
