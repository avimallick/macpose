package planner

import (
	"strings"
	"testing"

	"github.com/avimallick/macpose/internal/compose"
)

func TestIsSensitiveEnvKey(t *testing.T) {
	tests := []struct {
		key   string
		want  bool
	}{
		{"SECRET_KEY", true},
		{"POSTGRES_PASSWORD", true},
		{"API_TOKEN", true},
		{"MY_API_KEY", true},
		{"AWS_ACCESS_KEY_ID", true},
		{"PRIVATE_KEY", true},
		{"DB_CREDENTIAL", true},
		{"OAUTH_CLIENT_SECRET", true},
		{"BASIC_AUTH", true},
		{"NODE_ENV", false},
		{"PORT", false},
		{"MACPOSE_SERVICE_DB_HOST", false},
		{"DATABASE_URL", false},
	}
	for _, tt := range tests {
		if got := IsSensitiveEnvKey(tt.key); got != tt.want {
			t.Errorf("IsSensitiveEnvKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestRedactCommandArgs(t *testing.T) {
	args := []string{
		"run", "-d",
		"-e", "SECRET_KEY=super-secret",
		"-e", "NODE_ENV=production",
		"-e", "POSTGRES_PASSWORD=postgres",
		"--env-file", "/tmp/.env",
	}
	got := RedactCommandArgs(args)

	assertArgValue := func(t *testing.T, args []string, key, want string) {
		t.Helper()
		for i := 0; i < len(args); i++ {
			if args[i] != "-e" || i+1 >= len(args) {
				continue
			}
			k, v, ok := strings.Cut(args[i+1], "=")
			if ok && k == key {
				if v != want {
					t.Fatalf("%s=%q, want %q", key, v, want)
				}
				return
			}
		}
		t.Fatalf("missing -e %s in %#v", key, args)
	}

	assertArgValue(t, got, "SECRET_KEY", RedactedEnvValue)
	assertArgValue(t, got, "POSTGRES_PASSWORD", RedactedEnvValue)
	assertArgValue(t, got, "NODE_ENV", "production")

	if got[len(got)-1] != "/tmp/.env" {
		t.Fatalf("env-file path changed: %#v", got)
	}
}

func TestShellJoinRedactsSecretsByDefault(t *testing.T) {
	p := sampleProject()
	p.Services["db"] = compose.Service{
		Name:  "db",
		Image: "postgres:16",
		Environment: map[string]string{
			"POSTGRES_PASSWORD": "super-secret-password",
			"POSTGRES_USER":     "postgres",
		},
		EnvFiles: []string{"/tmp/project/.env"},
	}
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	gen := NewCommandGenerator(p, plan)
	run := gen.RunForService("db")

	redacted := ShellJoin(run, true)
	if strings.Contains(redacted, "super-secret-password") {
		t.Fatalf("password leaked in redacted output: %s", redacted)
	}
	if !strings.Contains(redacted, "POSTGRES_PASSWORD="+RedactedEnvValue) {
		t.Fatalf("expected redacted password in output: %s", redacted)
	}
	if !strings.Contains(redacted, "POSTGRES_USER=postgres") {
		t.Fatalf("expected non-sensitive env preserved: %s", redacted)
	}
	if !strings.Contains(redacted, "--env-file /tmp/project/.env") {
		t.Fatalf("expected env-file path preserved: %s", redacted)
	}

	raw := ShellJoin(run, false)
	if !strings.Contains(raw, "POSTGRES_PASSWORD=super-secret-password") {
		t.Fatalf("expected raw password when redaction disabled: %s", raw)
	}
}

func TestUpUsesUnredactedCommandArgs(t *testing.T) {
	p := sampleProject()
	p.Services["db"] = compose.Service{
		Name:  "db",
		Image: "postgres:16",
		Environment: map[string]string{
			"SECRET_KEY": "real-secret-value",
		},
	}
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	gen := NewCommandGenerator(p, plan)
	run := gen.RunForService("db")

	found := false
	for i := 0; i < len(run); i++ {
		if run[i] != "-e" || i+1 >= len(run) {
			continue
		}
		if run[i+1] == "SECRET_KEY=real-secret-value" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("RunForService should pass real values: %#v", run)
	}
}
