package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the ghostctl config file (default ~/.ghostctl/config.yaml).
type Config struct {
	Oracle       OracleConfig       `mapstructure:"oracle"`
	Cloudflare   CloudflareConfig   `mapstructure:"cloudflare"`
	ControlPlane ControlPlaneConfig `mapstructure:"control_plane"`
	Rotate       RotateConfig       `mapstructure:"rotate"`
	Nodes        []NodeConfig       `mapstructure:"nodes"`
}

type OracleConfig struct {
	// ConfigFile reuses an OCI CLI config (~/.oci/config) when the explicit
	// fields below are left empty.
	ConfigFile     string `mapstructure:"config_file"`
	Profile        string `mapstructure:"profile"`
	TenancyOCID    string `mapstructure:"tenancy_ocid"`
	UserOCID       string `mapstructure:"user_ocid"`
	Fingerprint    string `mapstructure:"fingerprint"`
	Region         string `mapstructure:"region"`
	PrivateKeyPath string `mapstructure:"private_key_path"`
}

type CloudflareConfig struct {
	APIToken string `mapstructure:"api_token"`
	ZoneID   string `mapstructure:"zone_id"`
	TTL      int    `mapstructure:"ttl"`
}

type ControlPlaneConfig struct {
	URL        string `mapstructure:"url"`
	AdminToken string `mapstructure:"admin_token"`
}

type RotateConfig struct {
	Attempts      int           `mapstructure:"attempts"`
	Probe         bool          `mapstructure:"probe"`
	ProbeTimeout  time.Duration `mapstructure:"probe_timeout"`
	ProbeAttempts int           `mapstructure:"probe_attempts"`
	BurnWindow    time.Duration `mapstructure:"burn_window"`
	BurnedStore   string        `mapstructure:"burned_store"`
}

type NodeConfig struct {
	Name         string `mapstructure:"name"`
	InstanceOCID string `mapstructure:"instance_ocid"`
	DNSRecord    string `mapstructure:"dns_record"`
	NodeID       string `mapstructure:"node_id"`
	SSHHost      string `mapstructure:"ssh_host"`
	ProbePort    int    `mapstructure:"probe_port"`
}

// LoadConfig reads the config file, expanding ${ENV_VAR} references so secrets
// can stay in the environment rather than on disk.
func LoadConfig(path string) (*Config, string, error) {
	resolved, err := resolveConfigPath(path)
	if err != nil {
		return nil, "", err
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, resolved, fmt.Errorf("reading config %s: %w", resolved, err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetDefault("oracle.profile", "DEFAULT")
	v.SetDefault("cloudflare.ttl", 60)
	v.SetDefault("rotate.attempts", 3)
	v.SetDefault("rotate.probe", true)
	v.SetDefault("rotate.probe_timeout", "6s")
	v.SetDefault("rotate.probe_attempts", 2)
	v.SetDefault("rotate.burn_window", "720h")
	v.SetDefault("rotate.burned_store", "~/.ghostctl/burned.json")

	if err := v.ReadConfig(bytes.NewReader([]byte(os.ExpandEnv(string(raw))))); err != nil {
		return nil, resolved, fmt.Errorf("parsing config %s: %w", resolved, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, resolved, fmt.Errorf("decoding config %s: %w", resolved, err)
	}

	cfg.Oracle.ConfigFile = expandHome(cfg.Oracle.ConfigFile)
	cfg.Oracle.PrivateKeyPath = expandHome(cfg.Oracle.PrivateKeyPath)
	cfg.Rotate.BurnedStore = expandHome(cfg.Rotate.BurnedStore)

	if err := cfg.validate(); err != nil {
		return nil, resolved, err
	}
	return &cfg, resolved, nil
}

func (c *Config) validate() error {
	if len(c.Nodes) == 0 {
		return errors.New("config lists no nodes")
	}
	seen := map[string]bool{}
	for i, n := range c.Nodes {
		switch {
		case n.Name == "":
			return fmt.Errorf("nodes[%d]: name is required", i)
		case n.InstanceOCID == "":
			return fmt.Errorf("node %q: instance_ocid is required", n.Name)
		case seen[n.Name]:
			return fmt.Errorf("node %q is defined twice", n.Name)
		}
		seen[n.Name] = true
	}
	return nil
}

// Node finds a configured node by name.
func (c *Config) Node(name string) (NodeConfig, bool) {
	for _, n := range c.Nodes {
		if strings.EqualFold(n.Name, name) {
			return n, true
		}
	}
	return NodeConfig{}, false
}

// resolveConfigPath picks the first config location that exists: the --config
// flag, $GHOSTCTL_CONFIG, ~/.ghostctl/config.yaml, then ./configs/ghostctl.yaml.
func resolveConfigPath(flagValue string) (string, error) {
	if flagValue != "" {
		return expandHome(flagValue), nil
	}
	if env := os.Getenv("GHOSTCTL_CONFIG"); env != "" {
		return expandHome(env), nil
	}

	candidates := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".ghostctl", "config.yaml"),
			filepath.Join(home, ".ghostctl", "config.yml"),
		)
	}
	candidates = append(candidates, filepath.Join("configs", "ghostctl.yaml"))

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no config file found — looked in %s\n\nCopy configs/ghostctl.example.yaml to ~/.ghostctl/config.yaml to get started",
		strings.Join(candidates, ", "))
}

func expandHome(path string) string {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
