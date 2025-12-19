package configs

import (
	"flag"
	"log"
	"os"

	"github.com/hilthontt/visper/internal/infrastructure/env"
)

func DetermineConfigPath() string {
	var configPath string

	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.Parse()

	if configPath == "" {
		configPath = env.GetString("VISPER_CONFIG", "")
	}

	if configPath == "" {
		candidates := []string{
			"./config.yaml",
			"./config.yml",
			"./tmp/config.yml",
			"./tmp/config.yaml", // Add this for tmp directory
			"../../config.yaml", // keep for local dev
			"/etc/visper/config.yaml",
			"/app/config.yaml", // common in Docker
		}

		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	if configPath == "" {
		log.Fatal("config file not found. Use --config or VISPER_CONFIG env")
	}

	return configPath
}
