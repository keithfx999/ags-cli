package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	toolcode "github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/code"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	runCode     string
	runFiles    []string
	runLanguage string
	runStream   bool
)

func instanceCodeRunFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]

	if err := validateNDJSONOnlyForStream(runStream); err != nil {
		return nil, err
	}
	if runStream {
		if err := validateStreamNotJSON(); err != nil {
			return nil, err
		}
	}

	// Stream + NDJSON: create writer immediately so all subsequent errors produce a failed event
	if runStream && isNDJSON() {
		nw := output.NewNDJSONWriter(ios.Out, "instance.code.run")
		_ = nw.WriteStarted(map[string]string{"InstanceId": instanceID})

		if err := validateRunFlags(); err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: err.Error()})
			return StreamDone(output.ExitUsage), nil
		}
		if runCode != "" && len(runFiles) > 0 {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: "cannot use both -c and -f flags"})
			return StreamDone(output.ExitUsage), nil
		}
		codeStr, err := resolveCodeInput(runCode, runFiles)
		if err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: err.Error()})
			return StreamDone(output.ExitUsage), nil
		}
		if codeStr == "" {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: "no code provided: use -c, -f, or pipe via stdin"})
			return StreamDone(output.ExitUsage), nil
		}

		if err := config.Validate(); err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "CONFIG_ERROR", Kind: output.KindUsage, Message: err.Error()})
			return StreamDone(output.ExitUsage), nil
		}
		sandbox, err := ConnectSandboxWithCache(ctx, instanceID)
		if err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "CONNECTION_ERROR", Kind: output.KindGenericError, Message: err.Error()})
			return StreamDone(1), nil
		}
		runConfig := &toolcode.RunCodeConfig{Language: runLanguage}
		callbacks := &toolcode.OnOutputConfig{
			OnStdout: func(s string) { _ = nw.WriteStdout(s) },
			OnStderr: func(s string) { _ = nw.WriteStderr(s) },
		}
		result, err := sandbox.Code.RunCode(ctx, codeStr, runConfig, callbacks)
		if err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "EXECUTION_ERROR", Kind: output.KindGenericError, Message: err.Error()})
			return StreamDone(1), nil
		}
		if result.Error != nil {
			// AC12: remote details in Data only; Failure nil for remote execution errors
			_ = nw.WriteFailed(
				map[string]any{"Error": map[string]any{"Name": result.Error.Name, "Value": result.Error.Value, "Traceback": result.Error.Traceback}},
				nil)
			return StreamDone(output.ExitRemoteExecFailed), nil
		}
		_ = nw.WriteCompleted(map[string]any{"ExecutionCount": 1})
		return StreamDone(0), nil
	}

	if err := validateRunFlags(); err != nil {
		return nil, err
	}
	if runCode != "" && len(runFiles) > 0 {
		return nil, fmt.Errorf("cannot use both -c and -f flags")
	}

	codeStr, err := resolveCodeInput(runCode, runFiles)
	if err != nil {
		return nil, err
	}
	if codeStr == "" {
		return nil, exitError(2, fmt.Errorf("no code provided: use -c, -f, or pipe via stdin"))
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	sandbox, err := ConnectSandboxWithCache(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to instance %s: %w", instanceID, err)
	}

	runConfig := &toolcode.RunCodeConfig{
		Language: runLanguage,
	}

	// Stream + text
	if runStream {
		callbacks := &toolcode.OnOutputConfig{
			OnStdout: func(s string) { fmt.Fprint(ios.Out, s) },
			OnStderr: func(s string) { fmt.Fprint(ios.ErrOut, s) },
		}
		result, err := sandbox.Code.RunCode(ctx, codeStr, runConfig, callbacks)
		if err != nil {
			return nil, err
		}
		if result.Error != nil {
			fmt.Fprintf(ios.ErrOut, "\n--- error ---\n%s: %s\n", result.Error.Name, result.Error.Value)
			if result.Error.Traceback != "" {
				fmt.Fprintln(ios.ErrOut, result.Error.Traceback)
			}
			return StreamDone(output.ExitRemoteExecFailed), nil
		}
		return StreamDone(0), nil
	}

	// Non-stream execution
	result, err := sandbox.Code.RunCode(ctx, codeStr, runConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute code: %w", err)
	}

	codeData := &output.CodeRunData{
		Stdout:         strings.Join(result.Logs.Stdout, ""),
		Stderr:         strings.Join(result.Logs.Stderr, ""),
		Results:        convertResults(result.Results),
		Error:          nil,
		ExecutionCount: 1,
	}

	textFn := func(w io.Writer) {
		for _, line := range result.Logs.Stdout {
			fmt.Fprint(w, line)
		}
		if len(result.Logs.Stderr) > 0 {
			fmt.Fprintln(ios.ErrOut, "\n--- stderr ---")
			for _, line := range result.Logs.Stderr {
				fmt.Fprint(ios.ErrOut, line)
			}
		}
	}

	if result.Error != nil {
		codeData.Error = map[string]any{
			"Name":      result.Error.Name,
			"Value":     result.Error.Value,
			"Traceback": result.Error.Traceback,
		}
		// AC12: remote error details stay in Data.Error only; Failure is nil.
		// AC15: exit 11 for remote code exceptions.
		// Wrap sets Status:"failed" when ExitCode != 0 even with nil Failure.
		return &CmdResult{
			Data:     codeData,
			ExitCode: output.ExitRemoteExecFailed,
			RenderText: func(w io.Writer) {
				textFn(w)
				fmt.Fprintf(ios.ErrOut, "\n--- error ---\n%s: %s\n", result.Error.Name, result.Error.Value)
				if result.Error.Traceback != "" {
					fmt.Fprintln(ios.ErrOut, result.Error.Traceback)
				}
			},
		}, nil
	}

	return OK(codeData, textFn), nil
}

