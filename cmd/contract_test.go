package cmd

import (
	"encoding/json"
	"errors"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

type execResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func executeCmd(args ...string) execResult {
	testIOS, _, stdout, stderr := iostreams.Test()
	SetIOStreams(testIOS)
	defer func() { ios = nil }()

	saved := *rootCmd
	defer func() { *rootCmd = saved }()

	outputFmt = ""
	jqExpr = ""
	cfgFile = ""
	nonInteractive = false
	noColor = false
	showVersion = false

	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	backend = ""
	apiKey = ""
	secretID = ""
	secretKey = ""
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--backend":
			backend = args[i+1]
		case "--secret-id":
			secretID = args[i+1]
		case "--secret-key":
			secretKey = args[i+1]
		case "--api-key":
			apiKey = args[i+1]
		case "-o", "--output":
			outputFmt = args[i+1]
		case "--jq":
			jqExpr = args[i+1]
		}
	}
	initConfig()
	configInitErr = nil

	var exitCode int
	err := rootCmd.Execute()
	if err != nil {
		var envDone *envelopeAlreadyWritten
		if errors.As(err, &envDone) {
			exitCode = envDone.code
		} else {
			cliErr := output.ClassifyError(err)
			if cliErr.ExitCode == output.ExitGenericError && isCobraUsageError(err) {
				cliErr = output.NewUsageError("INVALID_USAGE", err.Error(), "")
			}
			exitCode = cliErr.ExitCode
		}
	}

	return execResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
		err:      err,
	}
}

func parseEnvelope(jsonStr string) map[string]any {
	var env map[string]any
	ExpectWithOffset(1, json.Unmarshal([]byte(jsonStr), &env)).To(Succeed())
	return env
}

