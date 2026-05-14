package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	completionCmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion scripts for ags.

Examples:
  # Bash (add to ~/.bashrc)
  ags completion bash > /etc/bash_completion.d/ags

  # Zsh (add to ~/.zshrc)
  ags completion zsh > "${fpath[1]}/_ags"

  # Fish
  ags completion fish > ~/.config/fish/completions/ags.fish

  # PowerShell
  ags completion powershell > ags.ps1`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	}
	completionCmd.RunE = WrapNoJSON(func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(ios.Out)
		case "zsh":
			return rootCmd.GenZshCompletion(ios.Out)
		case "fish":
			return rootCmd.GenFishCompletion(ios.Out, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(ios.Out)
		default:
			return exitError(2, nil)
		}
	})
	rootCmd.AddCommand(completionCmd)
}
