package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const DefaultServiceOrigin = "https://doitdoit-service.davidt-91d.workers.dev"

type Config struct {
	ServiceOrigin string `json:"service_origin,omitempty"`
	Theme         string `json:"theme,omitempty"`
}

func (c *Config) ResolvedServiceOrigin(override string) string {
	origin := strings.TrimSpace(override)
	if origin == "" && c != nil {
		origin = strings.TrimSpace(c.ServiceOrigin)
	}
	if origin == "" {
		origin = DefaultServiceOrigin
	}
	return strings.TrimSuffix(origin, "/")
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
	return &cfg, nil
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
