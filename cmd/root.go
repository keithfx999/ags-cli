package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	backend        string
	outputFmt      string
	showVersion    bool
	region         string
	domain         string
	internal       bool
	apiKey         string
	secretID       string
	secretKey      string
	jqExpr         string
	nonInteractive bool
	noColor        bool
	configInitErr  error
)

var rootCmd = &cobra.Command{
	Use:           "ags",
	Short:         "AGS CLI - Agent Sandbox Command Line Interface",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `AGS CLI is a command line tool for managing Agent Sandbox tools and instances.

Examples:
  id=$(ags instance create -t code-interpreter-v1 -o json --jq '.Data.Id')
  ags instance code run "$id" -c "print('Hello, World!')"
  ags instance delete "$id"

  ags status
  ags doctor`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			return Wrap("version", versionFn)(cmd, args)
		}
		if isJSON() {
			return Wrap("schema", schemaFn)(cmd, []string{})
		}
		return cmd.Help()
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		initConfig()
		explicitJSON := outputFmt == "json" || hasRawOutputFlag("json")
		wantJSON := explicitJSON || config.GetOutput() == "json"
		wantNDJSON := outputFmt == "ndjson" || config.GetOutput() == "ndjson" || hasRawOutputFlag("ndjson")
		hasJQ := jqExpr != "" || hasRawFlag("--jq")

		if hasJQ && !explicitJSON {
			fmt.Fprintln(os.Stderr, "Error: --jq can only be used with -o json")
			os.Exit(output.ExitUsage)
		}
		if wantNDJSON {
			fmt.Fprintln(os.Stderr, "Error: -o ndjson is not supported with --help")
			os.Exit(output.ExitUsage)
		}
		if wantJSON {
			cmdID := canonicalCommandID(cmd)
			if cmdID == "" {
				_ = Wrap("schema", schemaFn)(cmd, []string{})
			} else {
				_ = Wrap("schema", schemaFn)(cmd, []string{cmdID})
			}
			return
		}
		defaultHelp(cmd, args)
	})

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		explicitJSON := outputFmt == "json" || hasRawOutputFlag("json")
		if jqExpr != "" && !explicitJSON {
			return exitError(output.ExitUsage, fmt.Errorf("--jq can only be used with -o json"))
		}
		if config.GetOutput() == "ndjson" && !isNDJSONAllowedCommand(cmd) {
			return exitError(output.ExitUsage, fmt.Errorf("-o ndjson is only supported with 'instance code run --stream' and 'instance exec --stream'"))
		}
		if shouldSkipConfigPreflight(cmd) {
			return nil
		}
		if cmd == rootCmd && showVersion {
			return nil
		}
		if configInitErr != nil {
			return configInitErr
		}
		return config.ValidateBasics()
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ags/config.toml)")
	rootCmd.PersistentFlags().StringVar(&backend, "backend", "", "API backend: e2b or cloud")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "", "output format: text, json, or ndjson")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version information")
	rootCmd.PersistentFlags().StringVar(&region, "region", "", "Region for API access (default: ap-guangzhou)")
	rootCmd.PersistentFlags().StringVar(&domain, "domain", "", "Base domain (default: tencentags.com)")
	rootCmd.PersistentFlags().BoolVar(&internal, "internal", false, "Use internal endpoints")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key")
	rootCmd.PersistentFlags().StringVar(&secretID, "secret-id", "", "Tencent Cloud SecretID")
	rootCmd.PersistentFlags().StringVar(&secretKey, "secret-key", "", "Tencent Cloud SecretKey")
	rootCmd.PersistentFlags().StringVar(&jqExpr, "jq", "", "jq expression (only with -o json)")
	rootCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "Disable interactive behaviors")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable ANSI color output")
}

