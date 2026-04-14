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
	steps      []detector.Step
}

func (d *mockDetector) Info() detector.DetectorInfo            { return d.info }
func (d *mockDetector) Steps() []detector.Step                 { return d.steps }
func (d *mockDetector) PrepareSteps(_ *detector.Config)        {}
func (d *mockDetector) Detect() (*detector.Environment, error) { return d.env, nil }
func (d *mockDetector) Install(cfg *detector.Config) error     { return d.installErr }
func (d *mockDetector) SetupInstallFlags(cfg *detector.Config) []detector.SetupFlag {
	return []detector.SetupFlag{
		{Flag: "--db-host", Value: "db", Editable: false},
		{Flag: "--admin-user", Value: cfg.AdminUser, Editable: true},
		{Flag: "--admin-password", Value: cfg.AdminPassword, Editable: true},
	}
}
func (d *mockDetector) SetupCommandPrefix() string { return "mock exec bin/magento setup:install" }
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

// --- sample data toggle (US-004) ---

// TestSampleData_DefaultIsOff verifies that the sample data toggle is off by default.
func TestSampleData_DefaultIsOff(t *testing.T) {
	m := advanceToSetupConfig(t)
	if m.installSampleData {
		t.Error("installSampleData should be false by default")
	}
}

// TestSampleData_ToggledBySpace verifies that pressing space on the sample data
// toggle switches it on, and pressing space again switches it off.
func TestSampleData_ToggledBySpace(t *testing.T) {
	m := advanceToSetupConfig(t)

	// Tab through all admin fields to reach the sample data toggle
	for i := 0; i < len(setupFieldDefs); i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	// Now at sample data toggle (absPos == len(setupInputs))
	if !m.inTogglePhase || m.toggleFocus != -2 {
		t.Fatalf("expected focus on sample data toggle; inTogglePhase=%v toggleFocus=%d", m.inTogglePhase, m.toggleFocus)
	}

	// Press space: should enable sample data
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.installSampleData {
		t.Error("space should enable installSampleData")
	}

	// Press space again: should disable sample data
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.installSampleData {
		t.Error("second space should disable installSampleData")
	}
}

// TestSampleData_ViewContainsToggle verifies the setup form renders the sample data toggle.
func TestSampleData_ViewContainsToggle(t *testing.T) {
	m := advanceToSetupConfig(t)
	view := m.View()
	if !contains(view, "Install sample data") {
		t.Error("setup config view should contain 'Install sample data' toggle")
	}
}

// TestSampleData_ViewShowsChecked verifies the toggle renders as [x] when enabled.
func TestSampleData_ViewShowsChecked(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.installSampleData = true
	view := m.View()
	if !contains(view, "[x]") {
		t.Error("view should show [x] when installSampleData is true")
	}
}

// --- Hyvä theme toggle (US-005) ---

// navigateToHyvaToggle tabs through all admin fields and the sample data toggle
// to land on the Hyvä toggle.
func navigateToHyvaToggle(m Model) Model {
	// len(setupFieldDefs) tabs → sample data toggle; one more → Hyvä toggle
	for i := 0; i < len(setupFieldDefs)+1; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	return m
}

// TestHyva_DefaultIsOff verifies that the Hyvä toggle is off by default.
func TestHyva_DefaultIsOff(t *testing.T) {
	m := advanceToSetupConfig(t)
	if m.installHyva {
		t.Error("installHyva should be false by default")
	}
}

