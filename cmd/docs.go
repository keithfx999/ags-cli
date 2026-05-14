package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsOutputDir string

var docsCmd = &cobra.Command{
	Use:    "docs",
	Short:  "Generate documentation",
	Long:   `Generate documentation in various formats (man, markdown).`,
	Hidden: true,
	RunE: WrapNoJSON(func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}),
}

func init() {
	rootCmd.AddCommand(docsCmd)

	manCmd := &cobra.Command{
		Use:   "man",
		Short: "Generate man pages",
		Long: `Generate man pages for all commands.

Examples:
  ags docs man
  ags docs man --dir /tmp/ags-man`,
	}
	manCmd.RunE = WrapNoJSON(runDocsMan)
	manCmd.Flags().StringVar(&docsOutputDir, "dir", "man", "Output directory for man pages")
	docsCmd.AddCommand(manCmd)

	mdCmd := &cobra.Command{
		Use:     "markdown",
		Aliases: []string{"md"},
		Short:   "Generate markdown documentation",
		Long: `Generate markdown documentation for all commands.

Examples:
  ags docs markdown
  ags docs markdown --dir ./my-docs`,
	}
	mdCmd.RunE = WrapNoJSON(runDocsMarkdown)
	mdCmd.Flags().StringVar(&docsOutputDir, "dir", "docs/cmd", "Output directory for markdown docs")
	docsCmd.AddCommand(mdCmd)
}

func runDocsMan(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	header := &doc.GenManHeader{
		Title: "AGS", Section: "1", Source: "AGS CLI", Manual: "AGS Manual",
	}

	if versionCmd != nil {
		versionCmd.Hidden = true
		defer func() { versionCmd.Hidden = false }()
	}

	if err := doc.GenManTree(rootCmd, header, docsOutputDir); err != nil {
		return fmt.Errorf("failed to generate man pages: %w", err)
	}

	files, _ := filepath.Glob(filepath.Join(docsOutputDir, "*.1"))
	fmt.Fprintf(ios.Out, "Generated %d man pages in %s/\n", len(files), docsOutputDir)
	return nil
}

func runDocsMarkdown(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if versionCmd != nil {
		versionCmd.Hidden = true
		defer func() { versionCmd.Hidden = false }()
	}

	if err := doc.GenMarkdownTree(rootCmd, docsOutputDir); err != nil {
		return fmt.Errorf("failed to generate markdown docs: %w", err)
	}

	files, _ := filepath.Glob(filepath.Join(docsOutputDir, "*.md"))
	fmt.Fprintf(ios.Out, "Generated %d markdown files in %s/\n", len(files), docsOutputDir)
	return nil
}
