package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDetectFlagHelpRequest(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("filters", "", "Filter results")
	cmd.Flags().Int("offset", 0, "Pagination offset")
	cmd.Flags().String("output", "", "Output format")
	// Set annotation on --filters flag
	f := cmd.Flags().Lookup("filters")
	f.Annotations = map[string][]string{"agr.detailed_help": {"Detailed help for filters"}}

	tests := []struct {
		name     string
		args     []string
		wantLen  int
		wantFlag string
	}{
		{
			name:    "no help flag",
			args:    []string{"list", "--filters", "x"},
			wantLen: 0,
		},
		{
			name:    "help only, no flag with detailed help",
			args:    []string{"list", "--help"},
			wantLen: 0,
		},
		{
			name:     "flag with detailed help and --help",
			args:     []string{"list", "--filters", "--help"},
			wantLen:  1,
			wantFlag: "filters",
		},
		{
			name:     "help before flag",
			args:     []string{"list", "--help", "--filters"},
			wantLen:  1,
			wantFlag: "filters",
		},
		{
			name:    "flag without detailed help and --help",
			args:    []string{"list", "--offset", "--help"},
			wantLen: 0,
		},
		{
			name:    "global flag should be skipped",
			args:    []string{"list", "--output", "json", "--help"},
			wantLen: 0,
		},
		{
			name:     "flag=value form",
			args:     []string{"list", "--filters=test", "--help"},
			wantLen:  1,
			wantFlag: "filters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := detectFlagHelpRequest(cmd, tc.args)
			if len(result) != tc.wantLen {
				t.Fatalf("detectFlagHelpRequest(%v) returned %d flags, want %d", tc.args, len(result), tc.wantLen)
			}
			if tc.wantLen > 0 && result[0] != tc.wantFlag {
				t.Fatalf("detectFlagHelpRequest(%v) returned flag %q, want %q", tc.args, result[0], tc.wantFlag)
			}
		})
	}
}

func TestDetectFlagHelpRequestNilCmd(t *testing.T) {
	result := detectFlagHelpRequest(nil, []string{"--filters", "--help"})
	if result != nil {
		t.Fatalf("expected nil for nil cmd, got %v", result)
	}
}

func TestRenderFlagHelp(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Short: "List items"}
	cmd.Flags().StringP("filters", "f", "", "Filter results")
	f := cmd.Flags().Lookup("filters")
	f.Annotations = map[string][]string{
		"agr.detailed_help": {"Supported filters:\n  Name=value\n  Status=ACTIVE|FAILED"},
	}

	var buf bytes.Buffer
	renderFlagHelp(&buf, cmd, []string{"filters"})
	output := buf.String()

	// Should contain the flag name
	if !strings.Contains(output, "Flag: --filters") {
		t.Errorf("output missing flag name, got:\n%s", output)
	}
	// Should contain shorthand
	if !strings.Contains(output, "(-f)") {
		t.Errorf("output missing shorthand, got:\n%s", output)
	}
	// Should contain usage
	if !strings.Contains(output, "Filter results") {
		t.Errorf("output missing usage, got:\n%s", output)
	}
	// Should contain detailed help
	if !strings.Contains(output, "Supported filters:") {
		t.Errorf("output missing detailed help content, got:\n%s", output)
	}
	if !strings.Contains(output, "Status=ACTIVE|FAILED") {
		t.Errorf("output missing detailed help values, got:\n%s", output)
	}
}

func TestRenderFlagHelpJSON(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("filters", "", "Filter by attribute")
	f := cmd.Flags().Lookup("filters")
	f.Annotations = map[string][]string{
		"agr.detailed_help": {"JSON filters format: [{Name,Values}]"},
	}

	details := renderFlagHelpJSON(cmd, []string{"filters"})
	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	d := details[0]
	if d.Name != "filters" {
		t.Errorf("Name=%q, want \"filters\"", d.Name)
	}
	if d.Description != "Filter by attribute" {
		t.Errorf("Description=%q, want \"Filter by attribute\"", d.Description)
	}
	if d.DetailedHelp != "JSON filters format: [{Name,Values}]" {
		t.Errorf("DetailedHelp=%q, want expected value", d.DetailedHelp)
	}
}

func TestRenderFlagHelpUnknownFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("filters", "", "Filter results")

	var buf bytes.Buffer
	renderFlagHelp(&buf, cmd, []string{"nonexistent"})
	output := buf.String()

	if !strings.Contains(output, "Unknown flag: --nonexistent") {
		t.Errorf("expected unknown flag message, got:\n%s", output)
	}
}

func TestIsGlobalFlag(t *testing.T) {
	globals := []string{"help", "config", "output", "region", "domain",
		"cloud-endpoint", "secret-id", "secret-key", "jq",
		"non-interactive", "no-color", "debug", "generate-skeleton", "version"}
	for _, flag := range globals {
		if !isGlobalFlag(flag) {
			t.Errorf("isGlobalFlag(%q) = false, want true", flag)
		}
	}
	nonGlobals := []string{"filters", "offset", "limit", "tool-ids", "instance-ids"}
	for _, flag := range nonGlobals {
		if isGlobalFlag(flag) {
			t.Errorf("isGlobalFlag(%q) = true, want false", flag)
		}
	}
}

func TestIndentBlock(t *testing.T) {
	tests := []struct {
		input  string
		prefix string
		want   string
	}{
		{"hello\nworld", "  ", "  hello\n  world"},
		{"single", ">> ", ">> single"},
		{"line1\n\nline3", "  ", "  line1\n\n  line3"},
	}
	for _, tc := range tests {
		got := indentBlock(tc.input, tc.prefix)
		if got != tc.want {
			t.Errorf("indentBlock(%q, %q) = %q, want %q", tc.input, tc.prefix, got, tc.want)
		}
	}
}
