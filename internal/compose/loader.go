package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

type BuildConfig struct {
	Context    string
	Dockerfile string
}

type PortMapping struct {
	HostIP        string
	HostPort      string
	ContainerPort string
	Protocol      string
}

type VolumeMount struct {
	Type     string // bind, volume
	Source   string
	Target   string
	ReadOnly bool
}

type DependsOn struct {
	Service   string
	Condition string
}

type Healthcheck struct {
	Test     []string
	Interval string
	Timeout  string
	Retries  int
}

type Service struct {
	Name        string
	Image       string
	Build       *BuildConfig
	Ports       []PortMapping
	Environment map[string]string
	EnvFiles    []string
	Volumes     []VolumeMount
	Command     []string
	Entrypoint  []string
	DependsOn   []DependsOn
	WorkingDir  string
	Labels      map[string]string
	Restart     string
	Healthcheck *Healthcheck
	Networks    []string
}

type Network struct {
	Name     string
	External bool
}

type Volume struct {
	Name     string
	External bool
}

type Project struct {
	Name        string
	ComposeFile string
	WorkingDir  string
	ConfigHash  string
	Services    map[string]Service
	Networks    map[string]Network
	Volumes     map[string]Volume
}

type LoadOptions struct {
	ProjectName string
	ComposeFile string
	WorkingDir  string
}

func Load(opts LoadOptions) (*Project, error) {
	if opts.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		opts.WorkingDir = wd
	}

	composePath := opts.ComposeFile
	if !filepath.IsAbs(composePath) {
		composePath = filepath.Join(opts.WorkingDir, composePath)
	}

	raw, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	hash := sha256.Sum256(raw)
	configHash := hex.EncodeToString(hash[:])

	projectOptions, err := composecli.NewProjectOptions(
		[]string{composePath},
		composecli.WithOsEnv,
		composecli.WithDotEnv,
		composecli.WithName(opts.ProjectName),
		composecli.WithWorkingDirectory(opts.WorkingDir),
	)
	if err != nil {
		return nil, fmt.Errorf("compose options: %w", err)
	}

	cp, err := projectOptions.LoadProject(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load compose project: %w", err)
	}

	return normalizeProject(cp, opts.ProjectName, composePath, opts.WorkingDir, configHash)
}

func normalizeProject(cp *types.Project, name, composeFile, workingDir, configHash string) (*Project, error) {
	p := &Project{
		Name:        name,
		ComposeFile: composeFile,
		WorkingDir:  workingDir,
		ConfigHash:  configHash,
		Services:    map[string]Service{},
		Networks:    map[string]Network{},
		Volumes:     map[string]Volume{},
	}

	for n, net := range cp.Networks {
		p.Networks[n] = Network{Name: n, External: bool(net.External)}
	}
	if len(p.Networks) == 0 {
		p.Networks["default"] = Network{Name: "default"}
	}

	for n, vol := range cp.Volumes {
		p.Volumes[n] = Volume{Name: n, External: bool(vol.External)}
	}

	composeDir := filepath.Dir(composeFile)
	for svcName, svc := range cp.Services {
		s, err := normalizeService(svcName, svc, composeDir)
		if err != nil {
			return nil, err
		}
		p.Services[svcName] = s
	}

	return p, nil
}

func normalizeService(name string, svc types.ServiceConfig, composeDir string) (Service, error) {
	s := Service{
		Name:        name,
		Image:       svc.Image,
		Environment: map[string]string{},
		Labels:      map[string]string{},
	}

	if svc.Build != nil {
		ctx := svc.Build.Context
		if ctx == "" {
			ctx = "."
		}
		if !filepath.IsAbs(ctx) {
			ctx = filepath.Join(composeDir, ctx)
		}
		df := svc.Build.Dockerfile
		if df == "" {
			df = "Dockerfile"
		}
		s.Build = &BuildConfig{Context: ctx, Dockerfile: df}
	}

	for _, p := range svc.Ports {
		pm, err := parsePort(p)
		if err != nil {
			return s, fmt.Errorf("service %s ports: %w", name, err)
		}
		s.Ports = append(s.Ports, pm)
	}

	for k, v := range svc.Environment {
		if v == nil {
			s.Environment[k] = ""
		} else {
			s.Environment[k] = *v
		}
	}

	for _, ef := range svc.EnvFiles {
		path := ef.Path
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(composeDir, path)
		}
		s.EnvFiles = append(s.EnvFiles, path)
	}

	for _, v := range svc.Volumes {
		vm, err := parseVolume(v, composeDir)
		if err != nil {
			return s, fmt.Errorf("service %s volumes: %w", name, err)
		}
		s.Volumes = append(s.Volumes, vm)
	}

	if len(svc.Command) > 0 {
		s.Command = append([]string{}, svc.Command...)
	}
	if len(svc.Entrypoint) > 0 {
		s.Entrypoint = append([]string{}, svc.Entrypoint...)
	}

	for dep, cfg := range svc.DependsOn {
		s.DependsOn = append(s.DependsOn, DependsOn{Service: dep, Condition: string(cfg.Condition)})
	}

	s.WorkingDir = svc.WorkingDir
	for k, v := range svc.Labels {
		s.Labels[k] = v
	}
	s.Restart = svc.Restart

	if svc.HealthCheck != nil {
		hc := &Healthcheck{
			Test: append([]string{}, svc.HealthCheck.Test...),
		}
		if svc.HealthCheck.Interval != nil {
			hc.Interval = svc.HealthCheck.Interval.String()
		}
		if svc.HealthCheck.Timeout != nil {
			hc.Timeout = svc.HealthCheck.Timeout.String()
		}
		if svc.HealthCheck.Retries != nil {
			hc.Retries = int(*svc.HealthCheck.Retries)
		}
		s.Healthcheck = hc
	}

	if len(svc.Networks) > 0 {
		for n := range svc.Networks {
			s.Networks = append(s.Networks, n)
		}
	} else {
		s.Networks = []string{"default"}
	}

	return s, nil
}

func parsePort(p types.ServicePortConfig) (PortMapping, error) {
	pm := PortMapping{
		HostIP:        p.HostIP,
		HostPort:      p.Published,
		ContainerPort: fmt.Sprintf("%d", p.Target),
		Protocol:      p.Protocol,
	}
	if pm.HostPort == "" || pm.HostPort == "0" {
		pm.HostPort = pm.ContainerPort
	}
	return pm, nil
}

func parseVolume(v types.ServiceVolumeConfig, composeDir string) (VolumeMount, error) {
	vm := VolumeMount{Target: v.Target, ReadOnly: v.ReadOnly}
	switch v.Type {
	case "", "volume":
		if v.Source == "" {
			vm.Type = "volume"
			vm.Source = ""
		} else {
			vm.Type = "volume"
			vm.Source = v.Source
		}
	case "bind":
		vm.Type = "bind"
		src := v.Source
		if !filepath.IsAbs(src) {
			src = filepath.Join(composeDir, src)
		}
		vm.Source = src
	default:
		vm.Type = v.Type
		vm.Source = v.Source
	}
	return vm, nil
}

func ParseEnvList(items []string) map[string]string {
	out := map[string]string{}
	for _, item := range items {
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 1 {
			out[parts[0]] = ""
		} else {
			out[parts[0]] = parts[1]
		}
	}
	return out
}
