# AGS CLI

AGS CLI is a command-line tool for managing Tencent Cloud Agent Sandbox (AGS). It provides unified management of sandbox instances, code execution, file operations, and tool templates across both Cloud and E2B backends.

## Installation

### From source

```bash
git clone https://github.com/TencentCloudAgentRuntime/ags-cli.git
cd ags-cli
make build
sudo cp ags /usr/local/bin/
```

### Using go install

```bash
go install github.com/TencentCloudAgentRuntime/ags-cli/cmd/ags@latest
```

## Configure Cloud Credentials

The default backend is `cloud`. Set your Tencent Cloud credentials:

```bash
export TENCENTCLOUD_SECRET_ID="your-secret-id"
export TENCENTCLOUD_SECRET_KEY="your-secret-key"
```

Or create `~/.ags/config.toml`:

```toml
backend = "cloud"

[auth]
secret_id = "your-secret-id"
secret_key = "your-secret-key"
```

## Quick Start

```bash
# Create an instance, run code, then clean up
id=$(ags instance create -t code-interpreter-v1 -o json --jq '.Data.Id')
ags instance code run "$id" -c "print('Hello, World!')"
ags instance delete "$id"
```

## E2B Backend

To use the E2B backend instead of Cloud:

```bash
export AGS_API_KEY="your-api-key"

id=$(ags --backend e2b instance create -t code-interpreter-v1 -o json --jq '.Data.Id')
ags --backend e2b instance code run "$id" -c "print('Hello, World!')"
ags --backend e2b instance delete "$id"
```

Or in config:

```toml
backend = "e2b"

[auth]
api_key = "your-api-key"
```

## Command Overview

```
ags                              Print help
ags version                      Version info
ags status                       Current configuration status
ags capabilities                 Available commands for current backend
ags schema [command]             Command schema (machine-readable)
ags doctor                       Diagnose configuration issues

ags instance create              Create a new instance
ags instance list                List instances
ags instance get <id>            Get instance details
ags instance delete <id>         Delete instance(s)

ags instance code run <id>       Execute code in instance
ags instance exec <id> -- CMD    Execute shell command in instance
ags instance file upload <id>    Upload file to instance
ags instance file download <id>  Download file from instance
ags instance login <id>          PTY terminal session

ags instance browser vnc <id>    Show VNC URL for browser instance
ags instance proxy <id> PORT     Forward instance port to localhost
ags instance mobile connect <id> Connect ADB to mobile instance

ags tool list                    List sandbox tools (cloud only)
ags tool create                  Create a tool
ags tool get <id>                Get tool details
ags tool update <id>             Update a tool
ags tool delete <id>             Delete a tool

ags apikey create                Create API key (cloud only)
ags apikey list                  List API keys
ags apikey delete <id>           Delete API key

ags completion bash|zsh|fish     Shell completion
ags docs markdown|man            Generate documentation
```

## Machine-Readable Output

All commands that support `-o json` output a unified envelope:

```json
{
  "SchemaVersion": "ags.v1",
  "Command": "instance.create",
  "Status": "succeeded",
  "Data": { "Id": "sandbox-xxx", "ToolName": "code-interpreter-v1", ... },
  "Failure": null,
  "Warnings": [],
  "Meta": { "Backend": "cloud", "DurationMs": 123 }
}
```

Use `--jq` to extract fields without installing jq:

```bash
# Get instance ID after creation
id=$(ags instance create -t code-interpreter-v1 -o json --jq '.Data.Id')

# List all instance IDs
ags instance list -o json --jq '.Data.Items[].Id'

# Check current backend
ags status -o json --jq '.Data.Backend'
```

### Streaming (NDJSON)

`instance code run` and `instance exec` support `--stream -o ndjson` for machine-readable streaming:

```bash
ags instance exec "$id" --stream -o ndjson -- tail -f /var/log/app.log
```

Each line is a JSON event with `SchemaVersion: "ags.events.v1"`.

## Exit Codes

| Exit | Kind | Description |
|------|------|-------------|
| 0 | success | OK |
| 1 | generic_error | Unclassified error |
| 2 | usage | Invalid args, flags, or input |
| 3 | not_found | Instance, tool, or file not found |
| 4 | auth_or_permission | Missing credentials or access denied |
| 5 | conflict | Resource conflict (e.g. duplicate client-token) |
| 6 | rate_limit | Rate limited |
| 7 | timeout | Timeout |
| 8 | network | Network, DNS, or TLS error |
| 9 | backend_unsupported | Command not supported on current backend |
| 10 | partial_success | Batch operation partially failed |
| 11 | remote_execution_failed | Remote code execution error |

`instance exec` transparently passes through the remote command's exit code.

## Global Flags

```
--config          Config file path (default: ~/.ags/config.toml)
--backend         API backend: cloud or e2b
-o, --output      Output format: text, json, or ndjson
--jq              jq expression (only with -o json)
--api-key         API key
--secret-id       Tencent Cloud SecretID
--secret-key      Tencent Cloud SecretKey
--region          Region (default: ap-guangzhou)
--domain          Base domain (default: tencentags.com)
--internal        Use internal endpoints
--non-interactive Disable interactive behaviors
--no-color        Disable ANSI color
```

Environment variables: `AGS_API_KEY`, `E2B_API_KEY`, `TENCENTCLOUD_SECRET_ID`, `TENCENTCLOUD_SECRET_KEY`, `NO_COLOR`, `AGS_NON_INTERACTIVE`.

## Troubleshooting

```bash
# Check configuration
ags status

# Diagnose issues
ags doctor

# See what commands are available for your backend
ags capabilities

# Get command details
ags schema instance.create -o json
```

## License

See [LICENSE](LICENSE-AGS%20CLI.txt).
