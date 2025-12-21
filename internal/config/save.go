package config

import (
	"encoding/json"
	"os"
)

// Save
// 配置修改后立即落盘
func Save(path string, cfg *Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
