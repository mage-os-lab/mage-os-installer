package detector

import (
	"strings"
	"testing"
)

func TestAllDetectorInfos_HasEntries(t *testing.T) {
	if len(AllDetectorInfos()) == 0 {
		t.Error("expected at least one detector to be registered")
	}
}

func TestAllDetectorInfos_AllHaveNames(t *testing.T) {
	for _, info := range AllDetectorInfos() {
		if info.Name == "" {
			t.Error("a registered detector has an empty Name")
		}
	}
}

func TestAllDetectorInfos_AllHaveInstallURLs(t *testing.T) {
	for _, info := range AllDetectorInfos() {
		if info.InstallURL == "" {
			t.Errorf("detector %q has an empty InstallURL", info.Name)
		}
	}
}

func TestAllDetectorInfos_CountMatchesAllDetectors(t *testing.T) {
	if len(AllDetectorInfos()) != len(allDetectors()) {
		t.Errorf("AllDetectorInfos() returned %d entries but allDetectors() has %d",
			len(AllDetectorInfos()), len(allDetectors()))
	}
}

// TestDetectors_DdevAndWardenBothRegistered verifies that both DDEV and Warden
// detectors are checked (acceptance criterion: check ddev and warden on PATH).
func TestDetectors_DdevAndWardenBothRegistered(t *testing.T) {
	infos := AllDetectorInfos()
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = strings.ToLower(info.Name)
	}

	found := func(want string) bool {
		for _, n := range names {
			if strings.Contains(n, want) {
				return true
			}
		}
		return false
	}

	if !found("ddev") {
		t.Error("DDEV detector not registered")
	}
	if !found("warden") {
		t.Error("Warden detector not registered")
	}
}

// TestDetectors_AllHaveVersionInEnvironment verifies that Detect returns an
// Environment with a non-empty Name (acceptance criterion: show name and version).
// This test only runs if the binary is present on PATH.
func TestDetectors_EnvironmentHasName(t *testing.T) {
	for _, d := range allDetectors() {
		env, err := d.Detect()
		if err != nil {
			// Binary not on PATH — skip, this is expected in CI.
			continue
		}
		if env == nil {
			t.Errorf("detector %q returned nil environment without error", d.Info().Name)
			continue
		}
		if env.Name == "" {
			t.Errorf("detector %q returned environment with empty Name", d.Info().Name)
		}
	}
}
