package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultModel = "claude-sonnet-4-6"

// Config holds runtime configuration for smelt.
type Config struct {
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	Verbose bool   `yaml:"-"` // set via flag, not config file
}

// Load reads configuration from the environment and optional config file.
// Environment variables take precedence over the config file.
func Load() (*Config, error) {
	cfg := &Config{
		Model: defaultModel,
	}

	// Try reading config file (optional)
	cfgPath := filepath.Join(os.Getenv("HOME"), ".smelt", "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", cfgPath, err)
		}
	}

	// Environment overrides
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.APIKey = key
	}

	// Ensure model has a value
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}

	return cfg, nil
}

// Validate returns an error if the configuration is unusable.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is not set; export it or add api_key to ~/.smelt/config.yaml")
	}
	return nil
}