var _ = Describe("Output Contract", func() {

	Describe("JSON Envelope (AC1, AC5, AC11)", func() {

		It("ags prints help by default", func() {
			r := executeCmd()
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("AGS CLI"))
		})

		It("ags version outputs version info", func() {
			r := executeCmd("version")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("ags version"))
		})

		It("ags version -o json outputs envelope with SchemaVersion", func() {
			r := executeCmd("version", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["SchemaVersion"]).To(Equal("ags.v1"))
			Expect(env["Command"]).To(Equal("version"))
			Expect(env["Status"]).To(Equal("succeeded"))
			Expect(env["Failure"]).To(BeNil())
			Expect(env["Warnings"]).To(BeAssignableToTypeOf([]any{}))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Version"))
			Expect(data).To(HaveKey("Commit"))
			Expect(data).To(HaveKey("BuildTime"))
		})

		It("ags status -o json outputs envelope", func() {
			r := executeCmd("status", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["SchemaVersion"]).To(Equal("ags.v1"))
			Expect(env["Command"]).To(Equal("status"))
			Expect(env["Status"]).To(Equal("succeeded"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Backend"))
			Expect(data).To(HaveKey("Auth"))
		})

		It("ags doctor -o json outputs envelope with Checks", func() {
			r := executeCmd("doctor", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["Command"]).To(Equal("doctor"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Checks"))
		})

		It("ags capabilities -o json outputs envelope with Commands", func() {
			r := executeCmd("capabilities", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["Command"]).To(Equal("capabilities"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Backend"))
			Expect(data).To(HaveKey("Commands"))
		})

		It("envelope Failure is null on success (AC11)", func() {
			r := executeCmd("version", "-o", "json")
			env := parseEnvelope(r.stdout)
			Expect(env["Failure"]).To(BeNil())
		})

		It("envelope Meta contains Backend and DurationMs", func() {
			r := executeCmd("version", "-o", "json")
			env := parseEnvelope(r.stdout)
			meta := env["Meta"].(map[string]any)
			Expect(meta).To(HaveKey("Backend"))
			Expect(meta).To(HaveKey("DurationMs"))
		})
	})

	Describe("--jq flag (AC6, AC7)", func() {

		It("--jq filters envelope data", func() {
			r := executeCmd("version", "-o", "json", "--jq", ".Data.Version")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).NotTo(BeEmpty())
			Expect(r.stdout).NotTo(ContainSubstring("SchemaVersion"))
		})

		It("--jq without -o json returns exit 2 (AC7)", func() {
			r := executeCmd("version", "--jq", ".Data.Version")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("--jq with -o text returns exit 2", func() {
			r := executeCmd("version", "-o", "text", "--jq", ".Data")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("Schema command (AC23)", func() {

		It("ags schema -o json outputs all command schemas", func() {
			r := executeCmd("schema", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Commands"))
		})

		It("ags schema instance.create -o json outputs single schema", func() {
			r := executeCmd("schema", "instance.create", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data["Name"]).To(Equal("instance.create"))
			Expect(data).To(HaveKey("Mutation"))
			Expect(data).To(HaveKey("SupportsJson"))
			Expect(data).To(HaveKey("SupportsJq"))
			Expect(data).To(HaveKey("SupportsRequest"))
			Expect(data).To(HaveKey("RequestSchema"))
			Expect(data).To(HaveKey("CreatesResource"))
			Expect(data).To(HaveKey("Idempotency"))
			Expect(data).To(HaveKey("SupportsDryRun"))
		})

		It("schema marks SupportsDryRun=false (AC21)", func() {
			r := executeCmd("schema", "instance.create", "-o", "json")
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data["SupportsDryRun"]).To(BeFalse())
		})

		It("tool.create schema has RequestSchema (AC23)", func() {
			r := executeCmd("schema", "tool.create", "-o", "json")
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data["SupportsRequest"]).To(BeTrue())
			Expect(data["RequestSchema"]).NotTo(BeNil())
		})

		It("tool.update schema has RequestSchema", func() {
			r := executeCmd("schema", "tool.update", "-o", "json")
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data["SupportsRequest"]).To(BeTrue())
			Expect(data["RequestSchema"]).NotTo(BeNil())
		})
	})

	Describe("Stream and NDJSON validation (AC9, AC10)", func() {

		It("-o ndjson on non-stream command returns exit 2 (AC9)", func() {
			r := executeCmd("instance", "list", "-o", "ndjson")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("-o ndjson without --stream returns exit 2 (AC9)", func() {
			r := executeCmd("instance", "code", "run", "test-id", "-c", "x", "-o", "ndjson")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("No JSON support commands (AC27)", func() {

		It("completion -o json returns usage error", func() {
			r := executeCmd("completion", "bash", "-o", "json")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("docs -o json returns usage error", func() {
			r := executeCmd("docs", "-o", "json")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance proxy -o json returns usage error", func() {
			r := executeCmd("instance", "proxy", "sb-1", "8080", "-o", "json")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("Removed top-level commands", func() {
		for _, removed := range []string{"run", "exec", "file", "login", "browser", "proxy", "mobile", "process"} {
			removed := removed
			It("ags "+removed+" is not a valid command", func() {
				r := executeCmd(removed)
				Expect(r.exitCode).NotTo(Equal(0))
			})
		}
	})

	Describe("Stdout/stderr separation", func() {
		It("version -o json writes only JSON to stdout", func() {
			r := executeCmd("version", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(HavePrefix("{"))
			Expect(r.stderr).To(BeEmpty())
		})

		It("status text writes result to stdout and no JSON", func() {
			r := executeCmd("status")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("Backend"))
			Expect(r.stdout).NotTo(ContainSubstring("SchemaVersion"))
		})
	})

	Describe("Credential-free commands (AC28)", func() {
		for _, cmdName := range []string{"status", "capabilities", "schema", "doctor", "version"} {
			cmdName := cmdName
			It(cmdName+" does not require credentials", func() {
				r := executeCmd(cmdName)
				Expect(r.exitCode).To(Equal(0))
			})
		}
	})

	Describe("Missing instance ID (AC17)", func() {

		It("instance get with no args returns usage error", func() {
			r := executeCmd("instance", "get")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance delete with no args returns usage error", func() {
			r := executeCmd("instance", "delete")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance code run with no args returns usage error", func() {
			r := executeCmd("instance", "code", "run")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance exec with no args returns usage error", func() {
			r := executeCmd("instance", "exec")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance file upload with no args returns usage error", func() {
			r := executeCmd("instance", "file", "upload")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance file download with no args returns usage error", func() {
			r := executeCmd("instance", "file", "download")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("Separator enforcement (AC18)", func() {

		It("exec without -- returns non-zero exit", func() {
			r := executeCmd("instance", "exec", "test-id", "ls")
			Expect(r.exitCode).NotTo(Equal(0))
		})

		It("mobile adb without -- returns usage error", func() {
			r := executeCmd("instance", "mobile", "adb", "test-id", "shell", "ls")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("mobile adb with only instance-id returns usage error", func() {
			r := executeCmd("instance", "mobile", "adb", "test-id")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("Backend unsupported (AC25)", func() {

		for _, sub := range [][]string{
			{"--backend", "e2b", "--api-key", "test-key", "tool", "list"},
			{"--backend", "e2b", "--api-key", "test-key", "tool", "get", "sdt-test"},
			{"--backend", "e2b", "--api-key", "test-key", "tool", "create", "-n", "t", "-t", "code-interpreter"},
			{"--backend", "e2b", "--api-key", "test-key", "tool", "delete", "sdt-test"},
			{"--backend", "e2b", "--api-key", "test-key", "apikey", "list"},
			{"--backend", "e2b", "--api-key", "test-key", "apikey", "create", "-n", "test"},
			{"--backend", "e2b", "--api-key", "test-key", "apikey", "delete", "ak-test"},
		} {
			sub := sub
			desc := sub[4] + " " + sub[5]
			It(desc+" on e2b returns exit 9", func() {
				r := executeCmd(sub...)
				Expect(r.exitCode).To(Equal(output.ExitBackendUnsupported))
			})
		}
	})

	Describe("--request parsing (AC22)", func() {

		It("parseRequestFlag parses inline JSON", func() {
			data, err := parseRequestFlag(`{"ToolName":"test"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(data["ToolName"]).To(Equal("test"))
		})

		It("parseRequestFlag rejects invalid JSON with exit 2", func() {
			_, err := parseRequestFlag(`{invalid}`)
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitUsage))
		})

		It("parseRequestFlag rejects non-object JSON with exit 2", func() {
			_, err := parseRequestFlag(`[1,2,3]`)
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitUsage))
		})

		It("parseRequestFlag rejects scalar JSON with exit 2", func() {
			_, err := parseRequestFlag(`"hello"`)
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitUsage))
		})
	})

	Describe("instance delete --ignore-not-found (AC19)", func() {

		It("--ignore-not-found flag is registered", func() {
			var deleteCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					for _, sc := range c.Commands() {
						if sc.Name() == "delete" {
							deleteCmd = sc
						}
					}
				}
			}
			Expect(deleteCmd).NotTo(BeNil())
			f := deleteCmd.Flags().Lookup("ignore-not-found")
			Expect(f).NotTo(BeNil())
			Expect(f.DefValue).To(Equal("false"))
		})
	})

	Describe("Credential validation (exit 4)", func() {

		It("cloud backend without credentials returns auth error", func() {
			config.SetBackend("cloud")
			config.SetSecretID("")
			config.SetSecretKey("")
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitAuthOrPermission))
		})

		It("e2b backend without API key returns auth error", func() {
			config.SetBackend("e2b")
			config.SetAPIKey("")
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			var cliErr *output.CLIError
			Expect(errors.As(err, &cliErr)).To(BeTrue())
			Expect(cliErr.ExitCode).To(Equal(output.ExitAuthOrPermission))
			config.SetBackend("cloud")
		})
	})

	Describe("Command tree structure", func() {

		It("instance has create/list/get/delete/login/code/exec/file/browser/proxy/mobile", func() {
			var instanceCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					instanceCmd = c
				}
			}
			Expect(instanceCmd).NotTo(BeNil())
			names := make([]string, 0)
			for _, c := range instanceCmd.Commands() {
				names = append(names, c.Name())
			}
			Expect(names).To(ContainElements("create", "list", "get", "delete", "login",
				"code", "exec", "file", "browser", "proxy", "mobile"))
		})

		It("tool has create/list/get/update/delete", func() {
			var toolCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "tool" {
					toolCmd = c
				}
			}
			Expect(toolCmd).NotTo(BeNil())
			names := make([]string, 0)
			for _, c := range toolCmd.Commands() {
				names = append(names, c.Name())
			}
			Expect(names).To(ContainElements("create", "list", "get", "update", "delete"))
		})

		It("apikey has create/list/delete", func() {
			var akCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "apikey" {
					akCmd = c
				}
			}
			Expect(akCmd).NotTo(BeNil())
			names := make([]string, 0)
			for _, c := range akCmd.Commands() {
				names = append(names, c.Name())
			}
			Expect(names).To(ContainElements("create", "list", "delete"))
		})

		It("mobile has connect/disconnect/list/adb and hidden tunnel", func() {
			var mobileCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					for _, sc := range c.Commands() {
						if sc.Name() == "mobile" {
							mobileCmd = sc
						}
					}
				}
			}
			Expect(mobileCmd).NotTo(BeNil())
			names := make([]string, 0)
			var tunnelHidden bool
			for _, c := range mobileCmd.Commands() {
				names = append(names, c.Name())
				if c.Name() == "tunnel" {
					tunnelHidden = c.Hidden
				}
			}
			Expect(names).To(ContainElements("connect", "disconnect", "list", "adb", "tunnel"))
			Expect(tunnelHidden).To(BeTrue())
		})

		It("instance login does not support -o json", func() {
			var loginCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					for _, sc := range c.Commands() {
						if sc.Name() == "login" {
							loginCmd = sc
						}
					}
				}
			}
			Expect(loginCmd).NotTo(BeNil())
		})

		It("instance create has --client-token and --request flags", func() {
			var createCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					for _, sc := range c.Commands() {
						if sc.Name() == "create" {
							createCmd = sc
						}
					}
				}
			}
			Expect(createCmd).NotTo(BeNil())
			Expect(createCmd.Flags().Lookup("client-token")).NotTo(BeNil())
			Expect(createCmd.Flags().Lookup("request")).NotTo(BeNil())
		})

		It("tool create has --client-token and --request flags", func() {
			var createCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "tool" {
					for _, sc := range c.Commands() {
						if sc.Name() == "create" {
							createCmd = sc
						}
					}
				}
			}
			Expect(createCmd).NotTo(BeNil())
			Expect(createCmd.Flags().Lookup("client-token")).NotTo(BeNil())
			Expect(createCmd.Flags().Lookup("request")).NotTo(BeNil())
		})

		It("tool update has --request flag", func() {
			var updateCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "tool" {
					for _, sc := range c.Commands() {
						if sc.Name() == "update" {
							updateCmd = sc
						}
					}
				}
			}
			Expect(updateCmd).NotTo(BeNil())
			Expect(updateCmd.Flags().Lookup("request")).NotTo(BeNil())
		})
	})

	Describe("instance file (AC26)", func() {

		It("instance file has upload and download subcommands", func() {
			var fileCmd *cobra.Command
			for _, c := range rootCmd.Commands() {
				if c.Name() == "instance" {
					for _, sc := range c.Commands() {
						if sc.Name() == "file" {
							fileCmd = sc
						}
					}
				}
			}
			Expect(fileCmd).NotTo(BeNil())
			names := make([]string, 0)
			for _, c := range fileCmd.Commands() {
				names = append(names, c.Name())
			}
			Expect(names).To(ContainElements("upload", "download"))
			Expect(names).NotTo(ContainElement("list"))
			Expect(names).NotTo(ContainElement("remove"))
			Expect(names).NotTo(ContainElement("mkdir"))
		})
	})

	Describe("Help -o json equivalence (AC3, AC4)", func() {

		It("ags schema -o json outputs all schemas", func() {
			r := executeCmd("schema", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["SchemaVersion"]).To(Equal("ags.v1"))
			Expect(env["Command"]).To(Equal("schema"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Commands"))
		})

		It("ags schema instance.code.run -o json outputs single schema", func() {
			r := executeCmd("schema", "instance.code.run", "-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			data := env["Data"].(map[string]any)
			Expect(data["Name"]).To(Equal("instance.code.run"))
			Expect(data["Summary"]).NotTo(BeEmpty())
			Expect(data).To(HaveKey("BackendSupport"))
			Expect(data).To(HaveKey("Flags"))
			Expect(data).To(HaveKey("Args"))
		})

		It("ags -o json produces schema envelope (AC2)", func() {
			r := executeCmd("-o", "json")
			Expect(r.exitCode).To(Equal(0))
			env := parseEnvelope(r.stdout)
			Expect(env["SchemaVersion"]).To(Equal("ags.v1"))
			Expect(env["Command"]).To(Equal("schema"))
			data := env["Data"].(map[string]any)
			Expect(data).To(HaveKey("Commands"))
		})

		It("ags --help -o json produces same data as ags schema -o json (AC3)", func() {
			schemaResult := executeCmd("schema", "-o", "json")
			Expect(schemaResult.exitCode).To(Equal(0))
			schemaEnv := parseEnvelope(schemaResult.stdout)
			schemaData := schemaEnv["Data"]

			helpResult := executeCmd("--help", "-o", "json")
			Expect(helpResult.exitCode).To(Equal(0))
			helpEnv := parseEnvelope(helpResult.stdout)
			helpData := helpEnv["Data"]

			schemaJSON, _ := json.Marshal(schemaData)
			helpJSON, _ := json.Marshal(helpData)
			Expect(string(helpJSON)).To(Equal(string(schemaJSON)))
		})

		It("ags instance code run --help -o json produces same data as schema instance.code.run (AC4)", func() {
			schemaResult := executeCmd("schema", "instance.code.run", "-o", "json")
			Expect(schemaResult.exitCode).To(Equal(0))
			schemaEnv := parseEnvelope(schemaResult.stdout)
			schemaData := schemaEnv["Data"]

			helpResult := executeCmd("instance", "code", "run", "--help", "-o", "json")
			Expect(helpResult.exitCode).To(Equal(0))
			helpEnv := parseEnvelope(helpResult.stdout)
			helpData := helpEnv["Data"]

			schemaJSON, _ := json.Marshal(schemaData)
			helpJSON, _ := json.Marshal(helpData)
			Expect(string(helpJSON)).To(Equal(string(schemaJSON)))
		})
	})

	Describe("Client-token conflict classification (AC20)", func() {

		It("DuplicatedClientToken cloud error maps to exit 5 CONFLICT", func() {
			err := output.NewConflictError("CLIENT_TOKEN_CONFLICT",
				"duplicate client token",
				"The client token has already been used.")
			Expect(err.ExitCode).To(Equal(output.ExitConflict))
			Expect(err.Failure.Code).To(Equal("CLIENT_TOKEN_CONFLICT"))
			Expect(err.Failure.Kind).To(Equal(output.KindConflict))
		})
	})

	Describe("Per-command usage error paths", func() {

		It("instance browser vnc with no args returns usage error", func() {
			r := executeCmd("instance", "browser", "vnc")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance proxy with no args returns usage error", func() {
			r := executeCmd("instance", "proxy")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance proxy with one arg returns usage error", func() {
			r := executeCmd("instance", "proxy", "sb-1")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance mobile connect with no args returns usage error", func() {
			r := executeCmd("instance", "mobile", "connect")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("instance mobile disconnect with no args and no --all returns error", func() {
			r := executeCmd("instance", "mobile", "disconnect")
			Expect(r.exitCode).NotTo(Equal(0))
		})

		It("instance login with no args returns usage error", func() {
			r := executeCmd("instance", "login")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("tool get with no args returns usage error", func() {
			r := executeCmd("tool", "get")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("tool update with no args returns usage error", func() {
			r := executeCmd("tool", "update")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("tool delete with no args returns usage error", func() {
			r := executeCmd("tool", "delete")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("apikey delete with no args returns usage error", func() {
			r := executeCmd("apikey", "delete")
			Expect(r.exitCode).To(Equal(output.ExitUsage))
		})

		It("apikey create without --name returns error", func() {
			r := executeCmd("--secret-id", "id", "--secret-key", "key", "apikey", "create")
			Expect(r.exitCode).NotTo(Equal(0))
		})
	})

	Describe("Completion command paths", func() {

		It("completion bash succeeds", func() {
			r := executeCmd("completion", "bash")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).NotTo(BeEmpty())
		})

		It("completion zsh succeeds", func() {
			r := executeCmd("completion", "zsh")
			Expect(r.exitCode).To(Equal(0))
		})

		It("completion fish succeeds", func() {
			r := executeCmd("completion", "fish")
			Expect(r.exitCode).To(Equal(0))
		})

		It("completion powershell succeeds", func() {
			r := executeCmd("completion", "powershell")
			Expect(r.exitCode).To(Equal(0))
		})

		It("completion with invalid shell returns error", func() {
			r := executeCmd("completion", "csh")
			Expect(r.exitCode).NotTo(Equal(0))
		})
	})

	Describe("instance create usage paths", func() {

		It("instance create without tool-name or tool-id returns error", func() {
			r := executeCmd("--secret-id", "id", "--secret-key", "key", "instance", "create")
			Expect(r.exitCode).NotTo(Equal(0))
		})

		It("instance create with both --tool-name and --tool-id returns error", func() {
			r := executeCmd("--secret-id", "id", "--secret-key", "key", "instance", "create", "-t", "ci-v1", "--tool-id", "sdt-x")
			Expect(r.exitCode).NotTo(Equal(0))
		})
	})

	Describe("code run validation (AC10)", func() {

		It("validateRunFlags rejects unsupported language", func() {
			origLang := runLanguage
			defer func() { runLanguage = origLang }()
			runLanguage = "ruby"
			Expect(validateRunFlags()).To(HaveOccurred())
		})

		It("validateStreamNotJSON returns usage error when JSON", func() {
			config.SetOutput("json")
			err := validateStreamNotJSON()
			Expect(err).To(HaveOccurred())
			config.SetOutput("text")
		})

		It("validateNDJSONOnlyForStream errors without stream", func() {
			config.SetOutput("ndjson")
			err := validateNDJSONOnlyForStream(false)
			Expect(err).To(HaveOccurred())
			config.SetOutput("text")
		})
	})

	Describe("version -o json success path", func() {

		It("version -o json --jq extracts Version", func() {
			r := executeCmd("version", "-o", "json", "--jq", ".Data.Version")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("dev"))
		})
	})

	Describe("status text success path", func() {

		It("status text output includes Backend", func() {
			r := executeCmd("status")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("Backend"))
		})
	})

	Describe("doctor text success path", func() {

		It("doctor text output includes check results", func() {
			r := executeCmd("doctor")
			Expect(r.exitCode).To(Equal(0))
			Expect(r.stdout).To(ContainSubstring("backend"))
		})
	})
})
