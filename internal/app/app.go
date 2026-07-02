package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/avimallick/macpose/internal/applecontainer"
	"github.com/avimallick/macpose/internal/compose"
	"github.com/avimallick/macpose/internal/doctor"
	"github.com/avimallick/macpose/internal/output"
	"github.com/avimallick/macpose/internal/planner"
	"github.com/avimallick/macpose/internal/project"
	"github.com/avimallick/macpose/internal/runtime"
	"github.com/avimallick/macpose/internal/version"
	"github.com/spf13/cobra"
)

type Config struct {
	File        string
	ProjectName string
	Verbose     bool
	DryRun      bool
	JSON        bool
	WorkingDir  string
}

type App struct {
	Config   Config
	Executor applecontainer.Executor
	State    *project.StateStore
	Printer  *output.Printer
}

func New(cfg Config) *App {
	if cfg.WorkingDir == "" {
		wd, _ := os.Getwd()
		cfg.WorkingDir = wd
	}
	return &App{
		Config:   cfg,
		Executor: applecontainer.NewShellExecutor(),
		State:    project.NewStateStore(),
		Printer:  output.NewPrinter(cfg.JSON),
	}
}

func (a *App) loadProject() (*compose.Project, error) {
	composeFile, err := project.DiscoverComposeFile(a.Config.WorkingDir, a.Config.File)
	if err != nil {
		return nil, err
	}
	name := a.Config.ProjectName
	if name == "" {
		name = project.DefaultProjectName(a.Config.WorkingDir)
	}
	projectDir := filepath.Dir(composeFile)
	return compose.Load(compose.LoadOptions{
		ProjectName: name,
		ComposeFile: composeFile,
		WorkingDir:  projectDir,
	})
}

func (a *App) loadPlan() (*compose.Project, *planner.Plan, error) {
	p, err := a.loadProject()
	if err != nil {
		return nil, nil, err
	}
	plan, err := planner.BuildPlan(p)
	if err != nil {
		return nil, nil, err
	}
	return p, plan, nil
}

func (a *App) NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "macpose",
		Short: "Compose-style runner for Apple container on macOS",
		Long:  "Macpose is a Compose-style local development runner for Apple Containers.",
	}

	root.PersistentFlags().StringVarP(&a.Config.File, "file", "f", "", "Compose file path")
	root.PersistentFlags().StringVarP(&a.Config.ProjectName, "project-name", "p", "", "Project name")
	root.PersistentFlags().BoolVar(&a.Config.Verbose, "verbose", false, "Verbose output")
	root.PersistentFlags().BoolVar(&a.Config.DryRun, "dry-run", false, "Print actions without executing")
	root.PersistentFlags().BoolVar(&a.Config.JSON, "json", false, "JSON output where supported")

	root.AddCommand(a.cmdVersion())
	root.AddCommand(a.cmdDoctor())
	root.AddCommand(a.cmdCheck())
	root.AddCommand(a.cmdPlan())
	root.AddCommand(a.cmdConvert())
	root.AddCommand(a.cmdBuild())
	root.AddCommand(a.cmdUp())
	root.AddCommand(a.cmdDown())
	root.AddCommand(a.cmdPs())
	root.AddCommand(a.cmdLogs())
	root.AddCommand(a.cmdExec())
	root.CompletionOptions.DisableDefaultCmd = false

	return root
}

func (a *App) cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("macpose version " + version.String())
		},
	}
}

func (a *App) cmdDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check system and project readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			composeFile := a.Config.File
			projectName := a.Config.ProjectName
			if composeFile == "" {
				if p, err := project.DiscoverComposeFile(a.Config.WorkingDir, ""); err == nil {
					composeFile = p
				}
			}
			if projectName == "" && composeFile != "" {
				projectName = project.DefaultProjectName(a.Config.WorkingDir)
			}
			report := doctor.Run(context.Background(), a.Executor, a.Config.WorkingDir, composeFile, projectName)
			if a.Config.JSON {
				return a.Printer.PrintJSON(report)
			}
			fmt.Print(doctor.FormatHuman(report))
			if !report.Ready {
				return fmt.Errorf("not ready")
			}
			return nil
		},
	}
}

func (a *App) cmdCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Parse compose file and print compatibility report",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := a.loadProject()
			if err != nil {
				return err
			}
			report := compose.CheckCompatibility(p)
			if a.Config.JSON {
				return a.Printer.PrintJSON(report)
			}
			fmt.Printf("Project: %s\nCompose file: %s\n\nSupported:\n", report.Project, report.ComposeFile)
			for _, item := range report.Supported {
				fmt.Printf("  ✅ %s\n", item.Path)
			}
			if len(report.Warnings) > 0 {
				fmt.Println("\nWarnings:")
				for _, item := range report.Warnings {
					msg := item.Path
					if item.Message != "" {
						msg += " — " + item.Message
					}
					fmt.Printf("  ⚠️ %s\n", msg)
				}
			}
			if len(report.Unsupported) > 0 {
				fmt.Println("\nUnsupported:")
				for _, item := range report.Unsupported {
					msg := item.Path
					if item.Message != "" {
						msg += " — " + item.Message
					}
					fmt.Printf("  ❌ %s\n", msg)
				}
			}
			if len(report.Invalid) > 0 {
				fmt.Println("\nInvalid:")
				for _, item := range report.Invalid {
					fmt.Printf("  ❌ %s — %s\n", item.Path, item.Message)
				}
			}
			if !report.Usable {
				return fmt.Errorf("compose file is not usable")
			}
			return nil
		},
	}
}