// TestHyva_ToggledBySpace verifies that pressing space on the Hyvä toggle
// enables it, and pressing space again disables it.
func TestHyva_ToggledBySpace(t *testing.T) {
	m := advanceToSetupConfig(t)
	m = navigateToHyvaToggle(m)

	if !m.inTogglePhase || m.toggleFocus != -1 {
		t.Fatalf("expected focus on Hyvä toggle; inTogglePhase=%v toggleFocus=%d", m.inTogglePhase, m.toggleFocus)
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.installHyva {
		t.Error("space should enable installHyva")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.installHyva {
		t.Error("second space should disable installHyva")
	}
}

// TestHyva_ViewContainsToggle verifies the setup form renders the Hyvä toggle.
func TestHyva_ViewContainsToggle(t *testing.T) {
	m := advanceToSetupConfig(t)
	view := m.View()
	if !contains(view, "Install Hyv") {
		t.Error("setup config view should contain 'Install Hyvä' toggle label")
	}
}

// TestHyva_ViewShowsChecked verifies the toggle renders as [x] when Hyvä is enabled.
func TestHyva_ViewShowsChecked(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.installHyva = true
	view := m.View()
	if !contains(view, "[x]") {
		t.Error("view should show [x] when installHyva is true")
	}
}

// TestHyva_CredentialFieldsHiddenByDefault verifies that Hyvä repo/token fields
// are not shown when the toggle is off (AC2).
func TestHyva_CredentialFieldsHiddenByDefault(t *testing.T) {
	m := advanceToSetupConfig(t)
	view := m.View()
	if contains(view, "Repo URL") {
		t.Error("'Repo URL' field should not appear when installHyva is false")
	}
	if contains(view, "Auth token") {
		t.Error("'Auth token' field should not appear when installHyva is false")
	}
}

// TestHyva_CredentialFieldsAppearsWhenEnabled verifies that enabling the toggle
// shows the Hyvä repo URL and auth token fields (AC2).
func TestHyva_CredentialFieldsAppearsWhenEnabled(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.installHyva = true
	view := m.View()
	if !contains(view, "Repo URL") {
		t.Error("'Repo URL' field should appear when installHyva is true")
	}
	if !contains(view, "Auth token") {
		t.Error("'Auth token' field should appear when installHyva is true")
	}
}

// --- setup:install command preview (US-006) ---

// advanceToSetupPreview drives the model through name → dir → setup config → preview.
func advanceToSetupPreview(t *testing.T) Model {
	t.Helper()
	m := advanceToSetupConfig(t)
	// Navigate to the last field (Hyvä toggle) and press Enter to submit the form.
	totalFields := len(m.setupInputs) + 2 // +2 for sample data + hyvä toggles
	for i := 0; i < totalFields-1; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	m = pressEnter(m) // submit form → phaseSetupPreview
	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d", m.phase)
	}
	return m
}

// TestPreview_ShowsSetupInstallCommand verifies the preview screen renders the
// bin/magento setup:install command prefix.
func TestPreview_ShowsSetupInstallCommand(t *testing.T) {
	m := advanceToSetupPreview(t)
	view := m.View()
	if !contains(view, "setup:install") {
		t.Error("preview view should contain 'setup:install'")
	}
}

// TestPreview_ShowsAllFlags verifies that all flags returned by SetupInstallFlags
// appear in the preview view.
func TestPreview_ShowsAllFlags(t *testing.T) {
	m := advanceToSetupPreview(t)
	view := m.View()
	for _, flag := range m.selected.Detector.SetupInstallFlags(&m.installCfg) {
		if !contains(view, flag.Flag) {
			t.Errorf("preview view should contain flag %q", flag.Flag)
		}
	}
}

// TestPreview_EditableValuesAppear verifies that editable (user-supplied) values
// appear in the preview view.
func TestPreview_EditableValuesAppear(t *testing.T) {
	m := advanceToSetupPreview(t)
	view := m.View()
	// The admin user default is "admin"; it should appear as an editable value.
	if !contains(view, m.installCfg.AdminUser) {
		t.Errorf("preview view should show editable admin user value %q", m.installCfg.AdminUser)
	}
}

// TestPreview_ScrollsWithArrowKeys verifies that up/down arrow keys change previewScroll.
func TestPreview_ScrollsWithArrowKeys(t *testing.T) {
	m := advanceToSetupPreview(t)
	// Set window height so there is room to scroll.
	m.windowHeight = 5

	initial := m.previewScroll

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.previewScroll == initial {
		// It's possible all lines fit in the window; just verify no panic occurred.
		// Skip if the command has fewer lines than the window.
		totalLines := 1 + len(m.selected.Detector.SetupInstallFlags(&m.installCfg))
		maxVisible := m.windowHeight - 10
		if maxVisible < 3 {
			maxVisible = 3
		}
		if totalLines <= maxVisible {
			t.Skip("not enough lines to scroll; skipping")
		}
		t.Errorf("down arrow should increase previewScroll (was %d, still %d)", initial, m.previewScroll)
	}

	// Scroll back up
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.previewScroll != initial {
		t.Errorf("up arrow should decrease previewScroll back to %d, got %d", initial, m.previewScroll)
	}
}

// TestPreview_UpArrowDoesNotGoNegative verifies that previewScroll stays >= 0.
func TestPreview_UpArrowDoesNotGoNegative(t *testing.T) {
	m := advanceToSetupPreview(t)
	m.previewScroll = 0
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.previewScroll < 0 {
		t.Errorf("previewScroll should not go negative, got %d", m.previewScroll)
	}
}

// TestPreview_BackspaceGoesBackToSetupConfig verifies that pressing Backspace
// returns to the admin credentials form.
func TestPreview_BackspaceGoesBackToSetupConfig(t *testing.T) {
	m := advanceToSetupPreview(t)
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.phase != phaseSetupConfig {
		t.Errorf("Backspace should return to phaseSetupConfig, got %d", m.phase)
	}
}

// TestPreview_EnterThenSudoCachedAdvancesToInstall verifies that pressing Enter on
// the preview screen and receiving the sudoCachedMsg transitions to phaseInstalling.
func TestPreview_EnterThenSudoCachedAdvancesToInstall(t *testing.T) {
	m := advanceToSetupPreview(t)
	// Enter schedules sudo -v via tea.ExecProcess (a Cmd, not a phase change).
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	// Simulate sudo completing successfully.
	m = sendMsg(m, sudoCachedMsg{err: nil})
	if m.phase != phaseInstalling {
		t.Errorf("after Enter + sudoCachedMsg, expected phaseInstalling, got %d", m.phase)
	}
}

// --- multi-step installation with live logging (US-007) ---

// makeDetectedEnvWithSteps creates a DetectedEnvironment with a mockDetector
// that returns the given named steps.
func makeDetectedEnvWithSteps(name string, steps []detector.Step) detector.DetectedEnvironment {
	return detector.DetectedEnvironment{
		Env: detector.Environment{Name: name, Version: "1.0.0"},
		Detector: &mockDetector{
			info:  detector.DetectorInfo{Name: name, InstallURL: "https://example.com"},
			steps: steps,
		},
	}
}

// advanceToInstalling drives the model to phaseInstalling using a detector
// with the given named steps.
func advanceToInstalling(t *testing.T, steps []detector.Step) Model {
	t.Helper()
	m := pressEnter(New()) // name → dir
	env := makeDetectedEnvWithSteps("DDEV", steps)
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{env}})
	m = pressEnter(m) // dir → setup config
	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}
	// Navigate to last field and submit form
	totalFields := len(m.setupInputs) + 2 // +2 for sample data + hyvä toggles
	for i := 0; i < totalFields-1; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	m = pressEnter(m) // submit form → preview
	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d", m.phase)
	}
	// Enter triggers initInstallSteps + sudo -v Cmd; sudo cached → phaseInstalling
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = sendMsg(m, sudoCachedMsg{err: nil})
	if m.phase != phaseInstalling {
		t.Fatalf("expected phaseInstalling, got %d", m.phase)
	}
	return m
}

