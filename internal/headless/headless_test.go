package headless

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/options"
)

// fakeDetector records the config it was asked to install and plays a run.
type fakeDetector struct {
	name       string
	installed  *detector.Config
	installErr error
}

func (d *fakeDetector) Info() detector.DetectorInfo { return detector.DetectorInfo{Name: d.name} }
func (d *fakeDetector) Steps() []detector.Step {
	return []detector.Step{{Name: "Configure"}, {Name: "Install Mage-OS"}}
}
func (d *fakeDetector) PrepareSteps(_ *detector.Config)        {}
func (d *fakeDetector) Detect() (*detector.Environment, error) { return nil, nil }
func (d *fakeDetector) Install(cfg *detector.Config) error {
	d.installed = cfg
	cfg.OnStepStart(0)
	cfg.Log("▸ configuring")
	cfg.OnStepDone(0)
	cfg.OnStepStart(1)
	return d.installErr
}
func (d *fakeDetector) SetupInstallFlags(_ *detector.Config) []detector.SetupFlag { return nil }
func (d *fakeDetector) MagentoCommand() string                                    { return "fake bin/magento" }
func (d *fakeDetector) SetupCommandPrefix() string                                { return "fake bin/magento setup:install" }
func (d *fakeDetector) BaseURL(projectName string) string {
	return "https://" + projectName + ".test"
}

func env(name string, d detector.Detector) detector.DetectedEnvironment {
	return detector.DetectedEnvironment{Env: detector.Environment{Name: name}, Detector: d}
}

func complete(t *testing.T) options.Options {
	t.Helper()
	o := options.Defaults
	o.ProjectName = "shop"
	o.Directory = t.TempDir()
	o.AdminPassword = "Wachtwoord123"
	o.Yes = true
	return o
}

func TestRun_InstallsWithTheGivenAnswersAndPrintsProgress(t *testing.T) {
	d := &fakeDetector{name: "DDEV"}
	var out bytes.Buffer

	err := Run(complete(t), []detector.DetectedEnvironment{env("DDEV", d)}, &out)

	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if d.installed == nil || d.installed.AdminPassword != "Wachtwoord123" || d.installed.ProjectName != "shop" {
		t.Errorf("detector installed %+v, expected the options", d.installed)
	}
	for _, want := range []string{"with DDEV (2 steps)", "==> [1/2] Configure", "▸ configuring", "<== [1/2] Configure: done", "==> [2/2] Install Mage-OS", "Storefront: https://shop.test"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should contain %q, got:\n%s", want, out.String())
		}
	}
}

func TestRun_ReturnsTheInstallError(t *testing.T) {
	d := &fakeDetector{name: "DDEV", installErr: errors.New("setup:install failed")}

	err := Run(complete(t), []detector.DetectedEnvironment{env("DDEV", d)}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "setup:install failed") {
		t.Errorf("Run() = %v, expected the install's error", err)
	}
}

func TestRun_RefusesIncompleteAnswersBeforeTouchingAnything(t *testing.T) {
	d := &fakeDetector{name: "DDEV"}
	o := complete(t)
	o.AdminPassword = "short"

	err := Run(o, []detector.DetectedEnvironment{env("DDEV", d)}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "at least 12 characters") {
		t.Errorf("Run() = %v, expected the password rule", err)
	}
	if d.installed != nil {
		t.Error("nothing should be installed with invalid options")
	}
}

func TestRun_PicksTheEnvironmentAskedFor(t *testing.T) {
	ddev, warden := &fakeDetector{name: "DDEV"}, &fakeDetector{name: "Warden"}
	o := complete(t)
	o.Environment = "warden"

	if err := Run(o, []detector.DetectedEnvironment{env("DDEV", ddev), env("Warden", warden)}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if warden.installed == nil || ddev.installed != nil {
		t.Error("expected Warden to install and DDEV to be left alone")
	}
}

func TestRun_NeedsEnvToBreakATie(t *testing.T) {
	err := Run(complete(t), []detector.DetectedEnvironment{env("DDEV", &fakeDetector{}), env("Warden", &fakeDetector{})}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "--env=ddev or --env=warden") {
		t.Errorf("Run() = %v, expected to be told to pick with --env", err)
	}
}

func TestRun_ExplainsAMissingEnvironment(t *testing.T) {
	o := complete(t)
	o.Environment = "warden"

	err := Run(o, []detector.DetectedEnvironment{env("DDEV", &fakeDetector{})}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "warden is not installed") {
		t.Errorf("Run() = %v, expected to name the missing environment", err)
	}

	err = Run(complete(t), nil, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no supported environment") {
		t.Errorf("Run() = %v, expected the no-environment message", err)
	}
}
