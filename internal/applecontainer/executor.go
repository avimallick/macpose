package applecontainer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const Binary = "container"

type Executor interface {
	Run(ctx context.Context, args []string) (stdout, stderr string, err error)
	LookPath() (string, error)
}

type ShellExecutor struct {
	Binary string
}

func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{Binary: Binary}
}

func (e *ShellExecutor) LookPath() (string, error) {
	bin := e.Binary
	if bin == "" {
		bin = Binary
	}
	return exec.LookPath(bin)
}

func (e *ShellExecutor) Run(ctx context.Context, args []string) (string, string, error) {
	bin := e.Binary
	if bin == "" {
		bin = Binary
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func FormatCommand(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, Binary)
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

type CommandError struct {
	Action   string
	Resource string
	Args     []string
	Stderr   string
	Cause    error
	Hint     string
}

func (e *CommandError) Error() string {
	var b strings.Builder
	if e.Action != "" {
		b.WriteString(e.Action)
		if e.Resource != "" {
			b.WriteString(" ")
			b.WriteString(strconvQuote(e.Resource))
		}
		b.WriteString(".\n\n")
	}
	b.WriteString("Apple container command:\n  ")
	b.WriteString(FormatCommand(e.Args))
	b.WriteString("\n")
	if e.Stderr != "" {
		b.WriteString("\nReason:\n  ")
		b.WriteString(strings.TrimSpace(e.Stderr))
		b.WriteString("\n")
	}
	if e.Hint != "" {
		b.WriteString("\nTry:\n  ")
		b.WriteString(e.Hint)
		b.WriteString("\n")
	}
	if e.Cause != nil && e.Stderr == "" {
		b.WriteString("\nReason:\n  ")
		b.WriteString(e.Cause.Error())
		b.WriteString("\n")
	}
	return b.String()
}

func (e *CommandError) Unwrap() error {
	return e.Cause
}

func strconvQuote(s string) string {
	return fmt.Sprintf("%q", s)
}

func WrapError(action, resource string, args []string, stderr string, cause error, hint string) error {
	return &CommandError{
		Action:   action,
		Resource: resource,
		Args:     args,
		Stderr:   stderr,
		Cause:    cause,
		Hint:     hint,
	}
}
