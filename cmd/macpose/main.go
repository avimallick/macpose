package main

import (
	"fmt"
	"os"

	"github.com/avimallick/macpose/internal/app"
	"github.com/avimallick/macpose/internal/version"
)

func main() {
	cfg := app.Config{}
	a := app.New(cfg)
	root := a.NewRootCmd()
	root.Version = version.String()
	root.SetVersionTemplate("macpose version {{.Version}}\n")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
