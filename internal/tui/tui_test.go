package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mage-os/mage-os-install/internal/detector"
)

// mockDetector satisfies the detector.Detector interface for tests.
type mockDetector struct {
	info       detector.DetectorInfo
	env        *detector.Environment
	installErr error
}

func (d *mockDetector) Info() detector.DetectorInfo            { return d.info }
func (d *mockDetector) Steps() []detector.Step                 { return nil }
func (d *mockDetector) PrepareSteps(_ *detector.Config)        {}
func (d *mockDetector) Detect() (*detector.Environment, error) { return d.env, nil }
func (d *mockDetector) Install(cfg *detector.Config) error     { return d.installErr }
func (d *mockDetector) SetupInstallFlags(_ *detector.Config) []detector.SetupFlag {
	return nil
}
func (d *mockDetector) SetupCommandPrefix() string  { return "mock exec bin/magento setup:install" }
func (d *mockDetector) BaseURL(projectName string) string {
	return "https://" + projectName + ".test"
}

// sendMsg sends a message into the model and returns the updated model.
// The returned tea.Cmd is discarded — no side-effects run during tests.
func sendMsg(m Model, msg tea.Msg) Model {
	newModel, _ := m.Update(msg)
	return newModel.(Model)
}

func pressEnter(m Model) Model {
	return sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
}

// makeDetectedEnv builds a DetectedEnvironment backed by a mockDetector.
func makeDetectedEnv(name string) detector.DetectedEnvironment {
	return detector.DetectedEnvironment{
		Env:      detector.Environment{Name: name, Version: "1.0.0"},
		Detector: &mockDetector{info: detector.DetectorInfo{Name: name, InstallURL: "https://example.com"}},
	}
}

// --- initial state ---

func TestNew_StartsInNameInputPhase(t *testing.T) {
	m := New()
	if m.phase != phaseNameInput {
		t.Errorf("expected phaseNameInput, got %d", m.phase)
	}
}

func TestNew_NameInputMatchesOriginalName(t *testing.T) {
	m := New()
	if m.nameInput.Value() == "" {
		t.Error("nameInput should have a non-empty default")
	}
	if m.nameInput.Value() != m.originalName {
		t.Errorf("nameInput.Value() %q should equal originalName %q", m.nameInput.Value(), m.originalName)
	}
}

// --- name → directory phase ---

func TestUpdate_EnterOnNameGoesToDirectoryPhase(t *testing.T) {
	m := pressEnter(New())
	if m.phase != phaseDirectoryInput {
		t.Errorf("expected phaseDirectoryInput, got %d", m.phase)
	}
}

func TestUpdate_DirDefault_WhenNameUnchanged_IsCurrentDir(t *testing.T) {
	m := pressEnter(New()) // name → dir
	wd, _ := os.Getwd()
	if m.dirInput.Value() != wd {
		t.Errorf("expected dir %q, got %q", wd, m.dirInput.Value())
	}
}

func TestUpdate_DirDefault_WhenNameChanged_IsCwdPlusName(t *testing.T) {
	m := New()
	m.originalName = "original"
	m.nameInput.SetValue("my-custom-project")
	m = pressEnter(m) // name → dir
	wd, _ := os.Getwd()
	expected := filepath.Join(wd, "my-custom-project")
	if m.dirInput.Value() != expected {
		t.Errorf("expected dir %q, got %q", expected, m.dirInput.Value())
	}
}

// --- detection caching during input phases ---

func TestUpdate_DetectionCachedDuringNameInput(t *testing.T) {
	m := sendMsg(New(), detectionDoneMsg{envs: []detector.DetectedEnvironment{}})
	if m.phase != phaseNameInput {
		t.Errorf("phase should stay phaseNameInput, got %d", m.phase)
	}
	if m.envs == nil {
		t.Error("envs should be cached (non-nil)")
	}
}

