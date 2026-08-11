package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/commands"
)

func TestSchemaMetadataInvariantsFromRegistryDescriptors(t *testing.T) {
	descriptors := registryDescriptors(t)
	listed := schemaListByCommand(t)

	for _, desc := range descriptors {
		if desc.Spec.Hidden {
			continue
		}
		want := schemaMetadataExpectation(desc)
		if !want.hasContract {
			continue
		}

		fromCommand := metadataFromCommandSchema(schemaForCommand(t, desc.Spec.ID))
		requireSchemaMetadata(t, desc.Spec.ID, "agr schema <command>", fromCommand, want)

		fromList, ok := listed[desc.Spec.ID]
		if !ok {
			t.Fatalf("schema list missing registered command %q", desc.Spec.ID)
		}
		requireSchemaMetadata(t, desc.Spec.ID, "agr schema", fromList, want)
	}
}

func TestRegistryDescriptorSourceContracts(t *testing.T) {
	for _, desc := range registryDescriptors(t) {
		switch desc.Source {
		case "mixed-api":
			if desc.Generated == nil {
				t.Fatalf("%s source mixed-api missing generated descriptor", desc.Spec.ID)
			}
			if desc.Generated.Source != "apicli" {
				t.Fatalf("%s generated source = %q, want apicli", desc.Spec.ID, desc.Generated.Source)
			}
			if desc.Generated.Spec.ID != desc.Spec.ID {
				t.Fatalf("%s generated id = %q, want %q", desc.Spec.ID, desc.Generated.Spec.ID, desc.Spec.ID)
			}
		case "workflow":
			if desc.Generated != nil {
				t.Fatalf("%s source workflow unexpectedly has generated descriptor", desc.Spec.ID)
			}
		case "apicli":
			if desc.Generated != nil {
				t.Fatalf("%s source apicli unexpectedly has generated descriptor", desc.Spec.ID)
			}
		default:
			t.Fatalf("%s source = %q, want apicli, mixed-api, or workflow", desc.Spec.ID, desc.Source)
		}
	}
}

type schemaMetadataContract struct {
	hasContract     bool
	mutation        bool
	createsResource bool
	requiresAuth    bool
}

func schemaMetadataExpectation(desc command.Descriptor) schemaMetadataContract {
	var want schemaMetadataContract
	for _, effect := range desc.Spec.Output.Effects {
		if effect == "" {
			continue
		}
		want.hasContract = true
		want.requiresAuth = true
		kind, _, _ := strings.Cut(effect, ":")
		switch kind {
		case "create":
			want.mutation = true
			want.createsResource = true
		case "delete", "update", "pause", "resume":
			want.mutation = true
		}
	}
	return want
}

type schemaMetadataSnapshot struct {
	Name            string
	Mutation        bool
	CreatesResource bool
	RequiresAuth    bool
}

func metadataFromCommandSchema(schema commandSchemaSnapshot) schemaMetadataSnapshot {
	return schemaMetadataSnapshot{
		Name:            schema.Name,
		Mutation:        schema.Mutation,
		CreatesResource: schema.CreatesResource,
		RequiresAuth:    schema.RequiresAuth,
	}
}

func requireSchemaMetadata(t *testing.T, commandID, source string, got schemaMetadataSnapshot, want schemaMetadataContract) {
	t.Helper()
	if got.RequiresAuth != want.requiresAuth {
		t.Fatalf("%s %s RequiresAuth = %v, want %v", source, commandID, got.RequiresAuth, want.requiresAuth)
	}
	if got.Mutation != want.mutation {
		t.Fatalf("%s %s Mutation = %v, want %v", source, commandID, got.Mutation, want.mutation)
	}
	if got.CreatesResource != want.createsResource {
		t.Fatalf("%s %s CreatesResource = %v, want %v", source, commandID, got.CreatesResource, want.createsResource)
	}
}

func registryDescriptors(t *testing.T) []command.Descriptor {
	t.Helper()
	registry, err := commands.Registry()
	if err != nil {
		t.Fatalf("build command registry: %v", err)
	}
	return registry.Descriptors()
}

func schemaListByCommand(t *testing.T) map[string]schemaMetadataSnapshot {
	t.Helper()
	output, err := runAGR(t, "schema", "-o", "json")
	if err != nil {
		t.Fatalf("schema list failed: %v\n%s", err, output)
	}
	jsonStart := strings.Index(output, "{")
	if jsonStart < 0 {
		t.Fatalf("schema output did not contain JSON\n%s", output)
	}
	var env struct {
		Data struct {
			Commands []schemaMetadataSnapshot `json:"Commands"`
		} `json:"Data"`
	}
	if err := json.Unmarshal([]byte(output[jsonStart:]), &env); err != nil {
		t.Fatalf("decode schema JSON: %v\n%s", err, output)
	}
	byCommand := map[string]schemaMetadataSnapshot{}
	for _, schema := range env.Data.Commands {
		byCommand[schema.Name] = schema
	}
	return byCommand
}
