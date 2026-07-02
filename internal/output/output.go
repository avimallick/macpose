package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	IconOK   = "✅"
	IconWarn = "⚠️"
	IconFail = "❌"
)

type Printer struct {
	Out   io.Writer
	Err   io.Writer
	JSON  bool
	Quiet bool
}

func NewPrinter(jsonMode bool) *Printer {
	return &Printer{Out: os.Stdout, Err: os.Stderr, JSON: jsonMode}
}

func (p *Printer) Printf(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Out, format, args...)
}

func (p *Printer) Println(args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintln(p.Out, args...)
}

func (p *Printer) PrintJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) Section(title string) {
	p.Printf("\n%s:\n", title)
}

func (p *Printer) Item(icon, msg string) {
	p.Printf("  %s %s\n", icon, msg)
}

func (p *Printer) Lines(lines []string) {
	for _, l := range lines {
		p.Println(l)
	}
}

func JoinCommands(cmds [][]string, binary string) []string {
	out := make([]string, 0, len(cmds))
	for _, args := range cmds {
		parts := append([]string{binary}, args...)
		out = append(out, strings.Join(parts, " "))
	}
	return out
}
