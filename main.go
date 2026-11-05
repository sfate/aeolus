package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/ui"
)

func main() {
	settings, err := loadConfig(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	model := ui.NewModelWithConfig(settings)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running Aeolus: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig validates startup arguments and settings before terminal initialization.
func loadConfig(args []string, output io.Writer) (config.Config, error) {
	flags := flag.NewFlagSet("aeolus", flag.ContinueOnError)
	flags.SetOutput(output)
	path := flags.String("config", "", "path to Aeolus JSON configuration")
	if err := flags.Parse(args); err != nil {
		return config.Config{}, err
	}
	if flags.NArg() != 0 {
		return config.Config{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	return config.Load(*path)
}
