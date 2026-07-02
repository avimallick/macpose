package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/avimallick/macpose/internal/applecontainer"
	"github.com/avimallick/macpose/internal/compose"
	"github.com/avimallick/macpose/internal/planner"
	"github.com/avimallick/macpose/internal/project"
)

type Options struct {
	Project       *compose.Project
	Plan          *planner.Plan
	Executor      applecontainer.Executor
	State         *project.StateStore
	DryRun        bool
	Verbose       bool
	RemoveVolumes bool
	Services      []string
}

type Runtime struct {
	opts Options
	gen  *planner.CommandGenerator
}

func New(opts Options) *Runtime {
	return &Runtime{
		opts: opts,
		gen:  planner.NewCommandGenerator(opts.Project, opts.Plan),
	}
}

func (r *Runtime) Build(ctx context.Context, services []string) error {
	targets := r.filterBuilds(services)
	for _, b := range targets {
		args := r.gen.Build(b)
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
			continue
		}
		if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			return applecontainer.WrapError("Failed to build service", b.Service, args, stderr, err, "macpose build "+b.Service)
		}
	}
	return nil
}

func (r *Runtime) Up(ctx context.Context, detached bool, services []string) error {
	order := r.filterServiceOrder(services)

	for _, n := range r.opts.Plan.Networks {
		args := r.gen.NetworkCreate(n)
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
			continue
		}
		if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			if !alreadyExists(stderr) {
				return applecontainer.WrapError("Failed to create network", n.Name, args, stderr, err, "macpose down && macpose up -d")
			}
		}
	}

	for _, v := range r.opts.Plan.Volumes {
		args := r.gen.VolumeCreate(v)
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
			continue
		}
		if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			if !alreadyExists(stderr) {
				return applecontainer.WrapError("Failed to create volume", v.Name, args, stderr, err, "")
			}
		}
	}

	builds := r.filterBuilds(services)
	for _, b := range builds {
		if err := r.Build(ctx, []string{b.Service}); err != nil {
			return err
		}
	}

	for _, svcName := range order {
		svc := r.opts.Project.Services[svcName]
		for _, dep := range svc.DependsOn {
			if dep.Condition == "service_healthy" {
				if err := r.waitHealthy(ctx, dep.Service); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
			}
		}

		args := r.gen.RunForService(svcName)
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
			continue
		}
		if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			hint := "macpose down\nmacpose up -d"
			if strings.Contains(stderr, "address already in use") || strings.Contains(stderr, "port") {
				hint = "lsof -i :PORT\nmacpose down\nmacpose up -d"
			}
			return applecontainer.WrapError("Failed to start service", svcName, args, stderr, err, hint)
		}
	}

	if err := r.saveState(order); err != nil {
		return err
	}

	if r.opts.DryRun {
		return nil
	}

	if detached {
		r.printUpSummary(order)
		return nil
	}

	return r.followLogs(ctx, order)
}

func (r *Runtime) Down(ctx context.Context) error {
	containers, err := r.listProjectContainers(ctx)
	if err != nil {
		return err
	}

	for _, c := range containers {
		args := []string{"stop", c}
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
		} else if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			return applecontainer.WrapError("Failed to stop container", c, args, stderr, err, "")
		}

		args = []string{"delete", c}
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
		} else if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			return applecontainer.WrapError("Failed to remove container", c, args, stderr, err, "")
		}
	}

	for _, n := range r.opts.Plan.Networks {
		args := []string{"network", "delete", n.Name}
		if r.opts.DryRun {
			fmt.Println(applecontainer.FormatCommand(args))
		} else if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
			if !notFound(stderr) {
				return applecontainer.WrapError("Failed to delete network", n.Name, args, stderr, err, "")
			}
		}
	}

	if r.opts.RemoveVolumes {
		for _, v := range r.opts.Plan.Volumes {
			args := []string{"volume", "delete", v.Name}
			if r.opts.DryRun {
				fmt.Println(applecontainer.FormatCommand(args))
			} else if _, stderr, err := r.opts.Executor.Run(ctx, args); err != nil {
				if !notFound(stderr) && !inUse(stderr) {
					return applecontainer.WrapError("Failed to delete volume", v.Name, args, stderr, err, "")
				}
			}
		}
	}

	if !r.opts.DryRun {
		_ = r.opts.State.Delete(r.opts.Project.Name)
	}
	return nil
}

type ContainerRow struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
}

