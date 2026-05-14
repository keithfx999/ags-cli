package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/viper"
)

type Config struct {
	Backend  string        `mapstructure:"backend"`
	Output   string        `mapstructure:"output"`
	Region   string        `mapstructure:"region"`
	Domain   string        `mapstructure:"domain"`
	Internal bool          `mapstructure:"internal"`
	Auth     AuthConfig    `mapstructure:"auth"`
	Sandbox  SandboxConfig `mapstructure:"sandbox"`
}

type AuthConfig struct {
	APIKey    string `mapstructure:"api_key"`
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
}

type SandboxConfig struct {
	DefaultUser string `mapstructure:"default_user"`
}

const (
	defaultRegion = "ap-guangzhou"
	defaultDomain = "tencentags.com"
)

func (c *Config) ControlPlaneEndpoint() string {
	if c.Internal {
		return "ags.internal.tencentcloudapi.com"
	}
	return "ags.tencentcloudapi.com"
}

func (c *Config) DataPlaneDomain() string {
	return c.Domain
}

func (c *Config) DataPlaneRegionDomain() string {
	return fmt.Sprintf("%s.%s", c.Region, c.Domain)
}

func (c *Config) E2BControlPlaneEndpoint() string {
	return fmt.Sprintf("https://api.%s.%s", c.Region, c.Domain)
}

var (
	cfg     *Config
	cfgFile string
)

func SetConfigFile(path string) {
	cfgFile = path
}

func Init() error {
	viper.SetConfigType("toml")

	viper.SetDefault("backend", "cloud")
	viper.SetDefault("output", "text")
	viper.SetDefault("region", "")
	viper.SetDefault("domain", "")
	viper.SetDefault("internal", false)

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(home, ".ags")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("AGS")
	viper.AutomaticEnv()

	_ = viper.BindEnv("backend", "AGS_BACKEND")
	_ = viper.BindEnv("output", "AGS_OUTPUT")
	_ = viper.BindEnv("region", "AGS_REGION")
	_ = viper.BindEnv("domain", "AGS_DOMAIN")
	_ = viper.BindEnv("internal", "AGS_INTERNAL")

	_ = viper.BindEnv("auth.api_key", "AGS_API_KEY", "E2B_API_KEY")
	_ = viper.BindEnv("auth.secret_id", "TENCENTCLOUD_SECRET_ID")
	_ = viper.BindEnv("auth.secret_key", "TENCENTCLOUD_SECRET_KEY")
	_ = viper.BindEnv("sandbox.default_user", "AGS_SANDBOX_DEFAULT_USER")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	applyDefaults(cfg)
	warnIfCredentialConfigIsTooPermissive()

	return nil
}

func applyDefaults(c *Config) {
	if c.Region == "" {
		c.Region = defaultRegion
	}
	if c.Domain == "" {
		c.Domain = defaultDomain
	}
	if c.Internal && !strings.HasPrefix(c.Domain, "internal.") {
		c.Domain = "internal." + c.Domain
	}
}

func warnIfCredentialConfigIsTooPermissive() {
	path := viper.ConfigFileUsed()
	if path == "" || !configFileContainsCredentials() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: config file %q contains credentials and is readable by group or others; consider chmod 600.\n", path)
	}
}

func configFileContainsCredentials() bool {
	return viper.InConfig("auth.api_key") ||
		viper.InConfig("auth.secret_id") ||
		viper.InConfig("auth.secret_key")
}

func Get() *Config {
	if cfg == nil {
		cfg = &Config{
			Backend:  "cloud",
			Output:   "text",
			Region:   defaultRegion,
			Domain:   defaultDomain,
			Internal: false,
		}
	}
	return cfg
}

func GetBackend() string    { return Get().Backend }
func SetBackend(b string)   { Get().Backend = b }
func GetOutput() string     { return Get().Output }
func SetOutput(o string)    { Get().Output = o }
func GetRegion() string     { return Get().Region }
func SetRegion(r string)    { Get().Region = r }
func GetDomain() string     { return Get().Domain }
func GetInternal() bool     { return Get().Internal }
func GetAPIKey() string     { return Get().Auth.APIKey }
func SetAPIKey(k string)    { Get().Auth.APIKey = k }
func GetSecretID() string   { return Get().Auth.SecretID }
func SetSecretID(id string) { Get().Auth.SecretID = id }
func GetSecretKey() string  { return Get().Auth.SecretKey }
func SetSecretKey(k string) { Get().Auth.SecretKey = k }
func GetSandboxUser() string { return Get().Sandbox.DefaultUser }
func SetSandboxUser(u string) { Get().Sandbox.DefaultUser = u }

func SetDomain(domain string) {
	c := Get()
	c.Domain = domain
	if c.Internal && !strings.HasPrefix(c.Domain, "internal.") {
		c.Domain = "internal." + c.Domain
	}
}

func SetInternal(internal bool) {
	c := Get()
	c.Internal = internal
	if internal && !strings.HasPrefix(c.Domain, "internal.") {
		c.Domain = "internal." + c.Domain
	} else if !internal && strings.HasPrefix(c.Domain, "internal.") {
		c.Domain = strings.TrimPrefix(c.Domain, "internal.")
	}
}

func Validate() error {
	if err := ValidateBasics(); err != nil {
		return err
	}
	c := Get()
	switch c.Backend {
	case "e2b":
		if c.Auth.APIKey == "" {
			return output.NewAuthError("MISSING_API_KEY",
				"API key is required",
				"Set AGS_API_KEY, E2B_API_KEY, or auth.api_key in config.")
		}
	case "cloud":
		if c.Auth.SecretID == "" || c.Auth.SecretKey == "" {
			return output.NewAuthError("MISSING_CLOUD_CREDENTIALS",
				"cloud API credentials are required",
				"Set TENCENTCLOUD_SECRET_ID/TENCENTCLOUD_SECRET_KEY or auth.secret_id/auth.secret_key in config.")
		}
	}
	return nil
}

func ValidateBasics() error {
	c := Get()
	if c.Backend != "e2b" && c.Backend != "cloud" {
		return fmt.Errorf("invalid backend: %s (must be 'e2b' or 'cloud')", c.Backend)
	}
	if c.Output != "text" && c.Output != "json" && c.Output != "ndjson" {
		return fmt.Errorf("invalid output format: %s (must be 'text', 'json', or 'ndjson')", c.Output)
	}
	return nil
}
