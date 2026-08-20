package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	StoragePath   string `json:"storage_path"`
	Theme         string `json:"theme,omitempty"`
	RetentionDays *int   `json:"retention_days,omitempty"`
}

func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".doitdoit_config.json"), nil
}

func LoadConfig() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.RetentionDays != nil && *cfg.RetentionDays < 0 {
		return nil, fmt.Errorf("retention_days must be zero or a positive integer")
	}

	return &cfg, nil
}

// Retention returns the configured retention period. The boolean is false
// when the user has not made a choice yet. Zero days means keep history
// forever.
func (c *Config) Retention() (days int, decided bool) {
	if c.RetentionDays == nil {
		return 0, false
	}
	return *c.RetentionDays, true
}

// SetRetention records an explicit retention choice.
func (c *Config) SetRetention(days int) {
	c.RetentionDays = new(int)
	*c.RetentionDays = days
}

func SaveConfig(cfg *Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "doitdoit-config-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, 0600); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}
	return nil
}

func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not expand ~: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