// TestInstall_NamedStepsAppearInView verifies that installation steps loaded
// from the detector appear by name in the installation view (AC1).
func TestInstall_NamedStepsAppearInView(t *testing.T) {
	steps := []detector.Step{
		{Name: "Configure DDEV"},
		{Name: "Install addons"},
		{Name: "Run setup:install"},
	}
	m := advanceToInstalling(t, steps)
	view := m.View()
	for _, step := range steps {
		if !contains(view, step.Name) {
			t.Errorf("installing view should contain step name %q", step.Name)
		}
	}
}

// TestInstall_PendingStepShowsDot verifies pending steps show the '•' indicator (AC2).
func TestInstall_PendingStepShowsDot(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{{name: "Configure DDEV", status: stepPending}}
	view := m.View()
	if !contains(view, "•") {
		t.Error("pending step should show '•' indicator")
	}
	if !contains(view, "Configure DDEV") {
		t.Error("pending step name should appear in view")
	}
}

// TestInstall_RunningStepShowsArrow verifies running steps show the '▸' indicator (AC2).
func TestInstall_RunningStepShowsArrow(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{{name: "Install addons", status: stepRunning}}
	view := m.View()
	if !contains(view, "▸") {
		t.Error("running step should show '▸' indicator")
	}
	if !contains(view, "Install addons") {
		t.Error("running step name should appear in view")
	}
}