func TestUpdate_DetectionCachedDuringDirInput(t *testing.T) {
	m := pressEnter(New()) // go to dir phase
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{}})
	if m.phase != phaseDirectoryInput {
		t.Errorf("phase should stay phaseDirectoryInput, got %d", m.phase)
	}
	if m.envs == nil {
		t.Error("envs should be cached (non-nil)")
	}
}

// --- advancing from directory phase ---

func TestUpdate_EnterOnDir_NoEnvs_GoesToError(t *testing.T) {
	m := pressEnter(New())                                                     // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{}})   // cache: empty
	m = pressEnter(m)                                                          // confirm dir
	if m.phase != phaseError {
		t.Errorf("expected phaseError, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_OneEnv_GoesToSetupConfig(t *testing.T) {
	m := pressEnter(New())                                                                    // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}}) // cache: one
	m = pressEnter(m)                                                                         // confirm dir
	if m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_MultipleEnvs_GoesToSelection(t *testing.T) {
	m := pressEnter(New()) // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{
		makeDetectedEnv("DDEV"),
		makeDetectedEnv("Warden"),
	}})
	m = pressEnter(m) // confirm dir
	if m.phase != phaseSelection {
		t.Errorf("expected phaseSelection, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_StillDetecting_GoesToDetecting(t *testing.T) {
	m := pressEnter(New()) // → dir (envs still nil)
	m = pressEnter(m)      // confirm dir without cached envs
	if m.phase != phaseDetecting {
		t.Errorf("expected phaseDetecting, got %d", m.phase)
	}
}

// --- install done ---

func TestUpdate_InstallDone_GoesToOpenBrowser(t *testing.T) {
	m := sendMsg(New(), installDoneMsg{err: nil})
	if m.phase != phaseOpenBrowser {
		t.Errorf("expected phaseOpenBrowser, got %d", m.phase)
	}
}

func TestUpdate_InstallDone_WithError_GoesToInstallDone(t *testing.T) {
	m := sendMsg(New(), installDoneMsg{err: fmt.Errorf("something failed")})
	if m.phase != phaseInstallDone {
		t.Errorf("expected phaseInstallDone, got %d", m.phase)
	}
}

// --- selection phase cursor ---

func TestUpdate_SelectionCursorMovement(t *testing.T) {
	m := New()
	m.phase = phaseSelection
	m.envs = []detector.DetectedEnvironment{
		makeDetectedEnv("DDEV"),
		makeDetectedEnv("Warden"),
	}
	m.cursor = 0

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m.cursor)
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 after up, got %d", m.cursor)
	}
}

func TestUpdate_SelectionCursor_DoesNotGoOutOfBounds(t *testing.T) {
	m := New()
	m.phase = phaseSelection
	m.envs = []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}
	m.cursor = 0

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0 at end of list, got %d", m.cursor)
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0 at start of list, got %d", m.cursor)
	}
}

// --- view content ---

// TestView_SelectionShowsNameAndVersion verifies that the selection screen
// includes both the environment name and version (acceptance criterion 2).
func TestView_SelectionShowsNameAndVersion(t *testing.T) {
	m := New()
	m.phase = phaseSelection
	m.envs = []detector.DetectedEnvironment{
		{
			Env:      detector.Environment{Name: "DDEV", Version: "1.23.4"},
			Detector: &mockDetector{info: detector.DetectorInfo{Name: "DDEV", InstallURL: "https://ddev.example.com"}},
		},
	}
	view := m.View()
	if !contains(view, "DDEV") {
		t.Error("selection view should contain environment name 'DDEV'")
	}
	if !contains(view, "1.23.4") {
		t.Error("selection view should contain environment version '1.23.4'")
	}
}

// TestView_ErrorShowsInstallLinks verifies that the error phase shows
// links for each known environment (acceptance criterion 5).
func TestView_ErrorShowsInstallLinks(t *testing.T) {
	m := New()
	m.phase = phaseError
	view := m.View()
	for _, info := range detector.AllDetectorInfos() {
		if !contains(view, info.InstallURL) {
			t.Errorf("error view should contain install URL %q for %q", info.InstallURL, info.Name)
		}
	}
}

