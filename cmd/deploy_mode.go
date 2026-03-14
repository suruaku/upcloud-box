package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ikox01/upcloud-box/internal/config"
)

type deployMode string

const (
	deployModeSingle  deployMode = "single"
	deployModeCompose deployMode = "compose"
)

func detectDeployMode(configPath string) (deployMode, string, string, error) {
	configDir, err := resolveConfigDir(configPath)
	if err != nil {
		return deployModeSingle, "", "", err
	}

	for _, name := range []string{"docker-compose.yaml", "compose.yaml"} {
		fullPath := filepath.Join(configDir, name)
		if st, statErr := os.Stat(fullPath); statErr == nil && !st.IsDir() {
			return deployModeCompose, fullPath, name, nil
		}
	}

	return deployModeSingle, "", "", nil
}

func resolveConfigDir(configPath string) (string, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", fmt.Errorf("config path is required")
	}

	absolutePath, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("resolve config path %q: %w", configPath, err)
	}

	return filepath.Dir(filepath.Clean(absolutePath)), nil
}

var nonProjectCharPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func remoteProjectName(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return "app"
	}

	sanitized := nonProjectCharPattern.ReplaceAllString(project, "-")
	sanitized = strings.Trim(sanitized, "-.")
	if sanitized == "" {
		return "app"
	}

	return sanitized
}

func remoteComposeDir(project, sshUser string) string {
	sshUser = strings.TrimSpace(sshUser)
	if sshUser == "" {
		sshUser = "ubuntu"
	}

	return filepath.ToSlash(filepath.Join("/home", sshUser, ".upcloud-box", remoteProjectName(project), "current"))
}

func hasLikelySingleDeploySettings(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	return strings.TrimSpace(cfg.Deploy.ContainerName) != "" ||
		strings.TrimSpace(cfg.Deploy.Image) != "" ||
		strings.TrimSpace(cfg.Deploy.Port) != ""
}
