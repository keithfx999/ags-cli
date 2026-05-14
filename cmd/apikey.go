package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var apikeyName string

func apikeyCreateFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	result, err := apiClient.CreateAPIKey(ctx, apikeyName)
	if err != nil {
		return nil, err
	}

	data := map[string]any{
		"KeyId":  result.KeyID,
		"Name":   result.Name,
		"ApiKey": result.APIKey,
	}
	return OKWithEffects(data, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "API key created: %s\n", result.KeyID)
		fmt.Fprintf(ios.ErrOut, "Warning: Save this API key securely - it will not be shown again!\n")
		printKV(w, []KeyValue{
			{Key: "KeyID", Value: result.KeyID},
			{Key: "Name", Value: result.Name},
			{Key: "APIKey", Value: result.APIKey},
		})
	}, output.Effect{Kind: "create", Resource: "apikey", Id: result.KeyID}), nil
}

func apikeyListFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	keys, err := apiClient.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, len(keys))
	for i, k := range keys {
		items[i] = map[string]any{
			"KeyId": k.KeyID, "Name": k.Name, "Status": k.Status,
			"MaskedKey": k.MaskedKey, "CreatedAt": k.CreatedAt,
		}
	}
	data := map[string]any{"Items": items}

	return OK(data, func(w io.Writer) {
		if len(keys) == 0 {
			fmt.Fprintln(ios.ErrOut, "No API keys found")
			return
		}
		headers := []string{"KEY ID", "NAME", "STATUS", "MASKED KEY", "CREATED"}
		rows := make([][]string, len(keys))
		for i, k := range keys {
			rows[i] = []string{k.KeyID, k.Name, k.Status, k.MaskedKey, k.CreatedAt}
		}
		printTable(w, headers, rows)
	}), nil
}

func apikeyDeleteFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	if err := requireCloudBackend(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	keyID := args[0]
	if err := config.Validate(); err != nil {
		return nil, err
	}

	apiClient, err := newControlPlaneClient(config.GetBackend())
	if err != nil {
		return nil, err
	}

	if err := apiClient.DeleteAPIKey(ctx, keyID); err != nil {
		return nil, err
	}

	return OKWithEffects(map[string]any{"KeyId": keyID}, func(w io.Writer) {
		fmt.Fprintf(ios.ErrOut, "API key deleted: %s\n", keyID)
	}, output.Effect{Kind: "delete", Resource: "apikey", Id: keyID}), nil
}

func init() {
	addAPIKeyCommand(rootCmd)
}

func addAPIKeyCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "apikey",
		Aliases: []string{"ak", "key"},
		Short:   "Manage API keys",
		Long:    `Manage API keys for Agent Sandbox. Only available with cloud backend.`,
	}

	createCmd := &cobra.Command{Use: "create", Short: "Create a new API key"}
	createCmd.RunE = Wrap("apikey.create", apikeyCreateFn)
	createCmd.Flags().StringVarP(&apikeyName, "name", "n", "", "Name for the API key (required)")
	_ = createCmd.MarkFlagRequired("name")
	cmd.AddCommand(createCmd)

	listCmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List API keys"}
	listCmd.RunE = Wrap("apikey.list", apikeyListFn)
	cmd.AddCommand(listCmd)

	deleteCmd := &cobra.Command{Use: "delete <key-id>", Aliases: []string{"rm", "del"}, Short: "Delete an API key", Args: cobra.ExactArgs(1)}
	deleteCmd.RunE = Wrap("apikey.delete", apikeyDeleteFn)
	cmd.AddCommand(deleteCmd)

	parent.AddCommand(cmd)
}