func (a *App) cmdPlan() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Generate execution plan without running",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			if a.Config.JSON {
				return a.Printer.PrintJSON(plan)
			}
			fmt.Printf("Project: %s\nCompose file: %s\n\n", p.Name, p.ComposeFile)
			fmt.Println("Networks:")
			for _, n := range plan.Networks {
				fmt.Printf("  - %s\n", n.Name)
			}
			fmt.Println("\nVolumes:")
			for _, v := range plan.Volumes {
				fmt.Printf("  - %s\n", v.Name)
			}
			fmt.Println("\nBuilds:")
			for _, b := range plan.Builds {
				fmt.Printf("  - %s -> %s (%s)\n", b.Service, b.ImageTag, b.Context)
			}
			fmt.Println("\nServices (start order):")
			for _, svc := range plan.ServiceOrder {
				fmt.Printf("  - %s -> %s\n", svc, plan.ContainerNames[svc])
			}
			if len(plan.DependencyGraph) > 0 {
				fmt.Println("\nDependencies:")
				for _, svc := range plan.ServiceOrder {
					deps := plan.DependencyGraph[svc]
					if len(deps) > 0 {
						fmt.Printf("  %s depends on %s\n", svc, strings.Join(deps, ", "))
					}
				}
			}
			if len(plan.Warnings) > 0 {
				fmt.Println("\nWarnings:")
				for _, w := range plan.Warnings {
					fmt.Printf("  ⚠️ %s\n", w)
				}
			}
			return nil
		},
	}
}

func (a *App) cmdConvert() *cobra.Command {
	return &cobra.Command{
		Use:   "convert",
		Short: "Print equivalent Apple container commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			gen := planner.NewCommandGenerator(p, plan)
			for _, c := range gen.AllCommands() {
				fmt.Println(planner.ShellJoin(c))
			}
			return nil
		},
	}
}

func (a *App) cmdBuild() *cobra.Command {
	return &cobra.Command{
		Use:   "build [service...]",
		Short: "Build services with build specs",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:  p,
				Plan:     plan,
				Executor: a.Executor,
				State:    a.State,
				DryRun:   a.Config.DryRun,
				Verbose:  a.Config.Verbose,
			})
			return rt.Build(context.Background(), args)
		},
	}
}

func (a *App) cmdUp() *cobra.Command {
	var detached bool
	cmd := &cobra.Command{
		Use:   "up [service...]",
		Short: "Create resources and start services",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:  p,
				Plan:     plan,
				Executor: a.Executor,
				State:    a.State,
				DryRun:   a.Config.DryRun,
				Verbose:  a.Config.Verbose,
			})
			return rt.Up(context.Background(), detached, args)
		},
	}
	cmd.Flags().BoolVarP(&detached, "detach", "d", false, "Run in detached mode")
	return cmd
}

func (a *App) cmdDown() *cobra.Command {
	var removeVolumes bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop and remove project containers and network",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:       p,
				Plan:          plan,
				Executor:      a.Executor,
				State:         a.State,
				DryRun:        a.Config.DryRun,
				RemoveVolumes: removeVolumes,
			})
			return rt.Down(context.Background())
		},
	}
	cmd.Flags().BoolVarP(&removeVolumes, "volumes", "v", false, "Remove named volumes")
	return cmd
}

func (a *App) cmdPs() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List project services",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:  p,
				Plan:     plan,
				Executor: a.Executor,
				State:    a.State,
			})
			rows, err := rt.Ps(context.Background())
			if err != nil {
				return err
			}
			if a.Config.JSON {
				return a.Printer.PrintJSON(rows)
			}
			fmt.Printf("%-14s %-8s %-10s %s\n", "Name", "Service", "Status", "Ports")
			for _, row := range rows {
				fmt.Printf("%-14s %-8s %-10s %s\n", row.Name, row.Service, row.Status, row.Ports)
			}
			return nil
		},
	}
}

func (a *App) cmdLogs() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Fetch service logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:  p,
				Plan:     plan,
				Executor: a.Executor,
				State:    a.State,
			})
			return rt.Logs(context.Background(), follow, args, os.Stdout)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

func (a *App) cmdExec() *cobra.Command {
	return &cobra.Command{
		Use:                "exec <service> [command...]",
		Short:              "Execute a command in a service container",
		DisableFlagParsing: false,
		Args:               cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := args[0]
			cmdArgs := args[1:]
			p, plan, err := a.loadPlan()
			if err != nil {
				return err
			}
			rt := runtime.New(runtime.Options{
				Project:  p,
				Plan:     plan,
				Executor: a.Executor,
				State:    a.State,
				DryRun:   a.Config.DryRun,
			})
			interactive := true
			return rt.Exec(context.Background(), service, cmdArgs, interactive)
		},
	}
}
