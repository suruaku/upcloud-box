package cmd

import (
	"fmt"

	"github.com/ikox01/upcloud-box/internal/config"
)

func loadConfigOrErr() (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("load config %q: %w", cfgFile, err)
	}
	return cfg, nil
}
