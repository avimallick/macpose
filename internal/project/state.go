package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ServiceState struct {
	Name          string `json:"name"`
	ContainerName string `json:"container_name"`
	Image         string `json:"image,omitempty"`
}

type State struct {
	ProjectName string         `json:"project_name"`
	ComposeFile string         `json:"compose_file"`
	WorkingDir  string         `json:"working_dir"`
	ConfigHash  string         `json:"config_hash"`
	Network     string         `json:"network"`
	Volumes     []string       `json:"volumes,omitempty"`
	Services    []ServiceState `json:"services,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type StateStore struct {
	baseDir string
}

func NewStateStore() *StateStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &StateStore{baseDir: filepath.Join(home, ".macpose", "projects")}
}

func (s *StateStore) path(projectName string) string {
	return filepath.Join(s.baseDir, projectName+".json")
}

func (s *StateStore) Load(projectName string) (*State, error) {
	data, err := os.ReadFile(s.path(projectName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *StateStore) Save(st *State) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC()
	if st.CreatedAt.IsZero() {
		st.CreatedAt = now
	}
	st.UpdatedAt = now
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(st.ProjectName), data, 0o644)
}

func (s *StateStore) Delete(projectName string) error {
	err := os.Remove(s.path(projectName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *StateStore) PathFor(projectName string) string {
	return s.path(projectName)
}

func (s *StateStore) EnsureDir() error {
	return os.MkdirAll(s.baseDir, 0o755)
}

func ServiceHost(project, service string) string {
	return ContainerName(project, service)
}

func FormatStateError(projectName string, err error) error {
	return fmt.Errorf("state for project %q: %w", projectName, err)
}
