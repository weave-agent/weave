package settings

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"weave/sdk"
)

// HelpError is returned when --help or -h is requested.
// It implements error.Is(flag.ErrHelp) so callers can detect help requests.
type HelpError struct {
	Text string
}

func (e *HelpError) Error() string { return e.Text }

func (e *HelpError) Is(target error) bool {
	return target == flag.ErrHelp
}

// GenerateFullHelp returns the complete help text for weave, including
// global flags and all registered extension schemas.
func GenerateFullHelp() string {
	var b strings.Builder

	b.WriteString("Usage: weave [options] [command]\n\n")
	b.WriteString("Commands:\n")
	b.WriteString("  install <source>     Install an extension\n")
	b.WriteString("  list                 List installed extensions\n")
	b.WriteString("  update [<name>]      Update installed extensions\n")
	b.WriteString("  uninstall <name>     Remove an extension\n")
	b.WriteString("\n")

	writeGlobalFlags(&b)
	writeExtensionFlags(&b)

	return b.String()
}

// globalFlag describes a fixed top-level CLI flag.
type globalFlag struct {
	long, short, desc string
}

var globalFlags = []globalFlag{
	{"--prompt", "-p", "Prompt to pass to the agent"},
	{"--ui", "", "UI extension name"},
	{"--output", "", "Output format: text or json"},
	{"--tools", "", "Comma-separated tool allowlist"},
	{"--subagent-id", "", "Subagent ID for inter-agent communication"},
	{"--sandbox", "", "Sandbox mode override"},
	{"--model", "", "Model override for this session"}, //nolint:goconst // used across multiple packages
}

func writeGlobalFlags(b *strings.Builder) {
	b.WriteString("Global flags:\n")

	for _, f := range globalFlags {
		if f.short != "" {
			fmt.Fprintf(b, "  %s, %s\n", f.short, padRight(f.long, 18)+f.desc)
		} else {
			fmt.Fprintf(b, "  %s\n", padRight(f.long, 22)+f.desc)
		}
	}

	b.WriteString("\n")
}

func writeExtensionFlags(b *strings.Builder) {
	schemas := sdk.ListSchemas()
	if len(schemas) == 0 {
		return
	}

	// Group by scope.
	byScope := make(map[string][]namedSchema)
	for name, info := range schemas {
		byScope[info.Scope] = append(byScope[info.Scope], namedSchema{
			name:   name,
			schema: info.Schema,
		})
	}

	scopeOrder := []string{"tools", "providers", "ui", "extensions"}
	scopeTitles := map[string]string{
		"tools":      "Tool options",
		"providers":  "Provider options",
		"ui":         "UI options",
		"extensions": "Extension options",
	}

	for _, scope := range scopeOrder {
		items, ok := byScope[scope]
		if !ok || len(items) == 0 {
			continue
		}

		sort.Slice(items, func(i, j int) bool {
			return items[i].name < items[j].name
		})

		b.WriteString(scopeTitles[scope] + ":\n")

		for _, item := range items {
			if len(item.schema.Fields) == 0 {
				continue
			}

			fmt.Fprintf(b, "  %s\n", item.name)

			for _, field := range item.schema.Fields {
				flagName := field.Flag
				if flagName == "" {
					flagName = field.JSONName
				}

				flagName = "--" + item.name + "-" + flagName

				var parts []string

				if field.Short != "" {
					parts = append(parts, "-"+field.Short+",")
				}

				parts = append(parts, flagName)

				var meta []string

				if field.Default != "" {
					meta = append(meta, "default: "+field.Default)
				}

				if field.Env != "" {
					meta = append(meta, "env: "+field.Env)
				}

				flagCol := strings.Join(parts, " ")
				desc := field.Description

				if len(meta) > 0 {
					desc += " [" + strings.Join(meta, ", ") + "]"
				}

				fmt.Fprintf(b, "    %-26s %s\n", flagCol, desc)
			}
		}

		b.WriteString("\n")
	}
}

// namedSchema pairs a schema with its registered name.
type namedSchema struct {
	name   string
	schema sdk.Schema
}

// padRight pads s with spaces on the right to reach width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s + " "
	}

	return s + strings.Repeat(" ", width-len(s))
}
