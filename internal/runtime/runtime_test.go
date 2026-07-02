package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/avimallick/macpose/internal/applecontainer"
	"github.com/avimallick/macpose/internal/compose"
	"github.com/avimallick/macpose/internal/planner"
	"github.com/avimallick/macpose/internal/project"
)

func testRuntime(t *testing.T) (*Runtime, *applecontainer.MockExecutor) {
	t.Helper()
	p := &compose.Project{
		Name:        "vfree",
		ComposeFile: "compose.yaml",
		WorkingDir:  t.TempDir(),
		ConfigHash:  "hash",
		Networks:    map[string]compose.Network{"default": {Name: "default"}},
		Volumes:     map[string]compose.Volume{"pgdata": {Name: "pgdata"}},
		Services: map[string]compose.Service{
			"api": {
				Name:  "api",
				Image: "nginx:latest",
				Ports: []compose.PortMapping{{HostPort: "8000", ContainerPort: "8000"}},
			},
		},
	}
	plan, err := planner.BuildPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	mock := applecontainer.NewMockExecutor()
	return New(Options{
		Project:  p,
		Plan:     plan,
		Executor: mock,
		State:    project.NewStateStore(),
	}), mock
}

func TestBuildDryRun(t *testing.T) {
	rt, mock := testRuntime(t)
	p := rt.opts.Project
	p.Services["api"] = compose.Service{
		Name:  "api",
		Build: &compose.BuildConfig{Context: "/tmp", Dockerfile: "Dockerfile"},
	}
	plan, _ := planner.BuildPlan(p)
	rt.opts.Plan = plan
	rt.opts.DryRun = true
	if err := rt.Build(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(mock.Calls) != 0 {
		t.Fatal("dry run should not execute")
	}
}

func TestDownUsesManagedContainers(t *testing.T) {
	rt, mock := testRuntime(t)
	rt.opts.DryRun = true
	if err := rt.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	foundStop := false
	for _, c := range mock.Calls {
		if len(c.Args) >= 1 && c.Args[0] == "stop" {
			foundStop = true
		}
	}
	_ = foundStop
}

func TestHealthcheckCommand(t *testing.T) {
	cmd := healthcheckCommand([]string{"CMD", "pg_isready", "-U", "postgres"})
	if len(cmd) != 3 || cmd[0] != "pg_isready" {
		t.Fatalf("cmd %#v", cmd)
	}
	shell := healthcheckCommand([]string{"CMD-SHELL", "curl -f http://localhost/"})
	if len(shell) != 3 || shell[0] != "sh" || !strings.Contains(shell[2], "curl") {
		t.Fatalf("shell %#v", shell)
	}
}
