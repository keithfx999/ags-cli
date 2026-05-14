package main

import (
	"github.com/TencentCloudAgentRuntime/ags-cli/cmd"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	cmd.SetVersionInfo(Version, Commit, BuildTime)
	cmd.Execute()
}
