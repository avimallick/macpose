package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/avimallick/macpose/internal/applecontainer"
	"github.com/avimallick/macpose/internal/project"
	"github.com/avimallick/macpose/internal/version"
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warn, fail, skip
	Message string `json:"message"`
}

type Report struct {
	System  []Check `json:"system"`
	Project []Check `json:"project"`
	Ready   bool    `json:"ready"`
}

func Run(ctx context.Context, exec applecontainer.Executor, workingDir, composeFile, projectName string) *Report {
	r := &Report{Ready: true}

	r.System = append(r.System, checkOS())
	r.System = append(r.System, checkArch())
	r.System = append(r.System, checkContainerCLI(ctx, exec)...)
	r.System = append(r.System, checkMacOSVersion())
	r.System = append(r.System, Check{
		Name:    "macpose binary",
		Status:  "ok",
		Message: "version " + version.String(),
	})

	if composeFile != "" {
		r.Project = append(r.Project, Check{
			Name:    "compose file",
			Status:  "ok",
			Message: composeFile,
		})
	} else {
		_, err := project.DiscoverComposeFile(workingDir, "")
		if err != nil {
			r.Project = append(r.Project, Check{
				Name:    "compose file",
				Status:  "warn",
				Message: "no compose file found in current directory",
			})
		}
	}

	if projectName != "" {
		r.Project = append(r.Project, Check{
			Name:    "project name",
			Status:  "ok",
			Message: projectName,
		})
	}

	for _, c := range append(r.System, r.Project...) {
		if c.Status == "fail" {
			r.Ready = false
		}
	}
	return r
}

func checkOS() Check {
	if runtime.GOOS == "darwin" {
		return Check{Name: "macOS detected", Status: "ok", Message: runtime.GOOS}
	}
	return Check{Name: "macOS detected", Status: "fail", Message: fmt.Sprintf("running on %s; Macpose requires macOS", runtime.GOOS)}
}

func checkArch() Check {
	if runtime.GOOS != "darwin" {
		return Check{Name: "Apple silicon", Status: "skip", Message: "not on macOS"}
	}
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return Check{Name: "Apple silicon", Status: "warn", Message: "could not detect architecture"}
	}
	arch := strings.TrimSpace(string(out))
	if arch == "arm64" {
		return Check{Name: "Apple silicon detected", Status: "ok", Message: arch}
	}
	return Check{Name: "Apple silicon", Status: "warn", Message: fmt.Sprintf("%s detected; Apple container is optimized for arm64", arch)}
}

func checkContainerCLI(ctx context.Context, ex applecontainer.Executor) []Check {
	var checks []Check
	path, err := ex.LookPath()
	if err != nil {
		checks = append(checks, Check{Name: "Apple container CLI", Status: "fail", Message: "container not found in PATH"})
		return checks
	}
	checks = append(checks, Check{Name: "Apple container CLI found", Status: "ok", Message: path})

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stdout, stderr, err := ex.Run(ctx, []string{"--version"})
	if err != nil {
		checks = append(checks, Check{Name: "Apple container version", Status: "fail", Message: strings.TrimSpace(stderr)})
		return checks
	}
	ver := strings.TrimSpace(stdout)
	if ver == "" {
		ver = strings.TrimSpace(stderr)
	}
	checks = append(checks, Check{Name: "Apple container version", Status: "ok", Message: ver})

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	_, sterr, err := ex.Run(ctx2, []string{"system", "status"})
	if err != nil {
		checks = append(checks, Check{
			Name:    "container system",
			Status:  "warn",
			Message: fmt.Sprintf("system status check failed: %s (try: container system start)", strings.TrimSpace(sterr)),
		})
	} else {
		checks = append(checks, Check{Name: "container system", Status: "ok", Message: "usable"})
	}
	return checks
}

func checkMacOSVersion() Check {
	if runtime.GOOS != "darwin" {
		return Check{Name: "macOS version", Status: "skip"}
	}
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return Check{Name: "macOS version", Status: "warn", Message: "could not detect version"}
	}
	ver := strings.TrimSpace(string(out))
	return Check{Name: "macOS version", Status: "ok", Message: ver + " (Apple container requires macOS 26+)"}
}

func FormatHuman(r *Report) string {
	var b strings.Builder
	b.WriteString("Macpose doctor\n")
	b.WriteString("\nSystem:\n")
	for _, c := range r.System {
		b.WriteString(fmt.Sprintf("  %s %s", icon(c.Status), c.Name))
		if c.Message != "" {
			b.WriteString(": " + c.Message)
		}
		b.WriteString("\n")
	}
	if len(r.Project) > 0 {
		b.WriteString("\nProject:\n")
		for _, c := range r.Project {
			b.WriteString(fmt.Sprintf("  %s %s", icon(c.Status), c.Name))
			if c.Message != "" {
				b.WriteString(": " + c.Message)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nResult:\n")
	if r.Ready {
		b.WriteString("  Ready.\n")
	} else {
		b.WriteString("  Not ready.\n")
	}
	return b.String()
}

func icon(status string) string {
	switch status {
	case "ok":
		return "✅"
	case "warn":
		return "⚠️"
	case "fail":
		return "❌"
	default:
		return "⏭️"
	}
}

var _ = os.Getenv