// TestInstall_DoneStepShowsCheckmark verifies done steps show the '✓' indicator (AC2).
func TestInstall_DoneStepShowsCheckmark(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{{name: "Configure DDEV", status: stepDone}}
	view := m.View()
	if !contains(view, "✓") {
		t.Error("done step should show '✓' indicator")
	}
	if !contains(view, "Configure DDEV") {
		t.Error("done step name should appear in view")
	}
}

// TestInstall_FailedStepShowsCross verifies failed steps show the '✗' indicator (AC2).
func TestInstall_FailedStepShowsCross(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{{name: "Run setup:install", status: stepFailed}}
	view := m.View()
	if !contains(view, "✗") {
		t.Error("failed step should show '✗' indicator")
	}
	if !contains(view, "Run setup:install") {
		t.Error("failed step name should appear in view")
	}
}

// TestInstall_LogBoxShowsStreamedOutput verifies that log lines from running
// commands appear in the scrolling log box (AC3).
func TestInstall_LogBoxShowsStreamedOutput(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.logLines = []string{"Installing packages...", "Configuring database..."}
	m.windowHeight = 40
	m.windowWidth = 80
	view := m.View()
	if !contains(view, "Installing packages...") {
		t.Error("log box should contain 'Installing packages...'")
	}
	if !contains(view, "Configuring database...") {
		t.Error("log box should contain 'Configuring database...'")
	}
}

// TestInstall_LogMsgAppendsToLogLines verifies that receiving a logMsg appends
// output to the log (AC3).
func TestInstall_LogMsgAppendsToLogLines(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	m = sendMsg(m, logMsg("hello from installer"))
	if len(m.logLines) == 0 {
		t.Fatal("logMsg should append to logLines")
	}
	if m.logLines[0] != "hello from installer" {
		t.Errorf("expected logLines[0]=%q, got %q", "hello from installer", m.logLines[0])
	}
}

// TestInstall_AllStepsRemainVisible verifies that completed, running, and pending
// steps are all rendered while installation is in progress (AC4).
func TestInstall_AllStepsRemainVisible(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{
		{name: "Configure DDEV", status: stepDone},
		{name: "Install addons", status: stepRunning},
		{name: "Run setup:install", status: stepPending},
	}
	view := m.View()
	for _, stepName := range []string{"Configure DDEV", "Install addons", "Run setup:install"} {
		if !contains(view, stepName) {
			t.Errorf("step %q should remain visible during installation", stepName)
		}
	}
}

// TestInstall_FailedStepRemainsVisibleWhileNextRuns verifies that a failed step
// stays visible when a subsequent step begins running (AC4).
func TestInstall_FailedStepRemainsVisibleWhileNextRuns(t *testing.T) {
	m := New()
	m.phase = phaseInstalling
	env := makeDetectedEnv("DDEV")
	m.selected = &env
	m.installSteps = []installStep{
		{name: "Configure DDEV", status: stepFailed},
		{name: "Run setup:install", status: stepRunning},
	}
	view := m.View()
	if !contains(view, "Configure DDEV") {
		t.Error("failed step should remain visible while the next step runs")
	}
	if !contains(view, "Run setup:install") {
		t.Error("running step should be visible")
	}
}

// TestInstall_StepStartMsgSetsRunning verifies stepStartMsg transitions a step
// from pending to running.
func TestInstall_StepStartMsgSetsRunning(t *testing.T) {
	m := New()
	m.installSteps = []installStep{
		{name: "Configure DDEV", status: stepPending},
	}
	m = sendMsg(m, stepStartMsg{index: 0})
	if m.installSteps[0].status != stepRunning {
		t.Errorf("stepStartMsg should set step to stepRunning, got %d", m.installSteps[0].status)
	}
}

// TestInstall_StepDoneMsgSetsDone verifies stepDoneMsg transitions a step
// from running to done.
func TestInstall_StepDoneMsgSetsDone(t *testing.T) {
	m := New()
	m.installSteps = []installStep{
		{name: "Configure DDEV", status: stepRunning},
	}
	m = sendMsg(m, stepDoneMsg{index: 0})
	if m.installSteps[0].status != stepDone {
		t.Errorf("stepDoneMsg should set step to stepDone, got %d", m.installSteps[0].status)
	}
}

// --- resume from failed step (US-008) ---

