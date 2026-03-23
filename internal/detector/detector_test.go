package detector

import "testing"

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
