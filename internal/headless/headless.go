// Package headless runs an install from flags alone, for scripts and CI,
// printing progress as plain lines instead of drawing screens.
package headless

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/options"
)

// Run installs with the given answers into the first environment that fits,
// writing every line of output to out. It returns the install's error, or
// one of its own when the answers do not add up.
func Run(opts options.Options, envs []detector.DetectedEnvironment, out io.Writer) error {
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("missing or invalid options:\n%w", err)
	}
	env, err := choose(opts, envs)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opts.Directory, 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", opts.Directory, err)
	}

	cfg := opts.Config()
	env.Detector.PrepareSteps(&cfg)
	steps := env.Detector.Steps()
	fmt.Fprintf(out, "Installing Mage-OS into %s with %s (%d steps)\n", opts.Directory, env.Env.Name, len(steps))

	cfg.Log = func(line string) { fmt.Fprintln(out, line) }
	cfg.OnStepStart = func(i int) { fmt.Fprintf(out, "==> [%d/%d] %s\n", i+1, len(steps), steps[i].Name) }
	cfg.OnStepDone = func(i int) { fmt.Fprintf(out, "<== [%d/%d] %s: done\n", i+1, len(steps), steps[i].Name) }

	if err := env.Detector.Install(&cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nInstalled. Storefront: %s\n", env.Detector.BaseURL(cfg.ProjectName))
	return nil
}

// choose picks the environment the options ask for, or the only one there
// is. With nobody to ask, an ambiguous choice is an error.
func choose(opts options.Options, envs []detector.DetectedEnvironment) (detector.DetectedEnvironment, error) {
	var fitting []detector.DetectedEnvironment
	for _, env := range envs {
		if opts.WantsEnvironment(env.Env.Name) {
			fitting = append(fitting, env)
		}
	}
	switch len(fitting) {
	case 0:
		if opts.Environment != "" {
			return detector.DetectedEnvironment{}, fmt.Errorf("%s is not installed on this machine", opts.Environment)
		}
		return detector.DetectedEnvironment{}, errors.New("no supported environment found: install DDEV or Warden")
	case 1:
		return fitting[0], nil
	default:
		var names []string
		for _, env := range fitting {
			names = append(names, strings.ToLower(env.Env.Name))
		}
		return detector.DetectedEnvironment{}, fmt.Errorf("more than one environment is installed; pick one with --env=%s", strings.Join(names, " or --env="))
	}
}
