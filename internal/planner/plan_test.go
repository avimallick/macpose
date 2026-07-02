package planner

import (
	"strings"
	"testing"

	"github.com/avimallick/macpose/internal/compose"
)

func sampleProject() *compose.Project {
	return &compose.Project{
		Name:        "vfree",
		ComposeFile: "compose.yaml",
		ConfigHash:  "abc123",
		Networks:    map[string]compose.Network{"default": {Name: "default"}},
		Volumes:     map[string]compose.Volume{"pgdata": {Name: "pgdata"}},
		Services: map[string]compose.Service{
			"db": {
				Name:  "db",
				Image: "postgres:16",
				Volumes: []compose.VolumeMount{
					{Type: "volume", Source: "pgdata", Target: "/var/lib/postgresql/data"},
				},
			},
			"api": {
				Name:  "api",
				Build: &compose.BuildConfig{Context: "/tmp/api", Dockerfile: "Dockerfile"},
				Ports: []compose.PortMapping{{HostPort: "8000", ContainerPort: "8000"}},
				DependsOn: []compose.DependsOn{
					{Service: "db", Condition: "service_started"},
				},
				Restart: "unless-stopped",
			},
		},
	}
}

func TestServiceOrder(t *testing.T) {
	order, _, err := ServiceOrder(sampleProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "db" || order[1] != "api" {
		t.Fatalf("order %#v", order)
	}
}

func TestServiceOrderCycle(t *testing.T) {
	p := sampleProject()
	p.Services["db"] = compose.Service{
		Name:      "db",
		Image:     "postgres:16",
		DependsOn: []compose.DependsOn{{Service: "api"}},
	}
	if _, _, err := ServiceOrder(p); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestBuildPlan(t *testing.T) {
	plan, err := BuildPlan(sampleProject())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Networks[0].Name != "vfree_default" {
		t.Fatalf("network %q", plan.Networks[0].Name)
	}
	if len(plan.Volumes) != 1 || plan.Volumes[0].Name != "vfree_pgdata" {
		t.Fatalf("volumes %#v", plan.Volumes)
	}
	if len(plan.Builds) != 1 || plan.Builds[0].ImageTag != "vfree_api:latest" {
		t.Fatalf("builds %#v", plan.Builds)
	}
}

func TestCommandGenerator(t *testing.T) {
	p := sampleProject()
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	gen := NewCommandGenerator(p, plan)
	cmds := gen.AllCommands()
	if len(cmds) < 4 {
		t.Fatalf("expected at least 4 commands, got %d", len(cmds))
	}
	joined := ShellJoin(cmds[0])
	if !strings.Contains(joined, "container network create") {
		t.Fatalf("unexpected first command: %s", joined)
	}
	run := gen.RunForService("api")
	found := false
	for _, a := range run {
		if a == "-p" {
			found = true
		}
	}
	if !found {
		t.Fatalf("run args missing publish: %#v", run)
	}
}
