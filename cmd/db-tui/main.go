package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Jason-Wang1245/db-tui/internal/app"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("db-tui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print version information and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: db-tui [--version]")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "db-tui: unexpected arguments: %v\n", flags.Args())
		flags.Usage()
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "db-tui %s (commit %s, built %s)\n", version, commit, date)
		return 0
	}

	program := tea.NewProgram(app.New(app.Dependencies{}))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(stderr, "db-tui: %v\n", err)
		return 1
	}
	return 0
}
