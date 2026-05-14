package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func init() {
	schemaCmd.RunE = Wrap("schema", schemaFn)
	rootCmd.AddCommand(schemaCmd)
}

// CommandSchema describes a command's interface for machine consumption.
type CommandSchema struct {
	Name            string         `json:"Name"`
	Summary         string         `json:"Summary"`
	BackendSupport  []string       `json:"BackendSupport"`
	Mutation        bool           `json:"Mutation"`
	CreatesResource bool           `json:"CreatesResource"`
	Idempotency     string         `json:"Idempotency"`
	SupportsDryRun  bool           `json:"SupportsDryRun"`
	Interactive     bool           `json:"Interactive"`
	RequiresAuth    bool           `json:"RequiresAuth"`
	SupportsJson    bool           `json:"SupportsJson"`
	SupportsNdjson  bool           `json:"SupportsNdjson"`
	SupportsJq      bool           `json:"SupportsJq"`
	SupportsRequest bool           `json:"SupportsRequest"`
	RequestSchema   *RequestSchema `json:"RequestSchema"`
	Args            []ArgSchema    `json:"Args,omitempty"`
	Flags           []FlagSchema   `json:"Flags,omitempty"`
	Output          string         `json:"Output,omitempty"`
	Failures        []string       `json:"Failures,omitempty"`
}

// RequestSchema describes the --request JSON schema.
type RequestSchema struct {
	Type                 string                    `json:"Type"`
	AdditionalProperties bool                      `json:"AdditionalProperties"`
	Required             []string                  `json:"Required,omitempty"`
	Properties           map[string]PropertySchema `json:"Properties,omitempty"`
}

// PropertySchema describes a property in a request schema.
type PropertySchema struct {
	Type    string   `json:"Type"`
	Minimum *int     `json:"Minimum,omitempty"`
	Values  []string `json:"Values,omitempty"`
}

// ArgSchema describes a positional argument.
type ArgSchema struct {
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Required bool   `json:"Required"`
}

// FlagSchema describes a flag.
type FlagSchema struct {
	Name             string   `json:"Name"`
	Shorthand        string   `json:"Shorthand,omitempty"`
	Type             string   `json:"Type"`
	Values           []string `json:"Values,omitempty"`
	IncompatibleWith []string `json:"IncompatibleWith,omitempty"`
	AllowsOutput     []string `json:"AllowsOutput,omitempty"`
}

var schemaCmd = &cobra.Command{
	Use:   "schema [command-name]",
	Short: "Show command schema for machine consumption",
	Long: `Show the schema of commands for machine consumption.

Without arguments, shows all command schemas.
With a command name (dot-separated), shows that command's schema.

Examples:
  ags schema -o json
  ags schema instance.code.run -o json
  ags schema instance.create -o json`,
	Args: cobra.MaximumNArgs(1),
}

func schemaFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	schemas := getAllSchemas()

	if len(args) == 1 {
		name := args[0]
		for _, s := range schemas {
			if s.Name == name {
				return OK(s, func(w io.Writer) { renderSchemaText(w, s) }), nil
			}
		}
		return nil, fmt.Errorf("unknown command: %s", name)
	}

	data := map[string]any{"Commands": schemas}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(w, "%-35s %s\n", "COMMAND", "SUMMARY")
		for _, s := range schemas {
			fmt.Fprintf(w, "%-35s %s\n", s.Name, s.Summary)
		}
	}), nil
}

