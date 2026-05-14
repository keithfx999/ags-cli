package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/filesystem"
	"github.com/spf13/cobra"
)

var fileUser string

func fileUploadFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]
	localPath := args[1]
	remotePath := args[2]

	if err := config.Validate(); err != nil {
		return nil, err
	}

	var reader io.Reader
	var localSize int64
	if localPath == "-" {
		reader = os.Stdin
		localSize = -1
	} else {
		info, err := os.Stat(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat local file: %w", err)
		}
		localSize = info.Size()
		f, err := os.Open(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open local file: %w", err)
		}
		defer func() { _ = f.Close() }()
		reader = f
	}

	sandbox, err := ConnectSandboxWithCache(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to instance %s: %w", instanceID, err)
	}

	info, err := sandbox.Files.Write(ctx, remotePath, reader, &filesystem.WriteConfig{User: resolveUser(fileUser)})
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	data := map[string]any{
		"Operation": "upload",
		"Path":      info.Path,
		"LocalPath": localPath,
		"Size":      localSize,
	}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "Uploaded %s -> %s\n", localPath, info.Path)
	}), nil
}

func fileDownloadFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	ctx := context.Background()
	instanceID := args[0]
	remotePath := args[1]
	localPath := args[2]

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if localPath == "-" && isJSON() {
		return nil, output.NewUsageError("STDOUT_CONFLICT",
			"cannot use -o json with stdout download (-)",
			"Use a file path instead of - when using -o json.")
	}

	sandbox, err := ConnectSandboxWithCache(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to instance %s: %w", instanceID, err)
	}

	reader, err := sandbox.Files.Read(ctx, remotePath, &filesystem.ReadConfig{User: resolveUser(fileUser)})
	if err != nil {
		return nil, fmt.Errorf("failed to read remote file: %w", err)
	}

	if localPath == "-" {
		_, _ = io.Copy(ios.Out, reader)
		return StreamDone(0), nil
	}

	f, err := os.Create(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	n, err := io.Copy(f, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to write local file: %w", err)
	}

	data := map[string]any{
		"Operation": "download",
		"Path":      remotePath,
		"LocalPath": localPath,
		"Size":      n,
	}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "Downloaded %s -> %s (%s)\n", remotePath, localPath, output.FormatSize(n))
	}), nil
}

func addFileCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File operations in sandbox",
		Long: `Upload and download files in a sandbox instance.

Examples:
  ags instance file upload <id> local.txt /home/user/remote.txt
  ags instance file download <id> /home/user/remote.txt local.txt
  echo "data" | ags instance file upload <id> - /home/user/data.txt
  ags instance file download <id> /home/user/data.txt -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	uploadCmd := &cobra.Command{
		Use:   "upload <instance-id> <local-path|-> <remote-path>",
		Short: "Upload a file to sandbox",
		Args:  cobra.ExactArgs(3),
	}
	uploadCmd.RunE = Wrap("instance.file.upload", fileUploadFn)
	uploadCmd.Flags().StringVar(&fileUser, "user", "", "User for file operations")
	cmd.AddCommand(uploadCmd)

	downloadCmd := &cobra.Command{
		Use:   "download <instance-id> <remote-path> <local-path|->",
		Short: "Download a file from sandbox",
		Args:  cobra.ExactArgs(3),
	}
	downloadCmd.RunE = Wrap("instance.file.download", fileDownloadFn)
	downloadCmd.Flags().StringVar(&fileUser, "user", "", "User for file operations")
	cmd.AddCommand(downloadCmd)

	parent.AddCommand(cmd)
}

