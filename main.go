package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mage-os/mage-os-install/internal/selfupdate"
	"github.com/mage-os/mage-os-install/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	doSelfUpdate := flag.Bool("self-update", false, "check for a newer version and update if available")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if *doSelfUpdate {
		if err := selfupdate.Run(version); err != nil {
			fmt.Fprintf(os.Stderr, "Self-update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	p := tea.NewProgram(tui.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
