package settings

import (
	"encoding/json"
	"errors"
)

type Config struct {
	Version string
	Remotes
}

func DecodeConfig(b []byte) (*Config, error) {
	c := &Config{}
	err := json.Unmarshal(b, c)
	if err != nil {
		return nil, errors.New("Invalid JSON config")
	}
	return c, nil
}

func EncodeConfig(c Config) []byte {
	//Config doesn't have types that can fail to marshal
	b, _ := json.Marshal(c)
	return b
}
