package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvList(t *testing.T) {
	got := ParseEnvList([]string{"FOO=bar", "BAZ", "EMPTY="})
	if got["FOO"] != "bar" || got["BAZ"] != "" || got["EMPTY"] != "" {
		t.Fatalf("unexpected map: %#v", got)
	}
}

func TestLoadProject(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	content := `services:
  api:
    build: ./api
    ports:
      - "8000:8000"
      - "127.0.0.1:5432:5432"
    environment:
      POSTGRES_PASSWORD: postgres
      NODE_ENV: development
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./src:/app/src:ro
    depends_on:
      db:
        condition: service_healthy
  db:
    image: postgres:16
volumes:
  pgdata:
`
	if err := os.WriteFile(composePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(LoadOptions{
		ProjectName: "vfree",
		ComposeFile: composePath,
		WorkingDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "vfree" {
		t.Fatalf("project name %q", p.Name)
	}
	api := p.Services["api"]
	if api.Build == nil {
		t.Fatal("expected build config")
	}
	if len(api.Ports) != 2 {
		t.Fatalf("ports len %d", len(api.Ports))
	}
	if api.Ports[1].HostIP != "127.0.0.1" {
		t.Fatalf("host ip %q", api.Ports[1].HostIP)
	}
	if api.Environment["POSTGRES_PASSWORD"] != "postgres" {
		t.Fatalf("env not parsed")
	}
	if len(api.Volumes) != 2 {
		t.Fatalf("volumes len %d", len(api.Volumes))
	}
	if !api.Volumes[1].ReadOnly {
		t.Fatal("expected ro bind mount")
	}
	if len(api.DependsOn) != 1 || api.DependsOn[0].Condition != "service_healthy" {
		t.Fatalf("depends_on %#v", api.DependsOn)
	}
}

func TestCheckCompatibilityUnsupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	content := `services:
  api:
    image: nginx:latest
    deploy:
      replicas: 2
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(LoadOptions{ProjectName: "vfree", ComposeFile: path, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	report := CheckCompatibility(p)
	if len(report.Unsupported) == 0 {
		t.Fatal("expected unsupported deploy")
	}
}