func validateRunFlags() error {
	switch runLanguage {
	case "python", "javascript", "typescript", "r", "java", "bash":
		return nil
	default:
		return fmt.Errorf("unsupported language: %s (must be one of: python, javascript, typescript, r, java, bash)", runLanguage)
	}
}

// resolveCodeInput resolves code from -c flag, -f flag, or stdin (three choose one).
func resolveCodeInput(codeFlag string, files []string) (string, error) {
	if codeFlag != "" {
		return codeFlag, nil
	}
	if len(files) > 0 {
		if len(files) > 1 {
			return "", fmt.Errorf("only one file is supported; got %d", len(files))
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", files[0], err)
		}
		return string(data), nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(data), nil
	}

	return "", nil
}

// convertResults converts SDK results to output format
func convertResults(sdkResults []toolcode.Result) []map[string]any {
	results := make([]map[string]any, 0, len(sdkResults))
	for _, r := range sdkResults {
		m := make(map[string]any)
		if r.Text != nil {
			m["Text"] = *r.Text
		}
		if r.Html != nil {
			m["Html"] = *r.Html
		}
		if r.Markdown != nil {
			m["Markdown"] = *r.Markdown
		}
		if r.Svg != nil {
			m["Svg"] = *r.Svg
		}
		if r.Png != nil {
			m["Png"] = *r.Png
		}
		if r.Jpeg != nil {
			m["Jpeg"] = *r.Jpeg
		}
		if r.Pdf != nil {
			m["Pdf"] = *r.Pdf
		}
		if r.Latex != nil {
			m["Latex"] = *r.Latex
		}
		if r.Json != nil {
			m["Json"] = r.Json
		}
		if r.Javascript != nil {
			m["Javascript"] = *r.Javascript
		}
		if r.Data != nil {
			m["Data"] = r.Data
		}
		if r.Chart != nil {
			m["Chart"] = r.Chart
		}
		m["IsMainResult"] = r.IsMainResult
		if r.Extra != nil {
			m["Extra"] = r.Extra
		}
		if len(m) > 1 {
			results = append(results, m)
		}
	}
	return results
}

// addInstanceCodeRunCommand registers `instance code run` under the given parent.
func addInstanceCodeRunCommand(parent *cobra.Command) {
	codeCmd := &cobra.Command{
		Use:   "code",
		Short: "Code execution commands",
	}

	runCmd := &cobra.Command{
		Use:   "run <instance-id>",
		Short: "Execute code in an instance",
		Long: `Execute code in an existing sandbox instance.

Code can be provided in three ways:
  1. Direct string: ags instance code run <id> -c "print('Hello')"
  2. From file: ags instance code run <id> -f script.py
  3. From pipe: echo "print('Hello')" | ags instance code run <id>

Supported languages: python (default), javascript, typescript, r, java, bash`,
		Args: cobra.ExactArgs(1),
	}

	runCmd.Flags().StringVarP(&runCode, "code", "c", "", "Code to execute")
	runCmd.Flags().StringArrayVarP(&runFiles, "file", "f", nil, "File containing code to execute")
	runCmd.Flags().StringVarP(&runLanguage, "language", "l", "python", "Programming language (python, javascript, typescript, r, java, bash)")
	runCmd.Flags().BoolVarP(&runStream, "stream", "s", false, "Stream output in real-time")
	runCmd.RunE = Wrap("instance.code.run", instanceCodeRunFn)

	codeCmd.AddCommand(runCmd)
	parent.AddCommand(codeCmd)
}
