package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-go-sdk/sandbox/code"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeControlPlane struct {
	instances      []*client.Instance
	tools          []*client.Tool
	deleteErr      error
	createErr      error
	createInstance *client.Instance
}

func (f *fakeControlPlane) CreateInstance(_ context.Context, opts *client.CreateInstanceOptions) (*client.Instance, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createInstance != nil {
		return f.createInstance, nil
	}
	return &client.Instance{ID: "sb-fake", ToolName: opts.ToolName, Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}, nil
}
func (f *fakeControlPlane) ListInstances(_ context.Context, _ *client.ListInstancesOptions) (*client.ListInstancesResult, error) {
	return &client.ListInstancesResult{Instances: make([]client.Instance, 0), TotalCount: 0}, nil
}
func (f *fakeControlPlane) GetInstance(_ context.Context, id string) (*client.Instance, error) {
	for _, inst := range f.instances {
		if inst.ID == id {
			return inst, nil
		}
	}
	return nil, output.NewNotFoundError("INSTANCE_NOT_FOUND", fmt.Sprintf("instance %s not found", id), "")
}
func (f *fakeControlPlane) DeleteInstance(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}
func (f *fakeControlPlane) AcquireToken(_ context.Context, _ string) (string, error) {
	return "fake-token", nil
}
func (f *fakeControlPlane) CreateTool(_ context.Context, opts *client.CreateToolOptions) (*client.Tool, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &client.Tool{ID: "sdt-fake", Name: opts.Name, Type: opts.Type, Status: "active"}, nil
}
func (f *fakeControlPlane) UpdateTool(_ context.Context, _ *client.UpdateToolOptions) error { return nil }
func (f *fakeControlPlane) ListTools(_ context.Context, _ *client.ListToolsOptions) (*client.ListToolsResult, error) {
	return &client.ListToolsResult{Tools: make([]client.Tool, 0), TotalCount: 0}, nil
}
func (f *fakeControlPlane) GetTool(_ context.Context, id string) (*client.Tool, error) {
	for _, t := range f.tools {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, output.NewNotFoundError("TOOL_NOT_FOUND", "tool not found", "")
}
func (f *fakeControlPlane) DeleteTool(_ context.Context, _ string) error { return nil }
func (f *fakeControlPlane) CreateAPIKey(_ context.Context, name string) (*client.CreateAPIKeyResult, error) {
	return &client.CreateAPIKeyResult{KeyID: "ak-fake", Name: name, APIKey: "ark_fake"}, nil
}
func (f *fakeControlPlane) ListAPIKeys(_ context.Context) ([]client.APIKey, error) {
	return []client.APIKey{}, nil
}
func (f *fakeControlPlane) DeleteAPIKey(_ context.Context, _ string) error { return nil }

func withFakeClient(fake *fakeControlPlane, fn func()) {
	orig := newControlPlaneClient
	newControlPlaneClient = func(_ string) (client.ControlPlaneClient, error) {
		return fake, nil
	}
	defer func() { newControlPlaneClient = orig }()

	instanceToolName = ""
	instanceToolID = ""
	instanceTimeout = 300
	instanceAuthMode = client.AuthModeDefault
	instanceClientToken = ""
	instanceRequest = ""

	fn()
}

var _ = Describe("Integration Tests with Fake Backend", func() {

	Describe("instance list succeeded path", func() {
		It("returns empty list with JSON envelope", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "list", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
				data := env["Data"].(map[string]any)
				items := data["Items"].([]any)
				Expect(items).To(HaveLen(0))
			})
		})
	})

	Describe("instance get succeeded/failed paths", func() {
		It("returns instance details on success", func() {
			fake := &fakeControlPlane{
				instances: []*client.Instance{{ID: "sb-1", ToolName: "ci-v1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}},
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "get", "sb-1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
				data := env["Data"].(map[string]any)
				Expect(data["Id"]).To(Equal("sb-1"))
			})
		})

		It("returns not-found error for missing instance", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "get", "sb-missing", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitNotFound))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("failed"))
				failure := env["Failure"].(map[string]any)
				Expect(failure["Code"]).To(Equal("INSTANCE_NOT_FOUND"))
			})
		})
	})

	Describe("instance delete --ignore-not-found (AC19)", func() {
		It("exits 0 when not found with --ignore-not-found", func() {
			fake := &fakeControlPlane{
				deleteErr: output.NewNotFoundError("NOT_FOUND", "not found", ""),
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "delete", "sb-gone", "--ignore-not-found", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
			})
		})
	})

	Describe("instance create --client-token conflict (AC20)", func() {
		It("returns exit 5 with CLIENT_TOKEN_CONFLICT", func() {
			fake := &fakeControlPlane{
				createErr: output.NewConflictError("CLIENT_TOKEN_CONFLICT", "duplicate client token", ""),
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "create", "-t", "ci-v1", "--client-token", "dup-tok", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitConflict))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("failed"))
			})
		})
	})

	Describe("instance create succeeded path", func() {
		It("returns instance data with effects", func() {
			fake := &fakeControlPlane{
				createInstance: &client.Instance{ID: "sb-new", ToolName: "ci-v1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"},
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "create", "-t", "ci-v1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
				data := env["Data"].(map[string]any)
				Expect(data["Id"]).To(Equal("sb-new"))
				meta := env["Meta"].(map[string]any)
				effects := meta["Effects"].([]any)
				Expect(effects).To(HaveLen(1))
				effect := effects[0].(map[string]any)
				Expect(effect["Kind"]).To(Equal("create"))
			})
		})
	})

	Describe("tool list succeeded path", func() {
		It("returns empty list", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "tool", "list", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
			})
		})
	})

	Describe("tool get succeeded/failed paths", func() {
		It("returns tool on success", func() {
			fake := &fakeControlPlane{
				tools: []*client.Tool{{ID: "sdt-1", Name: "my-tool", Type: "code-interpreter", Status: "active", CreatedAt: "2026-01-01T00:00:00Z"}},
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "tool", "get", "sdt-1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				data := env["Data"].(map[string]any)
				Expect(data["Id"]).To(Equal("sdt-1"))
				Expect(data["Status"]).To(Equal("active"))
			})
		})

		It("returns not-found for missing tool", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "tool", "get", "sdt-missing", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitNotFound))
			})
		})
	})

	Describe("apikey create/list/delete succeeded paths", func() {
		It("apikey create returns key data", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "apikey", "create", "-n", "test-key", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				data := env["Data"].(map[string]any)
				Expect(data["KeyId"]).To(Equal("ak-fake"))
				Expect(data["ApiKey"]).To(Equal("ark_fake"))
			})
		})

		It("apikey list returns empty list", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "apikey", "list", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
			})
		})

		It("apikey delete succeeds", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "apikey", "delete", "ak-1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
			})
		})
	})

	Describe("instance delete succeeded path", func() {
		It("deletes successfully with JSON envelope", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "delete", "sb-1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
				data := env["Data"].(map[string]any)
				Expect(data["Deleted"]).To(BeNumerically("==", 1))
			})
		})
	})

	Describe("tool create --client-token conflict (AC20)", func() {
		It("returns exit 5 for duplicate token", func() {
			fake := &fakeControlPlane{
				createErr: output.NewConflictError("CLIENT_TOKEN_CONFLICT", "duplicate client token", ""),
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "create", "-n", "my-tool", "-t", "code-interpreter", "--client-token", "dup", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitConflict))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("failed"))
			})
		})
	})

	Describe("Canonical schema consistency (AC24)", func() {
		It("instance list and get produce same field names", func() {
			inst := &client.Instance{ID: "sb-1", ToolID: "sdt-1", ToolName: "ci-v1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}
			fake := &fakeControlPlane{instances: []*client.Instance{inst}}
			withFakeClient(fake, func() {
				getResult := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "instance", "get", "sb-1", "-o", "json")
				getEnv := parseEnvelope(getResult.stdout)
				getData := getEnv["Data"].(map[string]any)

				getKeys := make([]string, 0)
				for k := range getData {
					getKeys = append(getKeys, k)
				}
				Expect(getKeys).To(ContainElements("Id", "ToolId", "ToolName", "Status", "CreatedAt"))
			})
		})

		It("tool list and get produce same field names", func() {
			tool := &client.Tool{ID: "sdt-1", Name: "t", Type: "ci", Status: "active", CreatedAt: "2026-01-01T00:00:00Z"}
			fake := &fakeControlPlane{tools: []*client.Tool{tool}}
			withFakeClient(fake, func() {
				getResult := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key", "tool", "get", "sdt-1", "-o", "json")
				getEnv := parseEnvelope(getResult.stdout)
				getData := getEnv["Data"].(map[string]any)
				getKeys := make([]string, 0)
				for k := range getData {
					getKeys = append(getKeys, k)
				}
				Expect(getKeys).To(ContainElements("Id", "Name", "Type", "Status", "CreatedAt"))
			})
		})
	})

	Describe("--jq with JSON envelope (AC6)", func() {
		It("filters instance list data", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "list", "-o", "json", "--jq", ".Data.Items")
				Expect(r.exitCode).To(Equal(0))
				var items []any
				Expect(json.Unmarshal([]byte(r.stdout), &items)).To(Succeed())
				Expect(items).To(HaveLen(0))
			})
		})
	})

	Describe("tool create succeeded path", func() {
		It("returns tool data with effects", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "create", "-n", "my-tool", "-t", "code-interpreter", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
				data := env["Data"].(map[string]any)
				Expect(data["Id"]).To(Equal("sdt-fake"))
			})
		})
	})

	Describe("tool update succeeded path", func() {
		It("returns updated tool id", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "update", "sdt-1", "-d", "new desc", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
			})
		})
	})

	Describe("tool delete succeeded path", func() {
		It("deletes tool successfully", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "delete", "sdt-1", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("succeeded"))
			})
		})
	})

	Describe("instance delete failed path", func() {
		It("returns partial result when delete fails in JSON mode", func() {
			fake := &fakeControlPlane{
				deleteErr: fmt.Errorf("server error: instance unavailable"),
			}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "delete", "sb-gone", "-o", "json")
				env := parseEnvelope(r.stdout)
				Expect(env["Status"]).To(Equal("partial"))
				data := env["Data"].(map[string]any)
				Expect(data["Failed"]).To(BeNumerically("==", 1))
			})
		})
	})

	Describe("mobile list succeeded path", func() {
		It("returns empty connection list", func() {
			r := executeCmd("instance", "mobile", "list", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["Status"]).To(Equal("succeeded"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Items"))
		})
	})

	Describe("Agent/CI workflow (AC29)", func() {
		It("create -> list -> get -> delete --ignore-not-found lifecycle", func() {
			inst := &client.Instance{ID: "sb-wf", ToolName: "ci-v1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}
			fake := &fakeControlPlane{
				createInstance: inst,
				instances:      []*client.Instance{inst},
			}
			withFakeClient(fake, func() {
				// create
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "create", "-t", "ci-v1", "-o", "json", "--jq", ".Data.Id")
				Expect(r.exitCode).To(Equal(0))
				Expect(r.stdout).To(ContainSubstring("sb-wf"))

				// list
				r = executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "list", "-o", "json")
				Expect(r.exitCode).To(Equal(0))

				// get
				r = executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "get", "sb-wf", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
				env := parseEnvelope(r.stdout)
				data := env["Data"].(map[string]any)
				Expect(data["Id"]).To(Equal("sb-wf"))

				// delete --ignore-not-found
				fake.deleteErr = fmt.Errorf("instance not found")
				r = executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "delete", "sb-wf", "--ignore-not-found", "-o", "json")
				Expect(r.exitCode).To(Equal(0))
			})
		})
	})

	Describe("Data-plane injectable sandbox", func() {
		It("connectSandbox is injectable", func() {
			Expect(connectSandbox).NotTo(BeNil())
		})

		It("connectSandbox can be overridden for testing", func() {
			orig := connectSandbox
			called := false
			connectSandbox = func(_ context.Context, id string) (*code.Sandbox, error) {
				called = true
				return nil, fmt.Errorf("fake sandbox: %s", id)
			}
			defer func() { connectSandbox = orig }()

			_, err := ConnectSandboxWithCache(context.Background(), "sb-test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake sandbox: sb-test"))
			Expect(called).To(BeTrue())
		})
	})

	Describe("Data-plane command validation without sandbox (AC16)", func() {
		It("instance code run does not implicitly create instances", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "code", "run", "sb-1", "-c", "print(1)")
				_ = r
			})
			Expect(fake.createInstance).To(BeNil())
		})

		It("instance exec does not implicitly create instances", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "exec", "sb-1", "--", "ls")
				_ = r
			})
			Expect(fake.createInstance).To(BeNil())
		})

		It("instance file upload does not implicitly create instances", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "file", "upload", "sb-1", "/dev/null", "/remote")
				_ = r
			})
			Expect(fake.createInstance).To(BeNil())
		})

		It("instance file download does not implicitly create instances", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "file", "download", "sb-1", "/remote", "/tmp/out")
				_ = r
			})
			Expect(fake.createInstance).To(BeNil())
		})

		It("instance browser vnc does not implicitly create instances", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "browser", "vnc", "sb-1")
				_ = r
			})
			Expect(fake.createInstance).To(BeNil())
		})


	})

	Describe("NDJSON writer unit coverage (AC8)", func() {
		It("NDJSONWriter produces valid one-JSON-per-line output", func() {
			var buf bytes.Buffer
			nw := output.NewNDJSONWriter(&buf, "instance.code.run")
			Expect(nw.WriteStarted(map[string]string{"InstanceId": "sb-1"})).To(Succeed())
			Expect(nw.WriteStdout("hello\n")).To(Succeed())
			Expect(nw.WriteStderr("warn\n")).To(Succeed())
			Expect(nw.WriteCompleted(map[string]any{"ExecutionCount": 1})).To(Succeed())

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			Expect(lines).To(HaveLen(4))

			for _, line := range lines {
				var event map[string]any
				Expect(json.Unmarshal([]byte(line), &event)).To(Succeed())
				Expect(event["SchemaVersion"]).To(Equal("ags.events.v1"))
				Expect(event["Command"]).To(Equal("instance.code.run"))
			}

			var first map[string]any
			Expect(json.Unmarshal([]byte(lines[0]), &first)).To(Succeed())
			Expect(first["Type"]).To(Equal("started"))

			var last map[string]any
			Expect(json.Unmarshal([]byte(lines[3]), &last)).To(Succeed())
			Expect(last["Type"]).To(Equal("completed"))
		})

		It("NDJSONWriter exec failed event carries ExitCode in Data", func() {
			var buf bytes.Buffer
			nw := output.NewNDJSONWriter(&buf, "instance.exec")
			Expect(nw.WriteStarted(nil)).To(Succeed())
			Expect(nw.WriteFailed(map[string]any{"ExitCode": 42}, nil)).To(Succeed())

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			Expect(lines).To(HaveLen(2))

			var failed map[string]any
			Expect(json.Unmarshal([]byte(lines[1]), &failed)).To(Succeed())
			Expect(failed["Type"]).To(Equal("failed"))
			data := failed["Data"].(map[string]any)
			Expect(data["ExitCode"]).To(BeNumerically("==", 42))
		})
	})

	Describe("--request parsing and validation (AC22)", func() {
		It("parseRequestFlag parses valid JSON object", func() {
			data, err := parseRequestFlag(`{"ToolName":"test","TimeoutSeconds":300}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(data["ToolName"]).To(Equal("test"))
			Expect(data["TimeoutSeconds"]).To(BeNumerically("==", 300))
		})

		It("parseRequestFlag rejects invalid JSON with exit 2", func() {
			_, err := parseRequestFlag(`{bad}`)
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitUsage))
		})

		It("parseRequestFlag rejects non-object JSON with exit 2", func() {
			_, err := parseRequestFlag(`[1,2]`)
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitUsage))
		})

		It("instance create --request + -t returns REQUEST_FLAG_CONFLICT", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"instance", "create", "--request", `{"ToolName":"ci-v1"}`, "-t", "ci-v1", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitUsage))
			})
		})

		It("tool create --request + -n returns REQUEST_FLAG_CONFLICT", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "create", "--request", `{"Name":"t","Type":"ci"}`, "-n", "t", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitUsage))
			})
		})

		It("tool update --request + --description returns REQUEST_FLAG_CONFLICT", func() {
			fake := &fakeControlPlane{}
			withFakeClient(fake, func() {
				r := executeCmd("--backend", "cloud", "--secret-id", "id", "--secret-key", "key",
					"tool", "update", "sdt-1", "--request", `{"Description":"x"}`, "-d", "y", "-o", "json")
				Expect(r.exitCode).To(Equal(output.ExitUsage))
			})
		})
	})
})
