package config

import (
	"encoding/json"
	"os"
)

// Load
// 启动时加载配置文件
// 若文件不存在，返回空配置（首次启动）
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
