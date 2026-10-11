package cmd

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type mcpPositional struct {
	name     string
	required bool
	array    bool
}

type mcpCommand struct {
	tool        *mcp.Tool
	preview     bool
	positionals []mcpPositional
	flags       []*pflag.Flag
	required    []string
	path        []string
	exec        bool
}

var mcpPropertyName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func mcpMetadata(risk string, positionals ...string) map[string]string {
	return map[string]string{"mcp-risk": risk, "mcp-positionals": strings.Join(positionals, ",")}
}

func mcpTransportMetadata() map[string]string {
	return map[string]string{"mcp-risk": "transport"}
}

func init() {
	existing := make(map[*cobra.Command]bool)
	for _, command := range rootCmd.Commands() {
		existing[command] = true
	}
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	var mark func(*cobra.Command)
	mark = func(command *cobra.Command) {
		command.Annotations = mcpTransportMetadata()
		for _, child := range command.Commands() {
			mark(child)
		}
	}
	for _, command := range rootCmd.Commands() {
		if !existing[command] {
			mark(command)
		}
	}
}

func collectMCPCommands(root *cobra.Command) ([]mcpCommand, error) {
	var commands []mcpCommand
	names := make(map[string]bool)
	var visit func(*cobra.Command, []string) error
	visit = func(command *cobra.Command, path []string) error {
		if command.Runnable() && command.Annotations["mcp-risk"] != "transport" {
			entry, err := describeMCPCommand(command, path)
			if err != nil {
				return fmt.Errorf("MCP command %s: %w", command.CommandPath(), err)
			}
			if names[entry.tool.Name] {
				return fmt.Errorf("MCP tool name collision: %s", entry.tool.Name)
			}
			names[entry.tool.Name] = true
			commands = append(commands, entry)
		}
		for _, child := range command.Commands() {
			childPath := make([]string, len(path)+1)
			copy(childPath, path)
			childPath[len(path)] = child.Name()
			if err := visit(child, childPath); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root, nil); err != nil {
		return nil, err
	}
	return commands, nil
}

func describeMCPCommand(command *cobra.Command, path []string) (mcpCommand, error) {
	entry := mcpCommand{path: path, required: []string{}}
	risk := command.Annotations["mcp-risk"]
	switch risk {
	case "read", "write":
	case "compose-write":
		entry.preview = true
	case "exec":
		entry.exec = true
	default:
		return entry, fmt.Errorf("missing or invalid risk metadata")
	}
	positionalText, ok := command.Annotations["mcp-positionals"]
	if !ok {
		return entry, fmt.Errorf("missing positional metadata")
	}
	properties := make(map[string]any)
	optional := false
	if positionalText != "" {
		parts := strings.Split(positionalText, ",")
		for index, text := range parts {
			positional := mcpPositional{required: !strings.HasSuffix(text, "?")}
			text = strings.TrimSuffix(text, "?")
			positional.array = strings.HasSuffix(text, "...")
			positional.name = strings.TrimSuffix(text, "...")
			if !mcpPropertyName.MatchString(positional.name) || (positional.array && index != len(parts)-1) || (optional && positional.required) {
				return entry, fmt.Errorf("invalid positional metadata %q", positionalText)
			}
			if _, exists := properties[positional.name]; exists {
				return entry, fmt.Errorf("duplicate positional property %q", positional.name)
			}
			optional = optional || !positional.required
			property := map[string]any{"type": "string", "description": strings.ReplaceAll(positional.name, "-", " ")}
			if positional.array {
				property["type"] = "array"
				property["items"] = map[string]any{"type": "string"}
				if positional.required {
					property["minItems"] = 1
				}
			}
			properties[positional.name] = property
			entry.positionals = append(entry.positionals, positional)
			if positional.required {
				entry.required = append(entry.required, positional.name)
			}
		}
	}
	usagePositionals := 0
	usage := strings.Fields(command.Use)
	if len(usage) == 0 {
		return entry, fmt.Errorf("missing command usage")
	}
	for _, token := range usage[1:] {
		if token != "--" {
			usagePositionals++
		}
	}
	metadataPositionals := len(entry.positionals)
	if entry.exec {
		metadataPositionals++
	}
	if usagePositionals != metadataPositionals {
		return entry, fmt.Errorf("positional metadata does not cover command usage")
	}
	if entry.exec {
		if len(entry.positionals) != 2 || entry.positionals[0].array || !entry.positionals[1].array {
			return entry, fmt.Errorf("exec requires service and command positional metadata")
		}
		properties[entry.positionals[0].name].(map[string]any)["pattern"] = "^[^-]"
		properties[entry.positionals[0].name].(map[string]any)["minLength"] = 1
	}
	flags := pflag.NewFlagSet(command.Name(), pflag.ContinueOnError)
	flags.AddFlagSet(command.LocalFlags())
	flags.AddFlagSet(command.InheritedFlags())
	var flagError error
	flags.VisitAll(func(flag *pflag.Flag) {
		if flagError != nil || mcpExcludedFlag(flag) {
			return
		}
		if _, exists := properties[flag.Name]; exists {
			flagError = fmt.Errorf("flag and positional property collision: %s", flag.Name)
			return
		}
		property, err := mcpFlagSchema(flag)
		if err != nil {
			flagError = err
			return
		}
		if len(flag.Annotations[cobra.BashCompOneRequiredFlag]) > 0 || mcpRequiredFlag(command, flag.Name) {
			delete(property, "default")
			entry.required = append(entry.required, flag.Name)
		}
		properties[flag.Name] = property
		entry.flags = append(entry.flags, flag)
	})
	if flagError != nil {
		return entry, flagError
	}
	for _, name := range strings.Fields(command.Annotations["mcp-required-flags"]) {
		if _, exists := properties[name]; !exists || flags.Lookup(name) == nil || mcpExcludedFlag(flags.Lookup(name)) {
			return entry, fmt.Errorf("required flag %q is unavailable", name)
		}
	}
	if entry.preview && (len(path) < 2 || path[0] != "compose" || flags.Lookup("yes") == nil || flags.Lookup("yes").Value.Type() != "bool") {
		return entry, fmt.Errorf("Compose mutation requires a boolean yes flag")
	}
	if len(path) == 0 || command.Short == "" {
		return entry, fmt.Errorf("missing command path or description")
	}
	destructive := risk != "read"
	openWorld := true
	entry.tool = &mcp.Tool{
		Name:        "berth_" + strings.ReplaceAll(strings.Join(path, "_"), "-", "_"),
		Description: command.Long,
		InputSchema: map[string]any{"type": "object", "properties": properties, "required": entry.required, "additionalProperties": false},
		Annotations: &mcp.ToolAnnotations{Title: command.Short, ReadOnlyHint: risk == "read", DestructiveHint: &destructive, OpenWorldHint: &openWorld},
	}
	if entry.tool.Description == "" {
		entry.tool.Description = command.Short
	}
	return entry, nil
}

func mcpRequiredFlag(command *cobra.Command, name string) bool {
	for _, required := range strings.Fields(command.Annotations["mcp-required-flags"]) {
		if required == name {
			return true
		}
	}
	return false
}

func mcpExcludedFlag(flag *pflag.Flag) bool {
	switch flag.Name {
	case "server", "api-key", "insecure", "verbose":
		return true
	}
	return len(flag.Annotations[cobra.FlagSetByCobraAnnotation]) > 0
}

func mcpFlagSchema(flag *pflag.Flag) (map[string]any, error) {
	property := map[string]any{"description": flag.Usage}
	switch flag.Value.Type() {
	case "string", "duration":
		property["type"] = "string"
		property["default"] = flag.DefValue
	case "bool":
		property["type"] = "boolean"
		value, err := strconv.ParseBool(flag.DefValue)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean flag default: %s", flag.Name)
		}
		property["default"] = value
	case "stringArray", "stringSlice":
		property["type"] = "array"
		property["items"] = map[string]any{"type": "string"}
	default:
		return nil, fmt.Errorf("unsupported flag type %s for %s", flag.Value.Type(), flag.Name)
	}
	if flag.Name == "output" {
		property["default"] = "json"
	}
	if flag.Name == "no-colour" {
		property["default"] = true
	}
	return property, nil
}

