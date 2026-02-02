package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   string `yaml:"server"`
	APIKey   string `yaml:"api_key"`
	Insecure bool   `yaml:"insecure"`
	Verbose  bool   `yaml:"-"`
}

func configPaths() []string {
	paths := []string{
		"./berth-cli.yaml",
	}

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths, filepath.Join(xdgConfig, "berth-cli", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "berth-cli", "config.yaml"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".berth-cli.yaml"))
	}

	return paths
}

func loadFromFile() (*Config, string, error) {
	for _, path := range configPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", fmt.Errorf("failed to read config file %s: %w", path, err)
		}

		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, "", fmt.Errorf("failed to parse config file %s: %w", path, err)
		}

		return &cfg, path, nil
	}

	return &Config{}, "", nil
}

func loadFromEnv(cfg *Config) {
	if server := os.Getenv("BERTH_SERVER_URL"); server != "" {
		cfg.Server = server
	}
	if apiKey := os.Getenv("BERTH_API_KEY"); apiKey != "" {
		cfg.APIKey = apiKey
	}
	if insecure := os.Getenv("BERTH_INSECURE"); insecure != "" {
		cfg.Insecure = insecure == "true" || insecure == "1" || insecure == "yes"
	}
}

func applyFlags(cfg *Config, server, apiKey string, insecure bool, insecureSet bool, verbose bool) {
	if server != "" {
		cfg.Server = server
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	if insecureSet {
		cfg.Insecure = insecure
	}
	cfg.Verbose = verbose
}

func normalizeURL(url string) string {
	return strings.TrimRight(url, "/")
}

func Load(flagServer, flagAPIKey string, flagInsecure bool, flagInsecureSet bool, flagVerbose bool) (*Config, error) {
	cfg, _, err := loadFromFile()
	if err != nil {
		return nil, err
	}

	loadFromEnv(cfg)

	applyFlags(cfg, flagServer, flagAPIKey, flagInsecure, flagInsecureSet, flagVerbose)

	cfg.Server = normalizeURL(cfg.Server)

	if cfg.Server == "" {
		return nil, fmt.Errorf("server URL is required (use --server, BERTH_SERVER_URL, or config file)")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required (use --api-key, BERTH_API_KEY, or config file)")
	}

	return cfg, nil
}
