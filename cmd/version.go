package cmd

import (
	"fmt"
	"io"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit hash, and build time of ags.`,
}

func init() {
	versionCmd.RunE = Wrap("version", versionFn)
	rootCmd.AddCommand(versionCmd)
}

func versionFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	data := &output.VersionData{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(w, "ags version %s\n", Version)
		fmt.Fprintf(w, "  commit: %s\n", Commit)
		fmt.Fprintf(w, "  built:  %s\n", BuildTime)
	}), nil
}

func SetVersionInfo(version, commit, buildTime string) {
	if version != "" {
		Version = version
	}
	if commit != "" {
		Commit = commit
	}
	if buildTime != "" {
		BuildTime = buildTime
	}
}