func (c mcpCommand) arguments(input map[string]any) ([]string, error) {
	properties := c.tool.InputSchema.(map[string]any)["properties"].(map[string]any)
	for name := range input {
		if _, known := properties[name]; !known {
			return nil, fmt.Errorf("unknown argument %q", name)
		}
	}
	for _, name := range c.required {
		if _, exists := input[name]; !exists {
			return nil, fmt.Errorf("missing required argument %q", name)
		}
	}
	argv := append([]string(nil), c.path...)
	for _, flag := range c.flags {
		value, exists := input[flag.Name]
		if !exists {
			switch flag.Name {
			case "output":
				value = "json"
			case "no-colour":
				value = true
			default:
				continue
			}
		}
		prefix := "--" + flag.Name + "="
		switch flag.Value.Type() {
		case "string", "duration":
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", flag.Name)
			}
			if flag.Value.Type() == "duration" {
				if _, err := time.ParseDuration(text); err != nil {
					return nil, fmt.Errorf("invalid %s duration: %w", flag.Name, err)
				}
			}
			argv = append(argv, prefix+text)
		case "bool":
			boolean, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be a boolean", flag.Name)
			}
			argv = append(argv, prefix+strconv.FormatBool(boolean))
		case "stringArray", "stringSlice":
			values, err := mcpStrings(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", flag.Name, err)
			}
			if flag.Value.Type() == "stringArray" {
				for _, text := range values {
					argv = append(argv, prefix+text)
				}
			} else {
				var buffer bytes.Buffer
				writer := csv.NewWriter(&buffer)
				if err := writer.Write(values); err != nil {
					return nil, err
				}
				writer.Flush()
				if err := writer.Error(); err != nil {
					return nil, err
				}
				encoded := strings.TrimSuffix(buffer.String(), "\n")
				if len(values) == 1 && values[0] == "" {
					encoded = `""`
				}
				argv = append(argv, prefix+encoded)
			}
		}
	}
	var positionals []string
	for _, positional := range c.positionals {
		value, exists := input[positional.name]
		if !exists {
			continue
		}
		if positional.array {
			values, err := mcpStrings(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", positional.name, err)
			}
			if positional.required && len(values) == 0 {
				return nil, fmt.Errorf("%s must not be empty", positional.name)
			}
			positionals = append(positionals, values...)
		} else {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", positional.name)
			}
			positionals = append(positionals, text)
		}
	}
	if c.exec {
		if len(positionals) < 2 || positionals[0] == "" || strings.HasPrefix(positionals[0], "-") || positionals[1] == "" {
			return nil, fmt.Errorf("exec requires a non-flag service name and a non-empty executable")
		}
		argv = append(argv, positionals[0], "--")
		return append(argv, positionals[1:]...), nil
	}
	if len(positionals) > 0 {
		argv = append(argv, "--")
		argv = append(argv, positionals...)
	}
	return argv, nil
}

func mcpStrings(value any) ([]string, error) {
	if values, ok := value.([]string); ok {
		return values, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("must be an array of strings")
	}
	result := make([]string, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("element %d must be a string", index)
		}
		result[index] = text
	}
	return result, nil
}
