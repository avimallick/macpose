package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"vfree", "vfree"},
		{"My Project!", "my_project"},
		{"  spaced  ", "spaced"},
		{"___", "project"},
		{"FastAPI-App", "fastapi-app"},
	}
	for _, tc := range tests {
		if got := SanitizeName(tc.in); got != tc.want {
			t.Fatalf("SanitizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiscoverComposeFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := DiscoverComposeFile(dir, ""); err == nil {
		t.Fatal("expected error when no compose file")
	}

	for i, name := range defaultComposeFiles {
		if i > 0 {
			_ = os.Remove(filepath.Join(dir, defaultComposeFiles[i-1]))
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := DiscoverComposeFile(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	}

	custom := filepath.Join(dir, "custom.yml")
	if err := os.WriteFile(custom, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverComposeFile(dir, "custom.yml")
	if err != nil {
		t.Fatal(err)
	}
	if got != custom {
		t.Fatalf("got %q want %q", got, custom)
	}
}

func TestContainerNaming(t *testing.T) {
	if got := ContainerName("vfree", "api"); got != "vfree_api" {
		t.Fatalf("got %q", got)
	}
	if got := NetworkName("vfree", "default"); got != "vfree_default" {
		t.Fatalf("got %q", got)
	}
	if got := VolumeName("vfree", "pgdata"); got != "vfree_pgdata" {
		t.Fatalf("got %q", got)
	}
	if got := ImageTag("vfree", "api"); got != "vfree_api:latest" {
		t.Fatalf("got %q", got)
	}
}
