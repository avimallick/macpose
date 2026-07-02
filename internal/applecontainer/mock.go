package applecontainer

import (
	"context"
	"fmt"
	"strings"
)

type RecordedCall struct {
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

type MockExecutor struct {
	Calls      []RecordedCall
	Responses  map[string]RecordedCall
	DefaultOut string
}

func NewMockExecutor() *MockExecutor {
	return &MockExecutor{Responses: map[string]RecordedCall{}}
}

func (m *MockExecutor) key(args []string) string {
	return strings.Join(args, "\x00")
}

func (m *MockExecutor) SetResponse(args []string, stdout, stderr string, err error) {
	m.Responses[m.key(args)] = RecordedCall{Args: args, Stdout: stdout, Stderr: stderr, Err: err}
}

func (m *MockExecutor) SetPrefixResponse(prefix []string, stdout, stderr string, err error) {
	m.Responses["prefix:"+strings.Join(prefix, " ")] = RecordedCall{Stdout: stdout, Stderr: stderr, Err: err}
}

func (m *MockExecutor) LookPath() (string, error) {
	return "/usr/local/bin/container", nil
}

func (m *MockExecutor) Run(ctx context.Context, args []string) (string, string, error) {
	if resp, ok := m.Responses[m.key(args)]; ok {
		m.Calls = append(m.Calls, RecordedCall{Args: args, Stdout: resp.Stdout, Stderr: resp.Stderr, Err: resp.Err})
		return resp.Stdout, resp.Stderr, resp.Err
	}
	for k, resp := range m.Responses {
		if strings.HasPrefix(k, "prefix:") {
			prefix := strings.TrimPrefix(k, "prefix:")
			if strings.HasPrefix(strings.Join(args, " "), prefix) {
				m.Calls = append(m.Calls, RecordedCall{Args: args, Stdout: resp.Stdout, Stderr: resp.Stderr, Err: resp.Err})
				return resp.Stdout, resp.Stderr, resp.Err
			}
		}
	}
	m.Calls = append(m.Calls, RecordedCall{Args: args, Stdout: m.DefaultOut})
	return m.DefaultOut, "", nil
}

func (m *MockExecutor) LastCall() (*RecordedCall, error) {
	if len(m.Calls) == 0 {
		return nil, fmt.Errorf("no calls recorded")
	}
	c := m.Calls[len(m.Calls)-1]
	return &c, nil
}
