package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// FlagHelpDetail holds extended help information for one flag, shown when the
// user types --<flag> --help.
type FlagHelpDetail struct {
	Name         string `json:"Name"`
	Type         string `json:"Type"`
	Description  string `json:"Description"`
	DetailedHelp string `json:"DetailedHelp"`
}

// detectFlagHelpRequest inspects os.Args for the pattern where a command-
// specific flag name (e.g. --filters) appears alongside --help/-h. If found,
// it returns the flag names that the user wants per-flag help for.
func detectFlagHelpRequest(cmd *cobra.Command, rawArgs []string) []string {
	if cmd == nil {
		return nil
	}
	hasHelp := false
	for _, arg := range rawArgs {
		if arg == "--help" || arg == "-h" {
			hasHelp = true
			break
		}
	}
	if !hasHelp {
		return nil
	}

	// Collect flag names specified on the command line (excluding global/root flags).
	var flagNames []string
	for _, arg := range rawArgs {
		if arg == "--help" || arg == "-h" || arg == "--" {
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		// Handle --flag=value form
		if idx := strings.Index(name, "="); idx >= 0 {
			name = name[:idx]
		}
		// Skip global/persistent flags that have no per-flag help
		if isGlobalFlag(name) {
			continue
		}
		// Check this flag exists on the target command and has detailed help
		f := cmd.Flags().Lookup(name)
		if f == nil {
			// Also check inherited flags
			f = cmd.InheritedFlags().Lookup(name)
		}
		if f == nil {
			continue
		}
		if ann, ok := f.Annotations["agr.detailed_help"]; ok && len(ann) > 0 && ann[0] != "" {
			flagNames = append(flagNames, name)
		}
	}
	return flagNames
}

// renderFlagHelp prints the detailed help for the given flags on the command.
func renderFlagHelp(w io.Writer, cmd *cobra.Command, flagNames []string) {
	commandPath := cmd.CommandPath()
	for i, name := range flagNames {
		if i > 0 {
			fmt.Fprintln(w)
		}
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.InheritedFlags().Lookup(name)
		}
		if f == nil {
			fmt.Fprintf(w, "Unknown flag: --%s\n", name)
			continue
		}

		fmt.Fprintf(w, "Flag: --%s", name)
		if f.Shorthand != "" {
			fmt.Fprintf(w, " (-%s)", f.Shorthand)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Command: %s\n", commandPath)
		fmt.Fprintf(w, "Type: %s\n", f.Value.Type())
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" {
			fmt.Fprintf(w, "Default: %s\n", f.DefValue)
		}
		fmt.Fprintln(w)

		if f.Usage != "" {
			fmt.Fprintf(w, "Summary:\n  %s\n\n", f.Usage)
		}

		if ann, ok := f.Annotations["agr.detailed_help"]; ok && len(ann) > 0 && ann[0] != "" {
			fmt.Fprintf(w, "Details:\n%s\n", indentBlock(ann[0], "  "))
		}
	}
}

// renderFlagHelpJSON returns structured flag help data for JSON output.
func renderFlagHelpJSON(cmd *cobra.Command, flagNames []string) []FlagHelpDetail {
	details := make([]FlagHelpDetail, 0, len(flagNames))
	for _, name := range flagNames {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.InheritedFlags().Lookup(name)
		}
		if f == nil {
			continue
		}
		detail := FlagHelpDetail{
			Name:        name,
			Type:        f.Value.Type(),
			Description: f.Usage,
		}
		if ann, ok := f.Annotations["agr.detailed_help"]; ok && len(ann) > 0 {
			detail.DetailedHelp = ann[0]
		}
		details = append(details, detail)
	}
	return details
}

// tryFlagHelp detects and handles per-flag help requests. Returns true if
// per-flag help was rendered (the caller should not show normal help).
func tryFlagHelp(cmd *cobra.Command) bool {
	flagNames := detectFlagHelpRequest(cmd, os.Args[1:])
	if len(flagNames) == 0 {
		return false
	}
	if isJSON() || hasRawOutputFlag("json") {
		details := renderFlagHelpJSON(cmd, flagNames)
		data := map[string]any{"Flags": details}
		_ = writeEnvelope(ios.Out, "flag-help", "succeeded", data, nil, nil, nil, 0, nil)
	} else {
		renderFlagHelp(ios.Out, cmd, flagNames)
	}
	return true
}

func isGlobalFlag(name string) bool {
	switch name {
	case "help", "config", "output", "region", "domain", "cloud-endpoint",
		"secret-id", "secret-key", "jq", "non-interactive", "no-color",
		"debug", "generate-skeleton", "version":
		return true
	default:
		return false
	}
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line != "" {
			b.WriteString(prefix)
			b.WriteString(line)
		}
	}
	return b.String()
}
