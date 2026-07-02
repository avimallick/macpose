package planner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/avimallick/macpose/internal/compose"
	"github.com/avimallick/macpose/internal/project"
)

type CommandGenerator struct {
	Project *compose.Project
	Plan    *Plan
}

func NewCommandGenerator(p *compose.Project, plan *Plan) *CommandGenerator {
	return &CommandGenerator{Project: p, Plan: plan}
}

func (g *CommandGenerator) AllCommands() [][]string {
	var cmds [][]string
	for _, n := range g.Plan.Networks {
		cmds = append(cmds, g.NetworkCreate(n))
	}
	for _, v := range g.Plan.Volumes {
		cmds = append(cmds, g.VolumeCreate(v))
	}
	for _, b := range g.Plan.Builds {
		cmds = append(cmds, g.Build(b))
	}
	for _, svcName := range g.Plan.ServiceOrder {
		svc := g.Project.Services[svcName]
		run := findRun(g.Plan, svcName)
		cmds = append(cmds, g.Run(svc, run))
	}
	return cmds
}

func findRun(plan *Plan, service string) RunStep {
	for _, r := range plan.Runs {
		if r.Service == service {
			return r
		}
	}
	return RunStep{Service: service, ContainerName: project.ContainerName(plan.ProjectName, service)}
}

func (g *CommandGenerator) NetworkCreate(step NetworkStep) []string {
	args := []string{"network", "create"}
	args = append(args, labelArgs(step.Labels)...)
	args = append(args, step.Name)
	return args
}

func (g *CommandGenerator) VolumeCreate(step VolumeStep) []string {
	args := []string{"volume", "create"}
	args = append(args, labelArgs(step.Labels)...)
	args = append(args, step.Name)
	return args
}

func (g *CommandGenerator) Build(step BuildStep) []string {
	args := []string{"build", "-t", step.ImageTag}
	if step.Dockerfile != "" && step.Dockerfile != "Dockerfile" {
		args = append(args, "-f", filepath.Join(step.Context, step.Dockerfile))
	}
	args = append(args, step.Context)
	return args
}

func (g *CommandGenerator) Run(svc compose.Service, run RunStep) []string {
	args := []string{"run", "-d", "--name", run.ContainerName, "--network", run.Network}
	args = append(args, labelArgs(macposeLabels(g.Project.Name, svc.Name, g.Project.ConfigHash))...)

	for _, ef := range svc.EnvFiles {
		args = append(args, "--env-file", ef)
	}

	envKeys := make([]string, 0, len(svc.Environment))
	for k := range svc.Environment {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, svc.Environment[k]))
	}

	for other := range g.Project.Services {
		if other == svc.Name {
			continue
		}
		hostKey := project.ServiceHostEnvKey(other)
		hostVal := project.ServiceHost(g.Project.Name, other)
		args = append(args, "-e", fmt.Sprintf("%s=%s", hostKey, hostVal))
	}

	for _, p := range svc.Ports {
		args = append(args, "-p", formatPort(p))
	}

	for _, vm := range svc.Volumes {
		args = append(args, volumeArg(g.Project.Name, vm)...)
	}

	if svc.WorkingDir != "" {
		args = append(args, "--workdir", svc.WorkingDir)
	}

	args = append(args, run.Image)

	if len(svc.Entrypoint) > 0 {
		args = append(args, svc.Entrypoint...)
	}
	if len(svc.Command) > 0 {
		args = append(args, svc.Command...)
	}

	return args
}

func labelArgs(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var args []string
	for _, k := range keys {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return args
}

func formatPort(p compose.PortMapping) string {
	proto := p.Protocol
	if proto != "" && proto != "tcp" {
		if p.HostIP != "" {
			return fmt.Sprintf("%s:%s:%s/%s", p.HostIP, p.HostPort, p.ContainerPort, proto)
		}
		return fmt.Sprintf("%s:%s/%s", p.HostPort, p.ContainerPort, proto)
	}
	if p.HostIP != "" {
		return fmt.Sprintf("%s:%s:%s", p.HostIP, p.HostPort, p.ContainerPort)
	}
	return fmt.Sprintf("%s:%s", p.HostPort, p.ContainerPort)
}

func volumeArg(projectName string, vm compose.VolumeMount) []string {
	if vm.Type == "bind" {
		spec := vm.Source + ":" + vm.Target
		if vm.ReadOnly {
			spec += ":ro"
		}
		return []string{"-v", spec}
	}
	source := vm.Source
	if source != "" {
		source = project.VolumeName(projectName, source)
	}
	if vm.ReadOnly {
		return []string{"--mount", fmt.Sprintf("type=volume,source=%s,target=%s,readonly", source, vm.Target)}
	}
	if source == "" {
		return []string{"-v", vm.Target}
	}
	return []string{"-v", fmt.Sprintf("%s:%s", source, vm.Target)}
}

func (g *CommandGenerator) RunForService(service string) []string {
	svc := g.Project.Services[service]
	run := findRun(g.Plan, service)
	return g.Run(svc, run)
}

func ShellJoin(args []string, redactSecrets bool) string {
	if redactSecrets {
		args = RedactCommandArgs(args)
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "container")
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
