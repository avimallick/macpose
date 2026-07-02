package planner

import (
	"fmt"
	"sort"

	"github.com/avimallick/macpose/internal/compose"
	"github.com/avimallick/macpose/internal/project"
)

type NetworkStep struct {
	Name   string
	Labels map[string]string
}

type VolumeStep struct {
	Name   string
	Labels map[string]string
}

type BuildStep struct {
	Service    string
	ImageTag   string
	Context    string
	Dockerfile string
	Labels     map[string]string
}

type RunStep struct {
	Service       string
	ContainerName string
	Image         string
	Network       string
	Args          []string
	Ports         []compose.PortMapping
	Warnings      []string
}

type Plan struct {
	ProjectName     string              `json:"project_name"`
	ComposeFile     string              `json:"compose_file"`
	ConfigHash      string              `json:"config_hash"`
	Networks        []NetworkStep       `json:"networks"`
	Volumes         []VolumeStep        `json:"volumes"`
	Builds          []BuildStep         `json:"builds"`
	Runs            []RunStep           `json:"runs"`
	ServiceOrder    []string            `json:"service_order"`
	ContainerNames  map[string]string   `json:"container_names"`
	DependencyGraph map[string][]string `json:"dependency_graph"`
	Warnings        []string            `json:"warnings"`
}

func BuildPlan(p *compose.Project) (*Plan, error) {
	order, graph, err := ServiceOrder(p)
	if err != nil {
		return nil, err
	}

	plan := &Plan{
		ProjectName:     p.Name,
		ComposeFile:     p.ComposeFile,
		ConfigHash:      p.ConfigHash,
		ServiceOrder:    order,
		DependencyGraph: graph,
		ContainerNames:  map[string]string{},
	}

	networkName := project.NetworkName(p.Name, "default")
	plan.Networks = append(plan.Networks, NetworkStep{
		Name:   networkName,
		Labels: macposeLabels(p.Name, "", p.ConfigHash),
	})

	volSeen := map[string]bool{}
	for _, svcName := range order {
		svc := p.Services[svcName]
		plan.ContainerNames[svcName] = project.ContainerName(p.Name, svcName)

		for _, vm := range svc.Volumes {
			if vm.Type == "volume" && vm.Source != "" && !volSeen[vm.Source] {
				volSeen[vm.Source] = true
				if vol, ok := p.Volumes[vm.Source]; ok && vol.External {
					continue
				}
				plan.Volumes = append(plan.Volumes, VolumeStep{
					Name:   project.VolumeName(p.Name, vm.Source),
					Labels: macposeLabels(p.Name, "", p.ConfigHash),
				})
			}
		}

		if svc.Build != nil {
			plan.Builds = append(plan.Builds, BuildStep{
				Service:    svcName,
				ImageTag:   project.ImageTag(p.Name, svcName),
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
				Labels:     macposeLabels(p.Name, svcName, p.ConfigHash),
			})
		}

		image := svc.Image
		if svc.Build != nil {
			image = project.ImageTag(p.Name, svcName)
		}

		run := RunStep{
			Service:       svcName,
			ContainerName: project.ContainerName(p.Name, svcName),
			Image:         image,
			Network:       networkName,
			Ports:         svc.Ports,
		}

		if svc.Restart != "" {
			run.Warnings = append(run.Warnings, fmt.Sprintf("restart policy %q is not enforced", svc.Restart))
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("services.%s.restart parsed but not enforced", svcName))
		}

		plan.Runs = append(plan.Runs, run)
	}

	plan.Warnings = append(plan.Warnings,
		"Service discovery uses container hostnames (e.g. "+project.ContainerName(p.Name, "db")+"), not bare service names",
	)

	return plan, nil
}

func macposeLabels(projectName, service, configHash string) map[string]string {
	l := map[string]string{
		"com.macpose.project":     projectName,
		"com.macpose.managed":     "true",
		"com.macpose.config-hash": configHash,
	}
	if service != "" {
		l["com.macpose.service"] = service
	}
	return l
}

func ServiceOrder(p *compose.Project) ([]string, map[string][]string, error) {
	names := make([]string, 0, len(p.Services))
	for n := range p.Services {
		names = append(names, n)
	}
	sort.Strings(names)

	graph := map[string][]string{}
	inDegree := map[string]int{}
	for _, n := range names {
		inDegree[n] = 0
		graph[n] = nil
	}

	for svcName, svc := range p.Services {
		for _, dep := range svc.DependsOn {
			if _, ok := p.Services[dep.Service]; !ok {
				return nil, nil, fmt.Errorf("service %q depends on unknown service %q", svcName, dep.Service)
			}
			graph[dep.Service] = append(graph[dep.Service], svcName)
			inDegree[svcName]++
		}
	}

	queue := []string{}
	for _, n := range names {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}
	sort.Strings(queue)

	order := []string{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		deps := graph[cur]
		sort.Strings(deps)
		for _, next := range deps {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
				sort.Strings(queue)
			}
		}
	}

	if len(order) != len(names) {
		return nil, nil, fmt.Errorf("dependency cycle detected among services")
	}

	depGraph := map[string][]string{}
	for svcName, svc := range p.Services {
		for _, d := range svc.DependsOn {
			depGraph[svcName] = append(depGraph[svcName], d.Service)
		}
		sort.Strings(depGraph[svcName])
	}

	return order, depGraph, nil
}