// TestView_DetectingPhaseShowsSpinner verifies that the detecting phase
// renders spinner output (acceptance criterion 6).
func TestView_DetectingPhaseShowsSpinner(t *testing.T) {
	m := New()
	m.phase = phaseDetecting
	view := m.View()
	if !contains(view, "Detecting") {
		t.Error("detecting phase view should contain 'Detecting'")
	}
}

// --- admin credentials form (US-003) ---

// advanceToSetupConfig drives the model through name → dir → setup config using a mock env.
func advanceToSetupConfig(t *testing.T) Model {
	t.Helper()
	m := pressEnter(New()) // name → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	m = pressEnter(m) // dir → setup config
	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}
	return m
}

// TestSetupConfig_HasAllFiveFields verifies the setup form view renders all five
// required admin credential field labels.
func TestSetupConfig_HasAllFiveFields(t *testing.T) {
	m := advanceToSetupConfig(t)
	view := m.View()
	for _, want := range []string{"Admin user", "Admin password", "Admin email", "Admin firstname", "Admin lastname"} {
		if !contains(view, want) {
			t.Errorf("setup config view missing field label %q", want)
		}
	}
}

// TestSetupConfig_DefaultsPreFilled verifies that all five admin fields have
// sensible (non-empty) default values after initSetupInputs.
func TestSetupConfig_DefaultsPreFilled(t *testing.T) {
	m := advanceToSetupConfig(t)
	for i, f := range setupFieldDefs {
		val := m.setupInputs[i].Value()
		if val == "" {
			t.Errorf("field %q should have a non-empty default, got empty string", f.label)
		}
	}
}

// TestSetupConfig_TabAdvancesField verifies Tab moves focus to the next field.
func TestSetupConfig_TabAdvancesField(t *testing.T) {
	m := advanceToSetupConfig(t)
	// Start at field 0 (admin user)
	if m.setupFocus != 0 {
		t.Fatalf("expected setupFocus 0, got %d", m.setupFocus)
	}
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.inTogglePhase || m.setupFocus != 1 {
		t.Errorf("Tab should move focus to field 1; inTogglePhase=%v setupFocus=%d", m.inTogglePhase, m.setupFocus)
	}
}

// TestSetupConfig_ShiftTabGoesBack verifies Shift+Tab moves focus to the previous field.
func TestSetupConfig_ShiftTabGoesBack(t *testing.T) {
	m := advanceToSetupConfig(t)
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab}) // → field 1
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.inTogglePhase || m.setupFocus != 0 {
		t.Errorf("Shift+Tab should move back to field 0; inTogglePhase=%v setupFocus=%d", m.inTogglePhase, m.setupFocus)
	}
}

// TestSetupConfig_ValidationRejectsEmpty verifies that clearing a field and
// pressing Enter on the last field triggers a validation error.
func TestSetupConfig_ValidationRejectsEmpty(t *testing.T) {
	m := advanceToSetupConfig(t)
	// Clear the admin user field (index 0)
	m.setupInputs[0].SetValue("")
	// Navigate to the last admin field then to the last toggle and submit
	// by pressing Enter on the last field in the form.
	// Simulate being on the last hyva toggle (absPos == totalFields-1) with enter.
	// Easier: set absPos to last and send enter directly via pressing tab many times.
	totalFields := len(m.setupInputs) + 2 // +2 for toggles
	for i := 0; i < totalFields-1; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	// Now at the last field; press Enter → should trigger validation
	m = pressEnter(m)
	if m.setupError == "" {
		t.Error("expected a setupError after submitting with an empty required field, got none")
	}
	if m.phase == phaseSetupPreview {
		t.Error("should not advance to phaseSetupPreview when validation fails")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