func Execute() {
	initIOStreams()

	if hasRawFlag("--help") || hasRawFlag("-h") {
		if hasRawOutputFlag("json") {
			initConfig()
			config.SetOutput("json")
			for i, arg := range os.Args {
				if (arg == "--jq") && i+1 < len(os.Args) {
					jqExpr = os.Args[i+1]
					break
				}
				if strings.HasPrefix(arg, "--jq=") {
					jqExpr = strings.TrimPrefix(arg, "--jq=")
					break
				}
			}
			targetCmd, _, _ := rootCmd.Find(stripHelpAndOutputFlags(os.Args[1:]))
			cmdID := ""
			if targetCmd != nil && targetCmd != rootCmd {
				var parts []string
				for c := targetCmd; c != nil && c != rootCmd; c = c.Parent() {
					parts = append([]string{c.Name()}, parts...)
				}
				cmdID = strings.Join(parts, ".")
			}
			if cmdID != "" {
				for _, s := range getAllSchemas() {
					if s.Name == cmdID && !s.SupportsJson {
						fmt.Fprintf(os.Stderr, "Error: %s does not support -o json\n", cmdID)
						os.Exit(output.ExitUsage)
					}
				}
			}
			var schemaErr error
			if cmdID == "" {
				schemaErr = Wrap("schema", schemaFn)(rootCmd, []string{})
			} else {
				schemaErr = Wrap("schema", schemaFn)(rootCmd, []string{cmdID})
			}
			if schemaErr != nil {
				cliErr := output.ClassifyError(schemaErr)
				fmt.Fprintln(os.Stderr, "Error:", cliErr.Failure.Message)
				os.Exit(cliErr.ExitCode)
			}
			return
		}
	}

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		var envDone *envelopeAlreadyWritten
		if errors.As(err, &envDone) {
			os.Exit(envDone.code)
		}
		cliErr := output.ClassifyError(err)
		if cliErr.ExitCode == output.ExitGenericError && isCobraUsageError(err) {
			cliErr = output.NewUsageError("INVALID_USAGE", err.Error(), "")
		}
		if isJSON() {
			cmdID := canonicalCommandID(cmd)
			env := output.NewFailedEnvelope(cmdID, cliErr.Failure, config.GetBackend(), 0)
			if jqErr := output.RenderEnvelope(os.Stdout, env, jqExpr); jqErr != nil {
				jqFailure := output.NewUsageError("INVALID_JQ_EXPRESSION", jqErr.Error(), "Check your --jq expression syntax.")
				jqEnv := output.NewFailedEnvelope(cmdID, jqFailure.Failure, config.GetBackend(), 0)
				_ = output.RenderEnvelopeToStdout(jqEnv)
				os.Exit(output.ExitUsage)
			}
		} else {
			fmt.Fprintln(ios.ErrOut, "Error:", cliErr.Failure.Message)
		}
		os.Exit(cliErr.ExitCode)
	}
}

func canonicalCommandID(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil && c != rootCmd; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, ".")
}

func stripHelpAndOutputFlags(args []string) []string {
	var result []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--help" || a == "-h" {
			continue
		}
		if a == "-o" || a == "--output" {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "-o") || strings.HasPrefix(a, "--output=") {
			continue
		}
		if a == "--jq" {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "--jq=") {
			continue
		}
		result = append(result, a)
	}
	return result
}

func hasRawOutputFlag(value string) bool {
	for i, arg := range os.Args {
		if (arg == "-o" || arg == "--output") && i+1 < len(os.Args) && os.Args[i+1] == value {
			return true
		}
		if arg == "-o"+value || arg == "--output="+value {
			return true
		}
	}
	return false
}

func hasRawFlag(flag string) bool {
	for _, arg := range os.Args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func isCobraUsageError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "accepts ") ||
		strings.Contains(msg, "required") ||
		strings.Contains(msg, "requires ") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "unknown shorthand")
}

func isNDJSONAllowedCommand(cmd *cobra.Command) bool {
	return cmd.Name() == "run" || cmd.Name() == "exec"
}

func shouldSkipConfigPreflight(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "version", "completion", "docs", "status", "capabilities", "schema", "doctor":
			return true
		}
	}
	return false
}

func initConfig() {
	configInitErr = nil
	if cfgFile != "" {
		config.SetConfigFile(cfgFile)
	}
	if err := config.Init(); err != nil {
		configInitErr = err
	}
	// Merge env vars into interactive/color flags
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
	if os.Getenv("TERM") == "dumb" {
		noColor = true
		nonInteractive = true
	}
	if os.Getenv("AGS_NON_INTERACTIVE") == "1" {
		nonInteractive = true
	}

	if backend != "" {
		config.SetBackend(backend)
	}
	if outputFmt != "" {
		config.SetOutput(outputFmt)
	}
	if apiKey != "" {
		config.SetAPIKey(apiKey)
	}
	if secretID != "" {
		config.SetSecretID(secretID)
	}
	if secretKey != "" {
		config.SetSecretKey(secretKey)
	}
	if region != "" {
		config.SetRegion(region)
	}
	if domain != "" {
		config.SetDomain(domain)
	}
	if rootCmd.PersistentFlags().Changed("internal") {
		config.SetInternal(internal)
	}
}
