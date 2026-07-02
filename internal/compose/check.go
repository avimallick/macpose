package compose

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type FieldStatus string

const (
	StatusSupported   FieldStatus = "supported"
	StatusWarning     FieldStatus = "warning"
	StatusUnsupported FieldStatus = "unsupported"
	StatusInvalid     FieldStatus = "invalid"
)

type CheckItem struct {
	Path    string      `json:"path"`
	Status  FieldStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

type CheckReport struct {
	Project     string      `json:"project"`
	ComposeFile string      `json:"compose_file"`
	Supported   []CheckItem `json:"supported"`
	Warnings    []CheckItem `json:"warnings"`
	Unsupported []CheckItem `json:"unsupported"`
	Invalid     []CheckItem `json:"invalid"`
	Usable      bool        `json:"usable"`
}

func CheckCompatibility(p *Project) *CheckReport {
	r := &CheckReport{
		Project:     p.Name,
		ComposeFile: p.ComposeFile,
		Usable:      true,
	}

	for name, svc := range p.Services {
		prefix := "services." + name

		if svc.Image != "" {
			r.addSupported(prefix + ".image")
		}
		if svc.Build != nil {
			r.addSupported(prefix + ".build")
			if svc.Build.Dockerfile != "" && svc.Build.Dockerfile != "Dockerfile" {
				r.addWarning(prefix+".build.dockerfile", "custom Dockerfile path; verify Apple container build -f support")
			}
		}
		if svc.Image == "" && svc.Build == nil {
			r.addInvalid(prefix, "service must define image or build")
		}

		if len(svc.Ports) > 0 {
			r.addSupported(prefix + ".ports")
		}
		if len(svc.Environment) > 0 {
			r.addSupported(prefix + ".environment")
		}
		if len(svc.EnvFiles) > 0 {
			r.addSupported(prefix + ".env_file")
		}
		if len(svc.Volumes) > 0 {
			r.addSupported(prefix + ".volumes")
		}
		if len(svc.Command) > 0 {
			r.addSupported(prefix + ".command")
		}
		if len(svc.Entrypoint) > 0 {
			r.addSupported(prefix + ".entrypoint")
		}
		if svc.WorkingDir != "" {
			r.addSupported(prefix + ".working_dir")
		}
		if len(svc.Labels) > 0 {
			r.addSupported(prefix + ".labels")
		}
		if len(svc.DependsOn) > 0 {
			r.addSupported("depends_on startup ordering")
			for _, d := range svc.DependsOn {
				if d.Condition == "service_healthy" {
					r.addWarning(prefix+".depends_on", "uses service_healthy; Macpose polls healthchecks best-effort")
				} else if d.Condition != "" && d.Condition != "service_started" {
					r.addWarning(prefix+".depends_on", fmt.Sprintf("condition %q may not be fully supported", d.Condition))
				} else {
					r.addWarning(prefix+".depends_on", "uses startup order only")
				}
			}
		}
		if svc.Healthcheck != nil {
			r.addSupported(prefix + ".healthcheck")
		}
		if svc.Restart != "" {
			r.addWarning(prefix+".restart", "parsed but not enforced by Macpose MVP")
		}
	}

	r.addWarning("service discovery", "containers resolve by Macpose container name (e.g. "+p.Name+"_db), not bare service name; use MACPOSE_SERVICE_*_HOST env vars")

	scanUnsupportedYAML(p.ComposeFile, r)
	return r
}

func (r *CheckReport) addSupported(path string) {
	r.Supported = append(r.Supported, CheckItem{Path: path, Status: StatusSupported})
}

func (r *CheckReport) addWarning(path, msg string) {
	r.Warnings = append(r.Warnings, CheckItem{Path: path, Status: StatusWarning, Message: msg})
}

func (r *CheckReport) addUnsupported(path, msg string) {
	r.Unsupported = append(r.Unsupported, CheckItem{Path: path, Status: StatusUnsupported, Message: msg})
}

func (r *CheckReport) addInvalid(path, msg string) {
	r.Invalid = append(r.Invalid, CheckItem{Path: path, Status: StatusInvalid, Message: msg})
	r.Usable = false
}

var unsupportedTopLevel = map[string]string{
	"secrets":  "secrets are not supported",
	"configs":  "configs are not supported",
	"profiles": "profiles are not supported at runtime",
}

var unsupportedServiceFields = map[string]string{
	"deploy":          "deploy is not supported",
	"extends":         "extends is not supported",
	"scale":           "scale is not supported",
	"network_mode":    "network_mode is not supported",
	"privileged":      "privileged is not supported",
	"pid":             "pid mode is not supported",
	"devices":         "devices are not supported",
	"cgroup":          "cgroup is not supported",
	"cap_add":         "cap_add is not supported",
	"cap_drop":        "cap_drop is not supported",
	"sysctls":         "sysctls are not supported",
	"tmpfs":           "tmpfs is not supported",
	"ulimits":         "ulimits are not supported",
	"user":            "user is parsed by compose-go but not mapped to container run flags yet",
	"extra_hosts":     "extra_hosts are not supported",
	"dns":             "dns overrides are not supported",
	"ipc":             "ipc mode is not supported",
	"mem_limit":       "mem_limit is not supported",
	"credential_spec": "credential_spec is not supported",
}

func scanUnsupportedYAML(composeFile string, r *CheckReport) {
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return
	}
	doc := root.Content[0]
	if doc == nil || doc.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		if msg, ok := unsupportedTopLevel[key]; ok {
			r.addUnsupported(key, msg)
		}
		if key == "services" {
			scanServices(doc.Content[i+1], r)
		}
	}
}

func scanServices(node *yaml.Node, r *CheckReport) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		svcName := node.Content[i].Value
		svcNode := node.Content[i+1]
		if svcNode == nil || svcNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(svcNode.Content); j += 2 {
			field := svcNode.Content[j].Value
			path := "services." + svcName + "." + field
			if msg, ok := unsupportedServiceFields[field]; ok {
				r.addUnsupported(path, msg)
			}
			if field == "volumes" {
				scanVolumeMounts(svcNode.Content[j+1], svcName, r)
			}
		}
	}
}

func scanVolumeMounts(node *yaml.Node, svcName string, r *CheckReport) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			continue
		}
		if strings.Contains(item.Value, "/var/run/docker.sock") || strings.Contains(item.Value, "docker.sock") {
			r.addUnsupported("services."+svcName+".volumes", "Docker socket mounts are not supported")
		}
	}
}
