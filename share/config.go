package chshare

import (
	"encoding/json"
	"fmt"
)

type Config struct {
	Version string
	Remotes []*Remote
}

func DecodeConfig(b []byte) (*Config, error) {
	c := &Config{}
	err := json.Unmarshal(b, c)
	if err != nil {
		return nil, fmt.Errorf("Invalid JSON config")
	}
	return c, nil
}

func EncodeConfig(c *Config) ([]byte, error) {
	return json.Marshal(c)
}
