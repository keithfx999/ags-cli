package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/command"
	"github.com/spf13/cobra"
)

var (
	execStream bool
	execCwd    string
	execEnv    []string
	execUser   string
)

func instanceExecFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]

	if err := validateNDJSONOnlyForStream(execStream); err != nil {
		return nil, err
	}
	if execStream {
		if err := validateStreamNotJSON(); err != nil {
			return nil, err
		}
	}

	// Stream + NDJSON: create writer immediately so all errors produce a failed event
	if execStream && isNDJSON() {
		nw := output.NewNDJSONWriter(ios.Out, "instance.exec")
		_ = nw.WriteStarted(map[string]string{"InstanceId": instanceID})

		dashPos := cmd.ArgsLenAtDash()
		if dashPos != 1 || len(args) <= 1 {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: "usage: ags instance exec <instance-id> -- <command> [args...]\nUse '--' immediately after <instance-id>"})
			return StreamDone(output.ExitUsage), nil
		}
		remoteArgs := args[1:]
		cmdStr := strings.Join(remoteArgs, " ")

		envs := make(map[string]string)
		for _, env := range execEnv {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				_ = nw.WriteFailed(nil, &output.Failure{Code: "VALIDATION_ERROR", Kind: output.KindUsage, Message: fmt.Sprintf("invalid environment variable format: %s (expected KEY=VALUE)", env)})
				return StreamDone(output.ExitUsage), nil
			}
			envs[parts[0]] = parts[1]
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
		procConfig := &command.ProcessConfig{User: resolveUser(execUser), Envs: envs}
		if execCwd != "" {
			procConfig.Cwd = &execCwd
		}
		callbacks := &command.OnOutputConfig{
			OnStdout: func(data []byte) { _ = nw.WriteStdout(string(data)) },
			OnStderr: func(data []byte) { _ = nw.WriteStderr(string(data)) },
		}
		result, err := sandbox.Commands.Run(ctx, cmdStr, procConfig, callbacks)
		if err != nil {
			_ = nw.WriteFailed(nil, &output.Failure{Code: "EXECUTION_ERROR", Kind: output.KindGenericError, Message: err.Error()})
			return StreamDone(1), nil
		}
		if result.ExitCode != 0 {
			// AC12: remote details in Data only; Failure nil for remote execution errors
			_ = nw.WriteFailed(
				map[string]any{"ExitCode": int(result.ExitCode)},
				nil,
			)
			return StreamDone(int(result.ExitCode)), nil
		}
		_ = nw.WriteCompleted(map[string]any{"ExitCode": 0})
		return StreamDone(0), nil
	}

	dashPos := cmd.ArgsLenAtDash()
	if dashPos != 1 || len(args) <= 1 {
		return nil, exitError(output.ExitUsage, fmt.Errorf("usage: ags instance exec <instance-id> -- <command> [args...]\nUse '--' immediately after <instance-id> to separate the remote command from flags"))
	}

	remoteArgs := args[1:]
	cmdStr := strings.Join(remoteArgs, " ")

	envs := make(map[string]string)
	for _, env := range execEnv {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", env)
		}
		envs[parts[0]] = parts[1]
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	sandbox, err := ConnectSandboxWithCache(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to instance %s: %w", instanceID, err)
	}

	procConfig := &command.ProcessConfig{
		User: resolveUser(execUser),
		Envs: envs,
	}
	if execCwd != "" {
		procConfig.Cwd = &execCwd
	}

	// Stream + text
	if execStream {
		callbacks := &command.OnOutputConfig{
			OnStdout: func(data []byte) { fmt.Fprint(ios.Out, string(data)) },
			OnStderr: func(data []byte) { fmt.Fprint(ios.ErrOut, string(data)) },
		}

		result, err := sandbox.Commands.Run(ctx, cmdStr, procConfig, callbacks)
		if err != nil {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}

		return StreamDone(int(result.ExitCode)), nil
	}

	// Non-stream execution
	result, err := sandbox.Commands.Run(ctx, cmdStr, procConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}

	data := &output.ExecData{
		Stdout:   string(result.Stdout),
		Stderr:   string(result.Stderr),
		ExitCode: int(result.ExitCode),
	}

	if result.ExitCode != 0 {
		// AC12: remote details in Data only; Failure nil for remote execution errors
		// AC14: exit code passthrough
		return &CmdResult{
			Data:     data,
			ExitCode: int(result.ExitCode),
			RenderText: func(w io.Writer) {
				if len(result.Stdout) > 0 {
					fmt.Fprint(w, string(result.Stdout))
				}
				if len(result.Stderr) > 0 {
					fmt.Fprintln(ios.ErrOut, "--- stderr ---")
					fmt.Fprint(ios.ErrOut, string(result.Stderr))
				}
				if result.Error != nil {
					fmt.Fprintf(ios.ErrOut, "--- error ---\n%s\n", *result.Error)
				}
			},
		}, nil
	}

	return OK(data, func(w io.Writer) {
		if len(result.Stdout) > 0 {
			fmt.Fprint(w, string(result.Stdout))
		}
		if len(result.Stderr) > 0 {
			fmt.Fprintln(ios.ErrOut, "--- stderr ---")
			fmt.Fprint(ios.ErrOut, string(result.Stderr))
		}
	}), nil
}

// addInstanceExecCommand registers `instance exec` under the given parent.
func addInstanceExecCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "exec <instance-id> -- <command> [args...]",
		Short: "Execute a shell command in an instance",
		Long: `Execute a shell command in an existing sandbox instance.

Use '--' to separate flags from the remote command.

Examples:
  ags instance exec <id> -- ls -la
  ags instance exec <id> -s -- ping -c 5 localhost
  ags instance exec <id> --env FOO=bar -- echo \$FOO
  ags instance exec <id> --cwd /home/user -- pwd`,
		Args: cobra.MinimumNArgs(1),
	}

	cmd.Flags().BoolVarP(&execStream, "stream", "s", false, "Stream output in real-time")
	cmd.Flags().StringVar(&execCwd, "cwd", "", "Working directory")
	cmd.Flags().StringArrayVar(&execEnv, "env", nil, "Environment variables (KEY=VALUE format)")
	cmd.Flags().StringVar(&execUser, "user", "", "User to run commands as (default: \"user\")")
	cmd.RunE = Wrap("instance.exec", instanceExecFn)

	parent.AddCommand(cmd)
}
