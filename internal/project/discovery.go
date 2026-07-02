package project

import (
	"fmt"
	"os"
	"path/filepath"
)

var defaultComposeFiles = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

func DiscoverComposeFile(workingDir, explicit string) (string, error) {
	if explicit != "" {
		path := explicit
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("compose file not found: %s", path)
		}
		return path, nil
	}

	for _, name := range defaultComposeFiles {
		path := filepath.Join(workingDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no compose file found in %s (tried: %v)", workingDir, defaultComposeFiles)
}