func renderSchemaText(w io.Writer, s CommandSchema) {
	fmt.Fprintf(w, "Command:         %s\n", s.Name)
	fmt.Fprintf(w, "Summary:         %s\n", s.Summary)
	fmt.Fprintf(w, "Backend:         %v\n", s.BackendSupport)
	fmt.Fprintf(w, "Mutation:        %v\n", s.Mutation)
	fmt.Fprintf(w, "CreatesResource: %v\n", s.CreatesResource)
	fmt.Fprintf(w, "Idempotency:     %s\n", s.Idempotency)
	fmt.Fprintf(w, "Interactive:     %v\n", s.Interactive)
	fmt.Fprintf(w, "RequiresAuth:    %v\n", s.RequiresAuth)
	fmt.Fprintf(w, "SupportsJson:    %v\n", s.SupportsJson)
	fmt.Fprintf(w, "SupportsNdjson:  %v\n", s.SupportsNdjson)
	fmt.Fprintf(w, "SupportsJq:      %v\n", s.SupportsJq)
	fmt.Fprintf(w, "SupportsRequest: %v\n", s.SupportsRequest)
	if len(s.Args) > 0 {
		fmt.Fprintln(w, "\nArgs:")
		for _, a := range s.Args {
			req := ""
			if a.Required {
				req = " (required)"
			}
			fmt.Fprintf(w, "  %-20s %s%s\n", a.Name, a.Type, req)
		}
	}
	if len(s.Flags) > 0 {
		fmt.Fprintln(w, "\nFlags:")
		for _, f := range s.Flags {
			sh := ""
			if f.Shorthand != "" {
				sh = fmt.Sprintf(" (-%s)", f.Shorthand)
			}
			fmt.Fprintf(w, "  --%s%s  %s\n", f.Name, sh, f.Type)
		}
	}
}