// TestResume_FailureShowsLast10LogLines verifies that on failure, the error screen
// shows the last 10 log lines (AC1).
func TestResume_FailureShowsLast10LogLines(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("step failed")
	// Add 15 log lines; only the last 10 should be shown.
	for i := 0; i < 15; i++ {
		m.logLines = append(m.logLines, fmt.Sprintf("log-line-%02d", i))
	}
	view := m.View()
	if !contains(view, "Last output:") {
		t.Error("error screen should contain 'Last output:' header")
	}
	// Line 05 is the 10th from the end (index 5 of 15); should appear.
	if !contains(view, "log-line-05") {
		t.Error("error screen should show log-line-05 (10th from end)")
	}
	// Line 14 is the last; should appear.
	if !contains(view, "log-line-14") {
		t.Error("error screen should show log-line-14 (last line)")
	}
	// Line 04 is beyond the 10-line window; should NOT appear.
	if contains(view, "log-line-04") {
		t.Error("error screen should NOT show log-line-04 (beyond last 10 lines)")
	}
}

// TestResume_RetryTransitionsToInstalling verifies that pressing r on the error
// screen retries the installation and transitions to phaseInstalling (AC2).
func TestResume_RetryTransitionsToInstalling(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("step 1 failed")
	env := makeDetectedEnvWithSteps("DDEV", []detector.Step{
		{Name: "Step 0"},
		{Name: "Step 1"},
		{Name: "Step 2"},
	})
	m.selected = &env
	m.installCfg = detector.Config{Directory: t.TempDir()}
	m.installSteps = []installStep{
		{name: "Step 0", status: stepDone},
		{name: "Step 1", status: stepFailed},
		{name: "Step 2", status: stepPending},
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if m.phase != phaseInstalling {
		t.Errorf("pressing r should transition to phaseInstalling, got %d", m.phase)
	}
	if m.installCfg.StartFromStep != 1 {
		t.Errorf("StartFromStep should be 1 (index of failed step), got %d", m.installCfg.StartFromStep)
	}
}

// TestResume_CompletedStepsSkippedOnRetry verifies that already-completed steps
// remain in the done state and StartFromStep is set to the failed step index (AC3).
func TestResume_CompletedStepsSkippedOnRetry(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("step 2 failed")
	env := makeDetectedEnvWithSteps("DDEV", []detector.Step{
		{Name: "Step 0"},
		{Name: "Step 1"},
		{Name: "Step 2"},
		{Name: "Step 3"},
	})
	m.selected = &env
	m.installCfg = detector.Config{Directory: t.TempDir()}
	m.installSteps = []installStep{
		{name: "Step 0", status: stepDone},
		{name: "Step 1", status: stepDone},
		{name: "Step 2", status: stepFailed},
		{name: "Step 3", status: stepPending},
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Completed steps should remain done.
	if m.installSteps[0].status != stepDone {
		t.Errorf("completed step 0 should stay stepDone on retry, got %d", m.installSteps[0].status)
	}
	if m.installSteps[1].status != stepDone {
		t.Errorf("completed step 1 should stay stepDone on retry, got %d", m.installSteps[1].status)
	}
	// Failed step should be reset to pending.
	if m.installSteps[2].status != stepPending {
		t.Errorf("failed step 2 should be reset to stepPending on retry, got %d", m.installSteps[2].status)
	}
	// Pending step should remain pending.
	if m.installSteps[3].status != stepPending {
		t.Errorf("pending step 3 should remain stepPending on retry, got %d", m.installSteps[3].status)
	}
	// StartFromStep must equal the failed step's index so Install() skips done steps.
	if m.installCfg.StartFromStep != 2 {
		t.Errorf("StartFromStep should be 2, got %d", m.installCfg.StartFromStep)
	}
}

// TestResume_EnterExitsInstaller verifies that pressing Enter on the failure
// screen quits the installer (AC4).
func TestResume_EnterExitsInstaller(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("failed")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("pressing Enter on failure screen should return a Cmd (tea.Quit)")
	}
	if msg := cmd(); msg == nil {
		t.Error("the returned Cmd should produce a message (tea.QuitMsg)")
	}
}

// TestResume_QExitsInstaller verifies that pressing q on the failure screen
// quits the installer (AC4).
func TestResume_QExitsInstaller(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("failed")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("pressing q on failure screen should return a Cmd (tea.Quit)")
	}
	if msg := cmd(); msg == nil {
		t.Error("the returned Cmd should produce a message (tea.QuitMsg)")
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
