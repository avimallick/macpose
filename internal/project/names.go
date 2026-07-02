package project

import (
	"path/filepath"
	"regexp"
	"strings"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func SanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = sanitizeRe.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "project"
	}
	return strings.ToLower(name)
}

func DefaultProjectName(workingDir string) string {
	base := filepath.Base(workingDir)
	if base == "." || base == string(filepath.Separator) {
		return "project"
	}
	return SanitizeName(base)
}

func ContainerName(project, service string) string {
	return project + "_" + service
}

func NetworkName(project, network string) string {
	if network == "" || network == "default" {
		return project + "_default"
	}
	return project + "_" + network
}

func VolumeName(project, volume string) string {
	return project + "_" + volume
}

func ImageTag(project, service string) string {
	return project + "_" + service + ":latest"
}

func ServiceHostEnvKey(service string) string {
	key := strings.ToUpper(sanitizeRe.ReplaceAllString(service, "_"))
	return "MACPOSE_SERVICE_" + key + "_HOST"
}