func getAllSchemas() []CommandSchema {
	min1 := 1
	return []CommandSchema{
		{
			Name: "instance.create", Summary: "Create a new sandbox instance",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: true, CreatesResource: true,
			Idempotency: "client_token_cloud_only", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: true,
			RequestSchema: &RequestSchema{
				Type: "object", AdditionalProperties: false,
				Required: []string{"ToolName"},
				Properties: map[string]PropertySchema{
					"ToolName":       {Type: "string"},
					"ToolId":         {Type: "string"},
					"TimeoutSeconds": {Type: "integer", Minimum: &min1},
					"AuthMode":       {Type: "enum", Values: []string{"DEFAULT", "TOKEN", "NONE", "PUBLIC"}},
					"MountOptions":   {Type: "array"},
					"ClientToken":    {Type: "string"},
				},
			},
			Args: []ArgSchema{},
			Flags: []FlagSchema{
				{Name: "tool-name", Shorthand: "t", Type: "string"},
				{Name: "tool-id", Type: "string"},
				{Name: "timeout", Type: "integer"},
				{Name: "mount-option", Type: "string_array"},
				{Name: "auth-mode", Type: "enum", Values: []string{"DEFAULT", "TOKEN", "NONE", "PUBLIC"}},
				{Name: "client-token", Type: "string"},
				{Name: "request", Type: "string"},
			},
			Output: "Instance", Failures: []string{"INVALID_TOOL", "CLIENT_TOKEN_CONFLICT", "CLIENT_TOKEN_UNSUPPORTED"},
		},
		{
			Name: "instance.list", Summary: "List sandbox instances",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{},
			Flags: []FlagSchema{
				{Name: "tool-id", Type: "string"},
				{Name: "status", Type: "string"},
				{Name: "offset", Type: "integer"},
				{Name: "limit", Type: "integer"},
			},
			Output: "InstanceList", Failures: []string{"AUTH_FAILED"},
		},
		{
			Name: "instance.get", Summary: "Get instance details",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
			Output: "Instance", Failures: []string{"INSTANCE_NOT_FOUND"},
		},
		{
			Name: "instance.delete", Summary: "Delete sandbox instances",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: true, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
			Flags: []FlagSchema{{Name: "ignore-not-found", Type: "bool"}},
			Output: "DeleteResult", Failures: []string{"INSTANCE_NOT_FOUND"},
		},
		{
			Name: "instance.code.run", Summary: "Execute code in an existing sandbox instance",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: true, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
			Flags: []FlagSchema{
				{Name: "code", Shorthand: "c", Type: "string"},
				{Name: "file", Shorthand: "f", Type: "string_array"},
				{Name: "language", Shorthand: "l", Type: "enum", Values: []string{"python", "javascript", "typescript", "r", "java", "bash"}},
				{Name: "stream", Shorthand: "s", Type: "bool", IncompatibleWith: []string{"output=json"}, AllowsOutput: []string{"text", "ndjson"}},
			},
			Output: "RunResult", Failures: []string{"MISSING_INSTANCE", "REMOTE_CODE_FAILED"},
		},
		{
			Name: "instance.exec", Summary: "Execute command in an existing sandbox instance",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: true, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
			Flags: []FlagSchema{
				{Name: "stream", Shorthand: "s", Type: "bool"},
				{Name: "cwd", Type: "string"},
				{Name: "env", Type: "string_array"},
				{Name: "user", Type: "string"},
			},
			Output: "ExecResult", Failures: []string{"MISSING_INSTANCE", "REMOTE_COMMAND_FAILED"},
		},
		{
			Name: "instance.file.upload", Summary: "Upload file to sandbox instance",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: true, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{
				{Name: "InstanceId", Type: "string", Required: true},
				{Name: "LocalPath", Type: "string", Required: true},
				{Name: "RemotePath", Type: "string", Required: true},
			},
			Flags:  []FlagSchema{{Name: "user", Type: "string"}},
			Output: "FileUploadResult", Failures: []string{"MISSING_INSTANCE"},
		},
		{
			Name: "instance.file.download", Summary: "Download file from sandbox instance",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{
				{Name: "InstanceId", Type: "string", Required: true},
				{Name: "RemotePath", Type: "string", Required: true},
				{Name: "LocalPath", Type: "string", Required: true},
			},
			Flags:  []FlagSchema{{Name: "user", Type: "string"}},
			Output: "FileDownloadResult", Failures: []string{"MISSING_INSTANCE"},
		},
		{
			Name: "instance.login", Summary: "Login to instance via terminal",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: true,
			RequiresAuth: true, SupportsJson: false, SupportsNdjson: false, SupportsJq: false,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
		},
		{
			Name: "instance.browser.vnc", Summary: "Show VNC URL for browser sandbox",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
			Flags: []FlagSchema{
				{Name: "port", Type: "integer"},
				{Name: "no-browser", Type: "bool"},
			},
			Output: "BrowserUrls",
		},
		{
			Name: "instance.proxy", Summary: "Forward a sandbox port to localhost",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: true,
			RequiresAuth: true, SupportsJson: false, SupportsNdjson: false, SupportsJq: false,
			SupportsRequest: false,
			Args: []ArgSchema{
				{Name: "InstanceId", Type: "string", Required: true},
				{Name: "PortSpec", Type: "string", Required: true},
			},
			Flags: []FlagSchema{
				{Name: "address", Type: "string"},
				{Name: "verbose", Type: "bool"},
			},
		},
		{
			Name: "instance.mobile.connect", Summary: "Connect to mobile sandbox",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
		},
		{
			Name: "instance.mobile.disconnect", Summary: "Disconnect from mobile sandbox",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: false}},
			Flags: []FlagSchema{{Name: "all", Type: "bool"}},
		},
		{
			Name: "instance.mobile.list", Summary: "List active mobile sandbox connections",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "instance.mobile.adb", Summary: "Execute adb command on mobile sandbox",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "InstanceId", Type: "string", Required: true}},
		},
		{
			Name: "tool.create", Summary: "Create a new sandbox tool",
			BackendSupport: []string{"cloud"}, Mutation: true, CreatesResource: true,
			Idempotency: "client_token", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: true,
			RequestSchema: &RequestSchema{
				Type: "object", AdditionalProperties: false,
				Required: []string{"Name", "Type"},
				Properties: map[string]PropertySchema{
					"Name":          {Type: "string"},
					"Type":          {Type: "string"},
					"Description":   {Type: "string"},
					"NetworkMode":   {Type: "enum", Values: []string{"PUBLIC", "VPC", "SANDBOX", "INTERNAL_SERVICE"}},
					"Tags":           {Type: "object"},
					"StorageMounts":  {Type: "array"},
					"VpcConfig":      {Type: "object"},
					"ClientToken":    {Type: "string"},
					"DefaultTimeout": {Type: "string"},
					"RoleArn":        {Type: "string"},
				},
			},
			Flags: []FlagSchema{
				{Name: "name", Shorthand: "n", Type: "string"},
				{Name: "type", Shorthand: "t", Type: "string"},
				{Name: "description", Shorthand: "d", Type: "string"},
				{Name: "timeout", Type: "string"},
				{Name: "network", Type: "enum", Values: []string{"PUBLIC", "VPC", "SANDBOX", "INTERNAL_SERVICE"}},
				{Name: "vpc-subnet", Type: "string_array"},
				{Name: "vpc-sg", Type: "string_array"},
				{Name: "tag", Type: "string_array"},
				{Name: "role-arn", Type: "string"},
				{Name: "mount", Type: "string_array"},
				{Name: "client-token", Type: "string"},
				{Name: "request", Type: "string"},
			},
			Output: "Tool", Failures: []string{"BACKEND_UNSUPPORTED", "CLIENT_TOKEN_CONFLICT"},
		},
		{
			Name: "tool.list", Summary: "List sandbox tools",
			BackendSupport: []string{"cloud"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Flags: []FlagSchema{
				{Name: "id", Type: "string_array"},
				{Name: "status", Type: "string"},
				{Name: "type", Type: "string"},
				{Name: "created-since", Type: "string"},
				{Name: "created-since-time", Type: "string"},
				{Name: "tag", Type: "string_array"},
				{Name: "offset", Type: "integer"},
				{Name: "limit", Type: "integer"},
			},
			Output: "ToolList",
		},
		{
			Name: "tool.get", Summary: "Get tool details",
			BackendSupport: []string{"cloud"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "ToolId", Type: "string", Required: true}},
		},
		{
			Name: "tool.update", Summary: "Update a sandbox tool",
			BackendSupport: []string{"cloud"}, Mutation: true, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: true,
			RequestSchema: &RequestSchema{
				Type: "object", AdditionalProperties: false,
				Properties: map[string]PropertySchema{
					"Description": {Type: "string"},
					"NetworkMode": {Type: "string"},
					"Tags":        {Type: "object"},
				},
			},
			Args: []ArgSchema{{Name: "ToolId", Type: "string", Required: true}},
			Flags: []FlagSchema{
				{Name: "description", Shorthand: "d", Type: "string"},
				{Name: "network", Type: "string"},
				{Name: "tag", Type: "string_array"},
				{Name: "clear-tags", Type: "bool"},
				{Name: "request", Type: "string"},
			},
		},
		{
			Name: "tool.delete", Summary: "Delete sandbox tools",
			BackendSupport: []string{"cloud"}, Mutation: true, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "ToolId", Type: "string", Required: true}},
		},
		{
			Name: "apikey.create", Summary: "Create a new API key",
			BackendSupport: []string{"cloud"}, Mutation: true, CreatesResource: true,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Flags: []FlagSchema{{Name: "name", Shorthand: "n", Type: "string"}},
		},
		{
			Name: "apikey.list", Summary: "List API keys",
			BackendSupport: []string{"cloud"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "apikey.delete", Summary: "Delete an API key",
			BackendSupport: []string{"cloud"}, Mutation: true, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: true, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
			Args: []ArgSchema{{Name: "KeyId", Type: "string", Required: true}},
		},
		{
			Name: "status", Summary: "Show current CLI configuration status",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "capabilities", Summary: "Show available commands for current environment",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "schema", Summary: "Show command schema for machine consumption",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "doctor", Summary: "Diagnose CLI configuration issues",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "version", Summary: "Print version information",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: true, SupportsNdjson: false, SupportsJq: true,
			SupportsRequest: false,
		},
		{
			Name: "docs", Summary: "Generate documentation",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: false, SupportsNdjson: false, SupportsJq: false,
			SupportsRequest: false,
		},
		{
			Name: "completion", Summary: "Generate shell completion script",
			BackendSupport: []string{"cloud", "e2b"}, Mutation: false, CreatesResource: false,
			Idempotency: "none", SupportsDryRun: false, Interactive: false,
			RequiresAuth: false, SupportsJson: false, SupportsNdjson: false, SupportsJq: false,
			SupportsRequest: false,
		},
	}
}