func (r *Runtime) Ps(ctx context.Context) ([]ContainerRow, error) {
	containers, err := r.listProjectContainersWithStatus(ctx)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func (r *Runtime) Logs(ctx context.Context, follow bool, services []string, out io.Writer) error {
	targets := r.resolveServices(services)
	if len(targets) == 0 {
		targets = r.opts.Plan.ServiceOrder
	}

	if len(targets) == 1 && !follow {
		cname := project.ContainerName(r.opts.Project.Name, targets[0])
		args := []string{"logs", cname}
		stdout, stderr, err := r.opts.Executor.Run(ctx, args)
		if err != nil {
			return applecontainer.WrapError("Failed to fetch logs", targets[0], args, stderr, err, "")
		}
		fmt.Fprint(out, stdout)
		return nil
	}

	if !follow {
		for _, svc := range targets {
			cname := project.ContainerName(r.opts.Project.Name, svc)
			args := []string{"logs", cname}
			stdout, stderr, err := r.opts.Executor.Run(ctx, args)
			if err != nil {
				return applecontainer.WrapError("Failed to fetch logs", svc, args, stderr, err, "")
			}
			for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
				if line == "" {
					continue
				}
				fmt.Fprintf(out, "%-8s| %s\n", svc, line)
			}
		}
		return nil
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	errCh := make(chan error, len(targets))
	for _, svc := range targets {
		svc := svc
		wg.Add(1)
		go func() {
			defer wg.Done()
			cname := project.ContainerName(r.opts.Project.Name, svc)
			args := []string{"logs", "--follow", cname}
			stdout, stderr, err := r.opts.Executor.Run(ctx, args)
			if err != nil && ctx.Err() == nil {
				errCh <- applecontainer.WrapError("Failed to follow logs", svc, args, stderr, err, "")
			}
			for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
				if line == "" {
					continue
				}
				fmt.Fprintf(out, "%-8s| %s\n", svc, line)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) Exec(ctx context.Context, service string, cmdArgs []string, interactive bool) error {
	cname := project.ContainerName(r.opts.Project.Name, service)
	args := []string{"exec"}
	if interactive {
		args = append(args, "-i", "-t")
	}
	args = append(args, cname)
	args = append(args, cmdArgs...)

	if r.opts.DryRun {
		fmt.Println(applecontainer.FormatCommand(args))
		return nil
	}

	// For interactive exec, use os/exec directly through executor is limited;
	// shell executor handles non-tty; for MVP run via executor.
	_, stderr, err := r.opts.Executor.Run(ctx, args)
	if err != nil {
		return applecontainer.WrapError("Failed to exec into service", service, args, stderr, err, "macpose ps")
	}
	return nil
}

func (r *Runtime) saveState(order []string) error {
	st := &project.State{
		ProjectName: r.opts.Project.Name,
		ComposeFile: r.opts.Project.ComposeFile,
		WorkingDir:  r.opts.Project.WorkingDir,
		ConfigHash:  r.opts.Project.ConfigHash,
		Network:     r.opts.Plan.Networks[0].Name,
	}
	for _, v := range r.opts.Plan.Volumes {
		st.Volumes = append(st.Volumes, v.Name)
	}
	for _, svc := range order {
		s := r.opts.Project.Services[svc]
		image := s.Image
		if s.Build != nil {
			image = project.ImageTag(r.opts.Project.Name, svc)
		}
		st.Services = append(st.Services, project.ServiceState{
			Name:          svc,
			ContainerName: project.ContainerName(r.opts.Project.Name, svc),
			Image:         image,
		})
	}
	return r.opts.State.Save(st)
}

func (r *Runtime) printUpSummary(order []string) {
	fmt.Printf("Started project %s\n\nServices:\n", r.opts.Project.Name)
	for _, svc := range order {
		cname := project.ContainerName(r.opts.Project.Name, svc)
		fmt.Printf("  ✅ %-8s %-14s running\n", svc, cname)
	}
	fmt.Println("\nPorts:")
	for _, svc := range order {
		s := r.opts.Project.Services[svc]
		for _, p := range s.Ports {
			host := p.HostIP
			if host == "" {
				host = "127.0.0.1"
			}
			fmt.Printf("  %s: %s:%s -> %s\n", svc, host, p.HostPort, p.ContainerPort)
		}
	}
	fmt.Println("\nTry:")
	fmt.Println("  macpose logs -f api")
	fmt.Println("  macpose exec api sh")
	fmt.Println("  macpose down")
}

func (r *Runtime) followLogs(ctx context.Context, order []string) error {
	fmt.Printf("Started project %s (foreground log streaming; Ctrl+C stops logs, containers keep running)\n\n", r.opts.Project.Name)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return r.Logs(ctx, true, order, os.Stdout)
}

func (r *Runtime) filterServiceOrder(services []string) []string {
	if len(services) == 0 {
		return r.opts.Plan.ServiceOrder
	}
	want := map[string]bool{}
	for _, s := range services {
		want[s] = true
	}
	var out []string
	for _, s := range r.opts.Plan.ServiceOrder {
		if want[s] {
			out = append(out, s)
		}
	}
	return out
}

func (r *Runtime) filterBuilds(services []string) []planner.BuildStep {
	if len(services) == 0 {
		return r.opts.Plan.Builds
	}
	want := map[string]bool{}
	for _, s := range services {
		want[s] = true
	}
	var out []planner.BuildStep
	for _, b := range r.opts.Plan.Builds {
		if want[b.Service] {
			out = append(out, b)
		}
	}
	return out
}

func (r *Runtime) resolveServices(services []string) []string {
	if len(services) == 0 {
		return nil
	}
	return services
}

func (r *Runtime) listProjectContainers(ctx context.Context) ([]string, error) {
	rows, err := r.listProjectContainersWithStatus(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names, nil
}

type containerListEntry struct {
	Status        string `json:"status"`
	Configuration struct {
		ID     string            `json:"id"`
		Labels map[string]string `json:"labels"`
	} `json:"configuration"`
	Networks []struct {
		Address string `json:"address"`
	} `json:"networks"`
}

func (r *Runtime) listProjectContainersWithStatus(ctx context.Context) ([]ContainerRow, error) {
	args := []string{"list", "--format", "json", "--all"}
	stdout, stderr, err := r.opts.Executor.Run(ctx, args)
	if err != nil {
		return nil, applecontainer.WrapError("Failed to list containers", r.opts.Project.Name, args, stderr, err, "")
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, nil
	}
	var entries []containerListEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		// Some versions may return single object or NDJSON; try wrapping
		if err2 := json.Unmarshal([]byte("["+strings.TrimSuffix(stdout, "\n")+"]"), &entries); err2 != nil {
			return r.containersFromState(), nil
		}
	}

	var rows []ContainerRow
	for _, e := range entries {
		labels := e.Configuration.Labels
		if labels == nil {
			continue
		}
		if labels["com.macpose.project"] != r.opts.Project.Name {
			continue
		}
		if labels["com.macpose.managed"] != "true" {
			continue
		}
		svc := labels["com.macpose.service"]
		name := e.Configuration.ID
		if name == "" {
			name = project.ContainerName(r.opts.Project.Name, svc)
		}
		rows = append(rows, ContainerRow{
			Name:    name,
			Service: svc,
			Status:  e.Status,
			Ports:   r.portsForService(svc),
		})
	}
	if len(rows) == 0 {
		return r.containersFromState(), nil
	}
	return rows, nil
}

func (r *Runtime) containersFromState() []ContainerRow {
	st, _ := r.opts.State.Load(r.opts.Project.Name)
	if st == nil {
		return nil
	}
	var rows []ContainerRow
	for _, s := range st.Services {
		rows = append(rows, ContainerRow{
			Name:    s.ContainerName,
			Service: s.Name,
			Status:  "unknown",
			Ports:   r.portsForService(s.Name),
		})
	}
	return rows
}

func (r *Runtime) portsForService(svc string) string {
	s, ok := r.opts.Project.Services[svc]
	if !ok {
		return ""
	}
	var parts []string
	for _, p := range s.Ports {
		host := p.HostIP
		if host == "" {
			host = "127.0.0.1"
		}
		parts = append(parts, fmt.Sprintf("%s:%s->%s", host, p.HostPort, p.ContainerPort))
	}
	return strings.Join(parts, ", ")
}

func (r *Runtime) waitHealthy(ctx context.Context, service string) error {
	svc, ok := r.opts.Project.Services[service]
	if !ok || svc.Healthcheck == nil || len(svc.Healthcheck.Test) == 0 {
		return fmt.Errorf("service %q has no healthcheck to wait on", service)
	}
	cname := project.ContainerName(r.opts.Project.Name, service)
	retries := svc.Healthcheck.Retries
	if retries <= 0 {
		retries = 10
	}
	interval := 5 * time.Second
	for i := 0; i < retries; i++ {
		cmd := healthcheckCommand(svc.Healthcheck.Test)
		args := append([]string{"exec", cname}, cmd...)
		_, stderr, err := r.opts.Executor.Run(ctx, args)
		if err == nil {
			return nil
		}
		if r.opts.Verbose {
			fmt.Fprintf(os.Stderr, "healthcheck %s attempt %d: %s\n", service, i+1, strings.TrimSpace(stderr))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("service %q did not become healthy in time", service)
}

func healthcheckCommand(test []string) []string {
	if len(test) == 0 {
		return []string{"true"}
	}
	if strings.ToUpper(test[0]) == "CMD" {
		return test[1:]
	}
	if strings.ToUpper(test[0]) == "CMD-SHELL" {
		return []string{"sh", "-c", strings.Join(test[1:], " ")}
	}
	return test
}

func alreadyExists(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "already exists") || strings.Contains(s, "exist")
}

func notFound(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not found") || strings.Contains(s, "no such")
}

func inUse(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "in use")
}
