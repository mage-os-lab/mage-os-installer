package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/hyva"

	"github.com/mage-os/mage-os-install/internal/locale"
	"github.com/mage-os/mage-os-install/internal/magento"
	"github.com/mage-os/mage-os-install/internal/prereq"

	"github.com/mage-os/mage-os-install/internal/resume"

	"github.com/mage-os/mage-os-install/internal/options"
)

// mockDetector satisfies the detector.Detector interface for tests.
type mockDetector struct {
	info          detector.DetectorInfo
	env           *detector.Environment
	installErr    error
	installOutput []string // lines Install logs before returning
	steps         []detector.Step
}

func (d *mockDetector) Info() detector.DetectorInfo            { return d.info }
func (d *mockDetector) Steps() []detector.Step                 { return d.steps }
func (d *mockDetector) PrepareSteps(_ *detector.Config)        {}
func (d *mockDetector) Detect() (*detector.Environment, error) { return d.env, nil }
func (d *mockDetector) Install(cfg *detector.Config) error {
	for _, line := range d.installOutput {
		cfg.Log(line)
	}
	return d.installErr
}
func (d *mockDetector) SetupInstallFlags(cfg *detector.Config) []detector.SetupFlag {
	return []detector.SetupFlag{
		{Flag: detector.BackendFrontnameFlag, Value: "backend"},
		{Flag: "--db-host", Value: "db", Editable: false},
		{Flag: "--admin-user", Value: cfg.AdminUser, Editable: true},
		{Flag: "--admin-password", Value: cfg.AdminPassword, Editable: true},
	}
}
func (d *mockDetector) MagentoCommand() string     { return "mock exec bin/magento" }
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

// awaitInstall lets the install goroutine started by runInstall finish, so a
// test does not return while it is still writing its log into the temp dir.
func awaitInstall(m Model) {
	for msg := range m.logCh {
		if _, done := msg.(installDoneMsg); done {
			return
		}
	}
}

// finishPage presses Enter on the last item of the current screen, which
// checks the screen and moves on to the next one (or to the preview).
func finishPage(m Model) Model {
	_, last := m.pageBounds(m.setupPage)
	m.focusAbsolutePos(last)
	return pressEnter(m)
}

// toOptionsPage walks past the admin and store screens with their current values.
func toOptionsPage(m Model) Model {
	return finishPage(finishPage(m))
}

// toHyvaPage walks past the admin, store and options screens.
func toHyvaPage(m Model) Model {
	return finishPage(toOptionsPage(m))
}

// completeForm walks all four screens with their current values.
func completeForm(m Model) Model {
	return finishPage(toHyvaPage(m))
}

func pressEnter(m Model) Model {
	return sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
}

// confirmDirectory presses Enter on the directory prompt and, since tests run
// in a directory that has content, answers the resulting warning with yes.
func confirmDirectory(m Model) Model {
	m = pressEnter(m)
	if m.phase == phaseDirectoryConfirm {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	}
	return m
}

// passChecks reports every prerequisite as met, so the flow can move past
// the directory prompt the way it does on a healthy machine.
func passChecks(m Model) Model {
	return sendMsg(m, checksDoneMsg{results: []prereq.Result{{Name: "Docker", Status: prereq.Pass}}})
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
	m := passChecks(pressEnter(New()))
	if m.phase != phaseDirectoryInput {
		t.Errorf("expected phaseDirectoryInput, got %d", m.phase)
	}
}

func TestUpdate_DirDefault_WhenNameUnchanged_IsCurrentDir(t *testing.T) {
	m := passChecks(pressEnter(New())) // name → dir
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
	m := passChecks(pressEnter(New())) // go to dir phase
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
	m := passChecks(pressEnter(New()))                                                     // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{}})   // cache: empty
	m = confirmDirectory(m)                                                          // confirm dir
	if m.phase != phaseError {
		t.Errorf("expected phaseError, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_OneEnv_GoesToSetupConfig(t *testing.T) {
	m := passChecks(pressEnter(New()))                                                                    // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}}) // cache: one
	m = confirmDirectory(m)                                                                         // confirm dir
	if m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_MultipleEnvs_GoesToSelection(t *testing.T) {
	m := passChecks(pressEnter(New())) // → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{
		makeDetectedEnv("DDEV"),
		makeDetectedEnv("Warden"),
	}})
	m = confirmDirectory(m) // confirm dir
	if m.phase != phaseSelection {
		t.Errorf("expected phaseSelection, got %d", m.phase)
	}
}

func TestUpdate_EnterOnDir_StillDetecting_GoesToDetecting(t *testing.T) {
	m := passChecks(pressEnter(New())) // → dir (envs still nil)
	m = confirmDirectory(m)            // confirm dir without cached envs
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
	m := passChecks(pressEnter(New())) // name → dir
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	m.dirInput.SetValue(t.TempDir()) // an install started from here writes its log there, not into the package
	m = confirmDirectory(m) // dir → setup config
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
	// Leaving the admin screen is what checks it.
	m = finishPage(m)
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

	m = toOptionsPage(m) // the options screen opens on the sample data toggle
	// Now at sample data toggle (absPos == len(setupInputs))
	if !m.inTogglePhase || m.toggleFocus != sampleDataToggle {
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
	m := toOptionsPage(advanceToSetupConfig(t))
	view := m.View()
	if !contains(view, "Install sample data") {
		t.Error("setup config view should contain 'Install sample data' toggle")
	}
}

// TestSampleData_ViewShowsChecked verifies the toggle renders as [x] when enabled.
func TestSampleData_ViewShowsChecked(t *testing.T) {
	m := toOptionsPage(advanceToSetupConfig(t))
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
	return toHyvaPage(m) // the Hyvä screen opens on its toggle
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

	if !m.inTogglePhase || m.toggleFocus != hyvaToggle {
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
	m := toHyvaPage(advanceToSetupConfig(t))
	view := m.View()
	if !contains(view, "Install Hyv") {
		t.Error("setup config view should contain 'Install Hyvä' toggle label")
	}
}

// TestHyva_ViewShowsChecked verifies the toggle renders as [x] when Hyvä is enabled.
func TestHyva_ViewShowsChecked(t *testing.T) {
	m := toHyvaPage(advanceToSetupConfig(t))
	m.installHyva = true
	view := m.View()
	if !contains(view, "[x]") {
		t.Error("view should show [x] when installHyva is true")
	}
}

// TestHyva_CredentialFieldsHiddenByDefault verifies that Hyvä repo/token fields
// are not shown when the toggle is off (AC2).
func TestHyva_CredentialFieldsHiddenByDefault(t *testing.T) {
	m := toHyvaPage(advanceToSetupConfig(t))
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
	m := toHyvaPage(advanceToSetupConfig(t))
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
	m = completeForm(m)
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
	awaitInstall(m)
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
	m := passChecks(pressEnter(New())) // name → dir
	env := makeDetectedEnvWithSteps("DDEV", steps)
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{env}})
	m.dirInput.SetValue(t.TempDir()) // the install started below must not log into the package directory
	m = confirmDirectory(m) // dir → setup config
	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}
	m = completeForm(m)
	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d", m.phase)
	}
	// Enter triggers initInstallSteps + sudo -v Cmd; sudo cached → phaseInstalling
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = sendMsg(m, sudoCachedMsg{err: nil})
	if m.phase != phaseInstalling {
		t.Fatalf("expected phaseInstalling, got %d", m.phase)
	}
	awaitInstall(m)
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

// TestResume_FailureShowsTheLastLinesOfOutput verifies that on failure, the
// error screen shows the tail of the installer output (AC1).
func TestResume_FailureShowsTheLastLinesOfOutput(t *testing.T) {
	m := New()
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("step failed")
	// Add more lines than fit; only the last maxErrorLines should be shown.
	for i := 0; i < maxErrorLines+3; i++ {
		m.logLines = append(m.logLines, fmt.Sprintf("log-line-%02d", i))
	}
	view := m.View()
	if !contains(view, "What went wrong:") {
		t.Error("error screen should contain the 'What went wrong:' header")
	}
	if !contains(view, "log-line-03") {
		t.Errorf("error screen should show log-line-03 (%d lines from the end)", maxErrorLines)
	}
	if !contains(view, fmt.Sprintf("log-line-%02d", maxErrorLines+2)) {
		t.Error("error screen should show the last line")
	}
	if contains(view, "log-line-02") {
		t.Errorf("error screen should not show log-line-02 (beyond the last %d lines)", maxErrorLines)
	}
}

// TestResume_RetryTransitionsToInstalling verifies that pressing r on the error
// screen retries the installation and transitions to phaseInstalling (AC2).
func TestResume_RetryTransitionsToInstalling(t *testing.T) {
	m := New()
	m.installCfg.Directory = t.TempDir()
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
	<-m.logCh // wait for runInstall goroutine to exit before t.TempDir cleanup

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
	m.installCfg.Directory = t.TempDir()
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
	<-m.logCh // wait for runInstall goroutine to exit before t.TempDir cleanup

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

// --- open browser phase (US-009) ---

// advanceToOpenBrowser drives the model to phaseOpenBrowser (successful install).
func advanceToOpenBrowser(t *testing.T) Model {
	t.Helper()
	m := passChecks(pressEnter(New())) // name → dir
	env := makeDetectedEnv("DDEV")
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{env}})
	m = confirmDirectory(m) // dir → setup config
	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}
	// Set a recognisable project name so we can assert on the URL.
	m.nameInput.SetValue("myproject")
	m.installCfg.ProjectName = "myproject"
	m.phase = phaseOpenBrowser
	m.selected = &env
	m.browserOpened = false
	return m
}

// TestOpenBrowser_ViewContainsPrompt verifies that the open-browser phase shows
// a prompt asking the user to open the store URL (AC1).
func TestOpenBrowser_ViewContainsPrompt(t *testing.T) {
	m := advanceToOpenBrowser(t)
	view := m.View()
	if !contains(view, "in your browser") {
		t.Error("open-browser view should contain 'in your browser' prompt")
	}
}

// TestOpenBrowser_ViewContainsStoreURL verifies that the URL shown in the prompt
// matches the configured base URL for the selected environment (AC4).
func TestOpenBrowser_ViewContainsStoreURL(t *testing.T) {
	m := advanceToOpenBrowser(t)
	expectedURL := m.selected.Detector.BaseURL(m.installCfg.ProjectName)
	view := m.View()
	if !contains(view, expectedURL) {
		t.Errorf("open-browser view should contain URL %q, got view:\n%s", expectedURL, view)
	}
}

// TestOpenBrowser_YOpensURLAndAdvancesToDone verifies that pressing y sets
// browserOpened and transitions to phaseInstallDone (AC2).
func TestOpenBrowser_YOpensURLAndAdvancesToDone(t *testing.T) {
	m := advanceToOpenBrowser(t)
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !m.browserOpened {
		t.Error("pressing y should set browserOpened = true")
	}
	if m.phase != phaseInstallDone {
		t.Errorf("pressing y should advance to phaseInstallDone, got %d", m.phase)
	}
}

// TestOpenBrowser_NSkipsBrowserAndAdvancesToDone verifies that pressing n does
// NOT open the browser and transitions to phaseInstallDone (AC3).
func TestOpenBrowser_NSkipsBrowserAndAdvancesToDone(t *testing.T) {
	m := advanceToOpenBrowser(t)
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.browserOpened {
		t.Error("pressing n should NOT open the browser")
	}
	if m.phase != phaseInstallDone {
		t.Errorf("pressing n should advance to phaseInstallDone, got %d", m.phase)
	}
}

// TestOpenBrowser_QExitsWithoutOpeningBrowser verifies that pressing q exits the
// installer without opening the browser (AC3).
func TestOpenBrowser_QExitsWithoutOpeningBrowser(t *testing.T) {
	m := advanceToOpenBrowser(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("pressing q should return a Cmd (tea.Quit)")
	}
	if msg := cmd(); msg == nil {
		t.Error("the returned Cmd should produce a message (tea.QuitMsg)")
	}
	// browserOpened must remain false since we quit without opening the URL.
	if m.browserOpened {
		t.Error("pressing q should NOT open the browser before quitting")
	}
}

// TestOpenBrowser_DoneViewShowsOpenedURL verifies that after y is pressed, the
// install-done screen mentions the opened URL (AC2 follow-up).
func TestOpenBrowser_DoneViewShowsOpenedURL(t *testing.T) {
	m := advanceToOpenBrowser(t)
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	// Now in phaseInstallDone; the done view should mention the URL that was opened.
	expectedURL := m.selected.Detector.BaseURL(m.installCfg.ProjectName)
	view := m.View()
	if !contains(view, expectedURL) {
		t.Errorf("install-done view should show opened URL %q", expectedURL)
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

// --- admin password validation ---

// submitSetupForm fills in the admin password and submits the setup form.
func submitSetupForm(t *testing.T, password string) Model {
	t.Helper()
	m := advanceToSetupConfig(t)
	m.setupInputs[adminPasswordField].SetValue(password)
	return completeForm(m) // stops on the first screen that does not pass
}

// TestSetupConfig_RejectsAPasswordMageOSWouldReject verifies a password that
// breaks the PCI 4 rules keeps the user on the form with an explanation.
func TestSetupConfig_RejectsAPasswordMageOSWouldReject(t *testing.T) {
	m := submitSetupForm(t, "wachtwoord")

	if m.phase != phaseSetupConfig {
		t.Fatalf("expected to stay in phaseSetupConfig, got %d", m.phase)
	}
	if !contains(m.setupError, "at least 12 characters") {
		t.Errorf("setupError = %q, expected it to explain the length rule", m.setupError)
	}
	if !contains(m.setupError, "numeric and alphabetic") {
		t.Errorf("setupError = %q, expected it to explain the character rule", m.setupError)
	}
}

// TestSetupConfig_FocusesThePasswordFieldAfterRejecting verifies the user can
// fix the password without navigating back to it.
func TestSetupConfig_FocusesThePasswordFieldAfterRejecting(t *testing.T) {
	m := submitSetupForm(t, "wachtwoord")

	if m.setupFocus != adminPasswordField {
		t.Errorf("setupFocus = %d, expected the password field (%d)", m.setupFocus, adminPasswordField)
	}
}

// TestSetupConfig_AcceptsAPasswordThatMeetsTheRules verifies a valid password
// moves on to the command preview.
func TestSetupConfig_AcceptsAPasswordThatMeetsTheRules(t *testing.T) {
	m := submitSetupForm(t, "Wachtwoord123")

	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d (error: %q)", m.phase, m.setupError)
	}
}

// TestSetupConfig_AcceptsAPasswordWithShellCharacters verifies the form does
// not reject characters that raw exec now passes through safely.
func TestSetupConfig_AcceptsAPasswordWithShellCharacters(t *testing.T) {
	m := submitSetupForm(t, "Se$cret`12 34")

	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d (error: %q)", m.phase, m.setupError)
	}
}

// TestSetupConfig_DefaultPasswordMeetsTheRules guards against a default that
// the installer itself would reject.
func TestSetupConfig_DefaultPasswordMeetsTheRules(t *testing.T) {
	if err := magento.ValidateAdminPassword(setupFieldDefaults[adminPasswordField]); err != nil {
		t.Errorf("default admin password is invalid: %v", err)
	}
}

// TestSetupConfig_ShowsThePasswordRules verifies the rules are on screen
// before the user types a password.
func TestSetupConfig_ShowsThePasswordRules(t *testing.T) {
	view := advanceToSetupConfig(t).View()

	if !contains(view, magento.AdminPasswordHint()) {
		t.Error("setup form should show the admin password rules")
	}
}

// --- sudo prompt warning ---

// TestPreview_WarnsAboutTheSudoPrompt verifies the password prompt that
// follows Enter is announced while the user can still read the screen.
func TestPreview_WarnsAboutTheSudoPrompt(t *testing.T) {
	view := advanceToSetupPreview(t).View()

	for _, want := range []string{"computer login password", "sudo", "/etc/hosts"} {
		if !contains(view, want) {
			t.Errorf("preview should explain the sudo prompt, missing %q", want)
		}
	}
}

// TestPreview_SeparatesTheSudoPromptFromTheAdminPassword verifies the warning
// says which password is being asked for.
func TestPreview_SeparatesTheSudoPromptFromTheAdminPassword(t *testing.T) {
	view := advanceToSetupPreview(t).View()

	if !contains(view, "not the Mage-OS admin password") {
		t.Error("preview should say the prompt is not for the Mage-OS admin password")
	}
}

// TestPreview_NamesTheEnvironmentThatNeedsSudo verifies the warning points at
// the environment doing the asking.
func TestPreview_NamesTheEnvironmentThatNeedsSudo(t *testing.T) {
	lines := sudoWarningLines("Warden")

	if !contains(strings.Join(lines, " "), "Warden needs sudo") {
		t.Errorf("got %q, expected the environment name in the warning", lines)
	}
}

// TestSudoRefreshCommand_ExplainsItselfAtThePrompt verifies sudo asks with a
// reason instead of a bare "Password:".
func TestSudoRefreshCommand_ExplainsItselfAtThePrompt(t *testing.T) {
	args := sudoRefreshCommand().Args

	if strings.Join(args[:3], " ") != "sudo -v -p" {
		t.Fatalf("got %q, expected sudo -v with a prompt", args)
	}
	if !contains(args[3], "/etc/hosts") {
		t.Errorf("prompt was %q, expected it to say what sudo is for", args[3])
	}
}

// --- git repository toggle ---

// navigateToGitToggle tabs from the first admin field to the Git toggle.
func navigateToGitToggle(m Model) Model {
	return sendMsg(toOptionsPage(m), tea.KeyMsg{Type: tea.KeyTab})
}

// TestGit_DefaultIsOn verifies a fresh project gets a repository unless the
// user says otherwise, which is what issue #1 asks for.
func TestGit_DefaultIsOn(t *testing.T) {
	if !advanceToSetupConfig(t).initGit {
		t.Error("initGit should be true by default")
	}
}

// TestGit_ToggledBySpace verifies the user can decline a repository.
func TestGit_ToggledBySpace(t *testing.T) {
	m := navigateToGitToggle(advanceToSetupConfig(t))

	if !m.inTogglePhase || m.toggleFocus != initGitToggle {
		t.Fatalf("expected focus on the Git toggle; inTogglePhase=%v toggleFocus=%d", m.inTogglePhase, m.toggleFocus)
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.initGit {
		t.Error("space should disable initGit")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.initGit {
		t.Error("space should enable initGit again")
	}
}

// TestGit_TogglingDoesNotTouchTheOtherOptions verifies the toggles stay apart.
func TestGit_TogglingDoesNotTouchTheOtherOptions(t *testing.T) {
	m := navigateToGitToggle(advanceToSetupConfig(t))
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	if m.installSampleData || m.installHyva {
		t.Errorf("only the Git toggle should change; sampleData=%v hyva=%v", m.installSampleData, m.installHyva)
	}
}

// TestSetupConfig_ExplainsTheGitOption verifies the form says what it does.
func TestSetupConfig_ExplainsTheGitOption(t *testing.T) {
	view := toOptionsPage(advanceToSetupConfig(t)).View()

	for _, want := range []string{"Initialize Git", "git init"} {
		if !contains(view, want) {
			t.Errorf("setup form should explain the Git option, missing %q", want)
		}
	}
}

// TestGit_ChoiceReachesTheInstaller verifies the toggle ends up in the config
// the detector is handed.
func TestGit_ChoiceReachesTheInstaller(t *testing.T) {
	m := submitSetupForm(t, "Wachtwoord123")
	if !m.installCfg.InitGit {
		t.Error("InitGit should be carried into the install config")
	}

	m = navigateToGitToggle(advanceToSetupConfig(t))
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	for i := 0; i < toggleCount-1; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}
	m = pressEnter(m)
	if m.installCfg.InitGit {
		t.Error("declining Git should be carried into the install config")
	}
}

// --- post-install summary ---

// installedModel drives the model to a finished install, so the success
// screens have a selected environment and an install config to summarise.
func installedModel(t *testing.T, phase phase) Model {
	t.Helper()
	m := advanceToSetupPreview(t)
	m.phase = phase
	return m
}

// TestSuccess_ShowsWhereTheStoreIs verifies the storefront and admin URLs are
// on screen, with the admin URL built from the environment's front name.
func TestSuccess_ShowsWhereTheStoreIs(t *testing.T) {
	m := installedModel(t, phaseOpenBrowser)
	view := m.View()

	storefront := "https://" + m.installCfg.ProjectName + ".test"
	for _, want := range []string{"Storefront", storefront, "Admin", storefront + "/backend"} {
		if !contains(view, want) {
			t.Errorf("success screen should contain %q", want)
		}
	}
}

// --- elapsed time on the install screen ---

// frozenClock makes now() return a fixed time, advanced by the test.
func frozenClock(t *testing.T, start time.Time) func(time.Duration) {
	t.Helper()
	current := start
	now = func() time.Time { return current }
	t.Cleanup(func() { now = time.Now })
	return func(d time.Duration) { current = current.Add(d) }
}

// installingModel is a model mid-install with three steps.
func installingModel(t *testing.T) Model {
	t.Helper()
	m := advanceToSetupPreview(t)
	m.installSteps = []installStep{{name: "Configure DDEV"}, {name: "Create Mage-OS project"}, {name: "Verify installation"}}
	m.phase = phaseInstalling
	m.installStart = now()
	return m
}

func TestFormatElapsed_ReadsLikeAStopwatch(t *testing.T) {
	cases := map[time.Duration]string{
		0:                                "0s",
		42 * time.Second:                 "42s",
		time.Minute + 5*time.Second:      "1m05s",
		12*time.Minute + 30*time.Second:  "12m30s",
		time.Hour + 2*time.Minute:        "1h02m",
		2*time.Hour + 45*time.Minute + 1: "2h45m",
	}
	for d, want := range cases {
		if got := formatElapsed(d); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

// --- non-empty install directory ---

// TestDirectory_WarnsWhenItAlreadyHasContent verifies Enter on a directory
// with files in it asks before going on, and says what is in there.
func TestDirectory_WarnsWhenItAlreadyHasContent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app", "vendor", "pub", "composer.json"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	m := passChecks(pressEnter(New()))
	m.dirInput.SetValue(dir)

	m = pressEnter(m)

	if m.phase != phaseDirectoryConfirm {
		t.Fatalf("expected phaseDirectoryConfirm, got %d", m.phase)
	}
	for _, want := range []string{"not empty", "4 entries", "app/", "composer.json", "Install here anyway?"} {
		if !contains(m.View(), want) {
			t.Errorf("warning should contain %q", want)
		}
	}
}

// TestSuccess_HidesThePasswordTheUserKeptHidden verifies the success screen
// respects the masked password field.
func TestSuccess_HidesThePasswordTheUserKeptHidden(t *testing.T) {
	m := installedModel(t, phaseOpenBrowser)
	view := m.View()

	if want := m.installCfg.AdminUser + " / " + maskedPassword; !contains(view, want) {
		t.Errorf("success screen should contain the masked login %q", want)
	}
	if contains(view, m.installCfg.AdminPassword) {
		t.Errorf("success screen should not show the hidden password %q", m.installCfg.AdminPassword)
	}
}

// TestSuccess_ShowsThePasswordTheUserRevealed verifies the clear text shows
// when the user chose to see it on the form.
func TestSuccess_ShowsThePasswordTheUserRevealed(t *testing.T) {
	m := installedModel(t, phaseOpenBrowser)
	m.passwordRevealed = true
	view := m.View()

	if want := m.installCfg.AdminUser + " / " + m.installCfg.AdminPassword; !contains(view, want) {
		t.Errorf("success screen should contain the login %q", want)
	}
}

// TestSuccess_TellsHowToRunMagentoCommands verifies the environment's command
// prefix and the project directory are on screen.
func TestSuccess_TellsHowToRunMagentoCommands(t *testing.T) {
	m := installedModel(t, phaseOpenBrowser)
	view := m.View()

	for _, want := range []string{"mock exec bin/magento <command>", m.installCfg.Directory} {
		if !contains(view, want) {
			t.Errorf("success screen should contain %q", want)
		}
	}
}

// TestDirectory_YesInstallsThere verifies confirming continues to the setup
// form as before.
func TestDirectory_YesInstallsThere(t *testing.T) {
	m := passChecks(pressEnter(New()))
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	m = pressEnter(m) // this test's working directory has content
	if m.phase != phaseDirectoryConfirm {
		t.Fatalf("expected phaseDirectoryConfirm, got %d", m.phase)
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig after confirming, got %d", m.phase)
	}
}

// TestDirectory_NoGoesBackToTheInput verifies declining returns to the prompt
// with the typed path intact.
func TestDirectory_NoGoesBackToTheInput(t *testing.T) {
	m := passChecks(pressEnter(New()))
	m = pressEnter(m)
	if m.phase != phaseDirectoryConfirm {
		t.Fatalf("expected phaseDirectoryConfirm, got %d", m.phase)
	}
	before := m.dirInput.Value()

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if m.phase != phaseDirectoryInput {
		t.Errorf("expected phaseDirectoryInput after declining, got %d", m.phase)
	}
	if m.dirInput.Value() != before {
		t.Errorf("directory input changed from %q to %q", before, m.dirInput.Value())
	}
}

// TestDirectory_EmptyOrMissingNeedsNoConfirmation verifies the common case, a
// fresh directory, is not slowed down by a question.
func TestDirectory_EmptyOrMissingNeedsNoConfirmation(t *testing.T) {
	for name, dir := range map[string]string{"empty": t.TempDir(), "missing": filepath.Join(t.TempDir(), "new-shop")} {
		m := passChecks(pressEnter(New()))
		m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
		m.dirInput.SetValue(dir)

		m = pressEnter(m)

		if m.phase != phaseSetupConfig {
			t.Errorf("[%s] expected phaseSetupConfig without a warning, got %d", name, m.phase)
		}
	}
}

// TestSuccess_SummaryStaysAfterTheBrowserQuestion verifies the final screen
// repeats the summary, so it is still there once the browser prompt is gone.
func TestSuccess_SummaryStaysAfterTheBrowserQuestion(t *testing.T) {
	m := installedModel(t, phaseInstallDone)
	m.browserOpened = true
	view := m.View()

	adminURL := "https://" + m.installCfg.ProjectName + ".test/backend"
	for _, want := range []string{adminURL, "Opened the storefront", "Press enter to exit"} {
		if !contains(view, want) {
			t.Errorf("final screen should contain %q", want)
		}
	}
}

// TestAdminURL_DoesNotDoubleTheSlash verifies a base URL with a trailing slash,
// as Warden reports it, still yields a clean admin URL.
func TestAdminURL_DoesNotDoubleTheSlash(t *testing.T) {
	m := installedModel(t, phaseOpenBrowser)

	if got := m.adminURL("https://app.shop.test/"); got != "https://app.shop.test/backend" {
		t.Errorf("adminURL() = %q, expected %q", got, "https://app.shop.test/backend")
	}
}

// --- password hint and reveal ---

// TestPassword_HintNamesTheDefaultWhileItIsUntouched verifies a masked default
// is not a mystery: the hint prints it until the user types something else.
func TestPassword_HintNamesTheDefaultWhileItIsUntouched(t *testing.T) {
	m := advanceToSetupConfig(t)

	if view := m.View(); !contains(view, "default: "+setupFieldDefaults[adminPasswordField]) {
		t.Error("setup form should name the default password while it is still in the field")
	}

	m.setupInputs[adminPasswordField].SetValue("Wachtwoord123")
	if view := m.View(); contains(view, "default: ") {
		t.Error("setup form should stop naming the default once the password was changed")
	}
}

// TestPassword_IsMaskedUntilRevealed verifies ctrl+r shows the clear text and
// pressing it again masks it.
func TestPassword_IsMaskedUntilRevealed(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[adminPasswordField].SetValue("Wachtwoord123")

	if contains(m.View(), "Wachtwoord123") {
		t.Fatal("password should be masked by default")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !contains(m.View(), "Wachtwoord123") {
		t.Error("ctrl+r should reveal the password")
	}
	if !contains(m.View(), revealPasswordKey+" to hide") {
		t.Error("hint should offer to hide the password again")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if contains(m.View(), "Wachtwoord123") {
		t.Error("ctrl+r a second time should mask the password again")
	}
}

// TestPassword_RevealWorksFromAnyField verifies the toggle is not tied to the
// password field having focus.
func TestPassword_RevealWorksFromAnyField(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[adminPasswordField].SetValue("Wachtwoord123")
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyCtrlR})

	if !contains(m.View(), "Wachtwoord123") {
		t.Error("ctrl+r should reveal the password from another field too")
	}
}

// TestPassword_HintStillStatesTheRules verifies the rules line survived the
// added hints.
func TestPassword_HintStillStatesTheRules(t *testing.T) {
	if view := advanceToSetupConfig(t).View(); !contains(view, magento.AdminPasswordHint()) {
		t.Error("setup form should still state the password rules")
	}
}

// --- step list on the preview screen ---

// previewWithSteps gives the mock detector a step list, since its PrepareSteps
// is a no-op.
func previewWithSteps(t *testing.T, steps []detector.Step) Model {
	t.Helper()
	m := advanceToSetupPreview(t)
	m.selected.Detector.(*mockDetector).steps = steps
	return m
}

// TestPreview_ListsEveryStepBeforeTheCommand verifies the user sees what the
// install is going to do, not only the setup:install flags.
func TestPreview_ListsEveryStepBeforeTheCommand(t *testing.T) {
	m := previewWithSteps(t, []detector.Step{{Name: "Configure things"}, {Name: "Install Mage-OS"}})
	view := m.View()

	for _, want := range []string{"1. Configure things", "2. Install Mage-OS", "Then run:", "setup:install"} {
		if !contains(view, want) {
			t.Errorf("preview should contain %q", want)
		}
	}
	if strings.Index(view, "Install Mage-OS") > strings.Index(view, "setup:install") {
		t.Error("steps should be listed before the command")
	}
}

// TestPreview_ShowsDurationHintsForSlowSteps verifies a step's estimate is
// printed next to it.
func TestPreview_ShowsDurationHintsForSlowSteps(t *testing.T) {
	m := previewWithSteps(t, []detector.Step{{Name: "Create Mage-OS project", Estimate: detector.EstimateComposer}})

	if !contains(m.View(), detector.EstimateComposer) {
		t.Errorf("preview should contain the estimate %q", detector.EstimateComposer)
	}
}

// TestPreview_ScrollCoversTheStepList verifies scrolling reaches the end of the
// command even now that the step list sits above it.
func TestPreview_ScrollCoversTheStepList(t *testing.T) {
	var steps []detector.Step
	for i := 0; i < 20; i++ {
		steps = append(steps, detector.Step{Name: fmt.Sprintf("Step %02d", i)})
	}
	m := previewWithSteps(t, steps)
	m.windowHeight = 24

	for i := 0; i < 100; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	}

	if want := len(m.previewLines()) - m.previewMaxVisible(); m.previewScroll != want {
		t.Errorf("previewScroll = %d, expected to stop at %d", m.previewScroll, want)
	}
	if !contains(m.View(), "--admin-password") {
		t.Error("scrolling to the end should reveal the last flag")
	}
}

// TestExistingContents_NamesAFewEntries verifies the description stays short
// however full the directory is.
func TestExistingContents_NamesAFewEntries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	got := existingContents(dir)

	if got != "5 entries, including a, b, c" {
		t.Errorf("existingContents() = %q", got)
	}
	if existingContents(filepath.Join(dir, "nope")) != "" {
		t.Error("a missing directory should describe as empty")
	}
}

// TestInstall_RunningStepShowsHowLongItHasTaken verifies the timer on the
// current step moves with the clock.
func TestInstall_RunningStepShowsHowLongItHasTaken(t *testing.T) {
	advance := frozenClock(t, time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	m := installingModel(t)
	m = sendMsg(m, stepStartMsg{index: 1})

	advance(3*time.Minute + 7*time.Second)

	if !contains(m.View(), "▸ Create Mage-OS project...  3m07s") {
		t.Errorf("running step should show its elapsed time, view was:\n%s", m.View())
	}
}

// TestInstall_FinishedStepKeepsItsDuration verifies a completed step shows how
// long it took and stops counting.
func TestInstall_FinishedStepKeepsItsDuration(t *testing.T) {
	advance := frozenClock(t, time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	m := installingModel(t)
	m = sendMsg(m, stepStartMsg{index: 0})
	advance(45 * time.Second)
	m = sendMsg(m, stepDoneMsg{index: 0})
	advance(10 * time.Minute)

	if !contains(m.View(), "✓ Configure DDEV  45s") {
		t.Errorf("finished step should keep its duration, view was:\n%s", m.View())
	}
}

// TestInstall_HeaderShowsTotalElapsed verifies the overall counter.
func TestInstall_HeaderShowsTotalElapsed(t *testing.T) {
	advance := frozenClock(t, time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	m := installingModel(t)

	advance(14*time.Minute + 2*time.Second)

	if !contains(m.View(), "14m02s elapsed") {
		t.Errorf("header should show total elapsed time, view was:\n%s", m.View())
	}
}

// TestInstall_FailedStepRecordsWhenItStopped verifies a failure freezes the
// step's clock at the moment it failed.
func TestInstall_FailedStepRecordsWhenItStopped(t *testing.T) {
	advance := frozenClock(t, time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	m := installingModel(t)
	m = sendMsg(m, stepStartMsg{index: 1})
	advance(2 * time.Minute)
	m = sendMsg(m, installDoneMsg{err: fmt.Errorf("boom")})

	if got := m.installSteps[1].elapsed(); got != 2*time.Minute {
		t.Errorf("failed step elapsed = %v, want 2m", got)
	}
}

// TestInstall_RetryRestartsTheClockOfTheRetriedStep verifies a step run again
// after a failure does not carry its earlier time along.
func TestInstall_RetryRestartsTheClockOfTheRetriedStep(t *testing.T) {
	advance := frozenClock(t, time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC))
	m := installingModel(t)
	m = sendMsg(m, stepStartMsg{index: 1})
	advance(5 * time.Minute)
	m = sendMsg(m, installDoneMsg{err: fmt.Errorf("boom")})
	advance(time.Minute)

	m = sendMsg(m, stepStartMsg{index: 1})
	advance(10 * time.Second)

	if got := m.installSteps[1].elapsed(); got != 10*time.Second {
		t.Errorf("retried step elapsed = %v, want 10s", got)
	}
}

// --- the install log file and the log view ---

// drainInstall runs runInstall against d and collects every message until
// the install reports done.
func drainInstall(t *testing.T, d detector.Detector, cfg detector.Config) []tea.Msg {
	t.Helper()
	ch, _ := runInstall(d, cfg)
	var msgs []tea.Msg
	for msg := range ch {
		msgs = append(msgs, msg)
		if _, done := msg.(installDoneMsg); done {
			return msgs
		}
	}
	return msgs
}

// TestInstallLog_WritesEveryLineToTheProjectDirectory verifies the file holds
// the output, a run header, and the outcome.
func TestInstallLog_WritesEveryLineToTheProjectDirectory(t *testing.T) {
	dir := t.TempDir()
	d := &mockDetector{installOutput: []string{"▸ ddev start", "\x1b[31mSomething red\x1b[0m"}, installErr: fmt.Errorf("boom")}

	drainInstall(t, d, detector.Config{Directory: dir})

	content, err := os.ReadFile(installLogPath(dir))
	if err != nil {
		t.Fatalf("expected a log file: %v", err)
	}
	for _, want := range []string{"=== mage-os-install run started", "▸ ddev start", "Something red", "=== failed: boom"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("log file should contain %q, got:\n%s", want, content)
		}
	}
	if strings.Contains(string(content), "\x1b[") {
		t.Error("log file should not contain colour codes")
	}
}

// TestInstallLog_AppendsAcrossRuns verifies a retry lands in the same file
// under its own header rather than replacing the first run.
func TestInstallLog_AppendsAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	drainInstall(t, &mockDetector{installOutput: []string{"first run"}}, detector.Config{Directory: dir})
	drainInstall(t, &mockDetector{installOutput: []string{"second run"}}, detector.Config{Directory: dir})

	content, _ := os.ReadFile(installLogPath(dir))

	if strings.Count(string(content), "=== mage-os-install run started") != 2 {
		t.Errorf("expected two run headers, got:\n%s", content)
	}
	for _, want := range []string{"first run", "second run", "=== installed"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("log file should contain %q", want)
		}
	}
}

// TestInstallLog_UnwritableDirectoryIsReportedNotFatal verifies a log file
// that cannot be opened is mentioned in the output and the install still runs.
func TestInstallLog_UnwritableDirectoryIsReportedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	msgs := drainInstall(t, &mockDetector{installOutput: []string{"still running"}}, detector.Config{Directory: dir})

	var joined []string
	for _, msg := range msgs {
		if line, ok := msg.(logMsg); ok {
			joined = append(joined, string(line))
		}
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "Could not write "+installLogPath(dir)) {
		t.Errorf("output should mention the log file could not be written, got:\n%s", all)
	}
	if !strings.Contains(all, "still running") {
		t.Error("the install should have carried on")
	}
	if _, done := msgs[len(msgs)-1].(installDoneMsg); !done {
		t.Error("the install should have finished")
	}
}

// failedModel is a model on the failure screen with a known log.
func failedModel(t *testing.T, lines int) Model {
	t.Helper()
	m := advanceToSetupPreview(t)
	m.phase = phaseInstallDone
	m.installErr = fmt.Errorf("step failed")
	for i := 0; i < lines; i++ {
		m.logLines = append(m.logLines, fmt.Sprintf("log-line-%03d", i))
	}
	return m
}

// TestFailure_PointsAtTheLogFile verifies the failure screen names the file and
// the key that opens the full log.
func TestFailure_PointsAtTheLogFile(t *testing.T) {
	m := failedModel(t, 3)
	view := m.View()

	for _, want := range []string{"Full log: " + installLogPath(m.installCfg.Directory), viewLogKey + " to view the full log"} {
		if !contains(view, want) {
			t.Errorf("failure screen should contain %q", want)
		}
	}
}

// TestLogView_OpensAtTheEndAndShowsEveryLine verifies l shows the log,
// scrolled to the most recent output.
func TestLogView_OpensAtTheEndAndShowsEveryLine(t *testing.T) {
	m := failedModel(t, 50)
	m.windowHeight = 30

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(viewLogKey)})

	if m.phase != phaseLogView {
		t.Fatalf("expected phaseLogView, got %d", m.phase)
	}
	view := m.View()
	if !contains(view, "log-line-049") {
		t.Error("log view should open showing the last line")
	}
	if contains(view, "log-line-000") {
		t.Error("log view should not show the first line while scrolled to the end")
	}
	if !contains(view, "of 50") {
		t.Error("log view should say how many lines there are")
	}
}

// TestLogView_ScrollsAndGoesBack verifies the navigation keys and the way out.
func TestLogView_ScrollsAndGoesBack(t *testing.T) {
	m := failedModel(t, 50)
	m.windowHeight = 30
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(viewLogKey)})

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !contains(m.View(), "log-line-000") || m.logScroll != 0 {
		t.Error("g should jump to the top")
	}
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.logScroll != m.logViewRows() {
		t.Errorf("PgDn should move a page, scroll is %d", m.logScroll)
	}
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.logScroll != m.logViewBottom() {
		t.Error("G should jump to the bottom")
	}
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.logScroll != m.logViewBottom() {
		t.Error("scrolling past the end should stay at the bottom")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.phase != phaseInstallDone {
		t.Errorf("esc should return to the failure screen, got phase %d", m.phase)
	}
}

// TestLogView_WrapsLongLinesInsteadOfCuttingThem verifies a long line stays
// readable in the viewer.
func TestLogView_WrapsLongLinesInsteadOfCuttingThem(t *testing.T) {
	m := failedModel(t, 0)
	m.windowWidth = 40
	m.logLines = []string{strings.Repeat("word ", 30) + "END"}

	lines := m.logViewLines()

	if len(lines) < 2 {
		t.Fatalf("expected the line to wrap, got %q", lines)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "END") {
		t.Error("wrapping should keep the end of the line")
	}
}

// --- prerequisite checks ---

// atDirectoryWithEnv is a model on the directory prompt with an environment
// already detected, and the checks still running.
func atDirectoryWithEnv() Model {
	m := pressEnter(New())
	return sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
}

// TestChecks_DirectoryWaitsForThemThenContinues verifies the flow pauses on
// the checks when they are slower than the user, and goes on once they pass.
func TestChecks_DirectoryWaitsForThemThenContinues(t *testing.T) {
	m := confirmDirectory(atDirectoryWithEnv())
	if m.phase != phaseChecking {
		t.Fatalf("expected phaseChecking while checks run, got %d", m.phase)
	}
	if !contains(m.View(), "Checking prerequisites") {
		t.Error("checking screen should say what it is waiting for")
	}

	m = passChecks(m)

	if m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig once the checks pass, got %d", m.phase)
	}
}

// TestChecks_DetectionFinishingEarlyDoesNotSkipThem verifies detection results
// arriving during the checks are kept for later rather than jumping ahead.
func TestChecks_DetectionFinishingEarlyDoesNotSkipThem(t *testing.T) {
	m := confirmDirectory(pressEnter(New()))
	if m.phase != phaseChecking {
		t.Fatalf("expected phaseChecking, got %d", m.phase)
	}

	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	if m.phase != phaseChecking {
		t.Fatalf("detection should not leave the checking phase, got %d", m.phase)
	}

	m = passChecks(m)
	if m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig after checks with envs cached, got %d", m.phase)
	}
}

// TestChecks_FailureBlocksTheInstall verifies a failed check stops the flow,
// says what is wrong, and does not let Enter through.
func TestChecks_FailureBlocksTheInstall(t *testing.T) {
	m := confirmDirectory(atDirectoryWithEnv())
	m = sendMsg(m, checksDoneMsg{results: []prereq.Result{
		{Name: "Docker", Status: prereq.Fail, Detail: "the Docker daemon is not running"},
		{Name: "git", Status: prereq.Pass, Detail: "found"},
	}})

	if m.phase != phaseChecksReview {
		t.Fatalf("expected phaseChecksReview, got %d", m.phase)
	}
	view := m.View()
	for _, want := range []string{"Some prerequisites are missing", "✗ Docker", "the Docker daemon is not running", "✓ git", "press r to check again"} {
		if !contains(view, want) {
			t.Errorf("review screen should contain %q", want)
		}
	}

	if m = pressEnter(m); m.phase != phaseChecksReview {
		t.Errorf("Enter should not get past a failed check, got phase %d", m.phase)
	}
}

// TestChecks_RetryRunsThemAgain verifies r re-runs the checks.
func TestChecks_RetryRunsThemAgain(t *testing.T) {
	m := confirmDirectory(atDirectoryWithEnv())
	m = sendMsg(m, checksDoneMsg{results: []prereq.Result{{Name: "Docker", Status: prereq.Fail}}})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)

	if m.phase != phaseChecking || m.checksDone {
		t.Errorf("r should go back to checking, got phase %d checksDone=%v", m.phase, m.checksDone)
	}
	if cmd == nil {
		t.Error("r should start the checks again")
	}
}

// TestChecks_WarningAsksToBeReadThenContinues verifies a warning shows the
// review screen once and lets Enter through.
func TestChecks_WarningAsksToBeReadThenContinues(t *testing.T) {
	m := confirmDirectory(atDirectoryWithEnv())
	m = sendMsg(m, checksDoneMsg{results: []prereq.Result{
		{Name: "git", Status: prereq.Warn, Detail: "git is not installed, the Git option will be skipped"},
	}})

	if m.phase != phaseChecksReview {
		t.Fatalf("expected phaseChecksReview for a warning, got %d", m.phase)
	}
	for _, want := range []string{"Before we start", "⚠ git", "Enter to continue"} {
		if !contains(m.View(), want) {
			t.Errorf("review screen should contain %q", want)
		}
	}

	if m = pressEnter(m); m.phase != phaseSetupConfig {
		t.Errorf("Enter should continue past a warning, got phase %d", m.phase)
	}
}

// TestChecks_AllPassingNeverShowsAScreen verifies the healthy path adds no
// keypress.
func TestChecks_AllPassingNeverShowsAScreen(t *testing.T) {
	m := passChecks(atDirectoryWithEnv())

	if m = confirmDirectory(m); m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig straight away, got %d", m.phase)
	}
}

// --- Hyvä credentials are checked on the form ---

// submitWithHyva fills in Hyvä credentials and submits the form.
func submitWithHyva(t *testing.T, repoURL, token string) (Model, tea.Cmd) {
	t.Helper()
	m := advanceToSetupConfig(t)
	m.installHyva = true
	m.hyvaInputs[hyvaRepoURLField].SetValue(repoURL)
	m.hyvaInputs[hyvaAuthTokenField].SetValue(token)
	m = toHyvaPage(m)
	_, last := m.pageBounds(pageHyva)
	m.focusAbsolutePos(last)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model), cmd
}

// TestHyva_ABadURLFailsOnTheFormWithoutANetworkCall verifies the URL shape is
// checked first, synchronously.
func TestHyva_ABadURLFailsOnTheFormWithoutANetworkCall(t *testing.T) {
	m, _ := submitWithHyva(t, "hyva-themes.repo.packagist.com/acme", "s3cret")

	if m.phase != phaseSetupConfig || m.verifyingHyva {
		t.Fatalf("expected to stay on the form without verifying, got phase %d verifying=%v", m.phase, m.verifyingHyva)
	}
	if !contains(m.setupError, "Repo URL must be a full URL") {
		t.Errorf("setupError = %q, expected the URL rule", m.setupError)
	}
}

// TestHyva_SubmitChecksTheCredentialsBeforeThePreview verifies submitting
// starts the check and waits for it instead of jumping to the preview.
func TestHyva_SubmitChecksTheCredentialsBeforeThePreview(t *testing.T) {
	m, cmd := submitWithHyva(t, "https://hyva-themes.repo.packagist.com/acme/", "s3cret")

	if m.phase != phaseSetupConfig || !m.verifyingHyva {
		t.Fatalf("expected the form to wait for the check, got phase %d verifying=%v", m.phase, m.verifyingHyva)
	}
	if cmd == nil {
		t.Error("submit should start the credential check")
	}
	if !contains(m.View(), "Checking the Hyvä credentials") {
		t.Error("form should say it is checking")
	}
}

// TestHyva_KeysAreIgnoredWhileChecking verifies the form cannot be resubmitted
// or edited mid-check.
func TestHyva_KeysAreIgnoredWhileChecking(t *testing.T) {
	m, _ := submitWithHyva(t, "https://hyva-themes.repo.packagist.com/acme/", "s3cret")

	m = pressEnter(m)

	if m.phase != phaseSetupConfig || !m.verifyingHyva {
		t.Errorf("Enter during the check should do nothing, got phase %d verifying=%v", m.phase, m.verifyingHyva)
	}
}

// TestHyva_AcceptedCredentialsGoToThePreview verifies the happy path.
func TestHyva_AcceptedCredentialsGoToThePreview(t *testing.T) {
	m, _ := submitWithHyva(t, "https://hyva-themes.repo.packagist.com/acme/", "s3cret")

	m = sendMsg(m, hyvaVerifiedMsg{})

	if m.phase != phaseSetupPreview {
		t.Errorf("expected phaseSetupPreview, got %d", m.phase)
	}
	if m.installCfg.HyvaRepoURL != "https://hyva-themes.repo.packagist.com/acme/" || m.installCfg.HyvaAuthToken != "s3cret" {
		t.Error("install config should carry the Hyvä credentials")
	}
}

// TestHyva_RejectedTokenStaysOnTheFormAtTheTokenField verifies the failure is
// shown where the user can fix it.
func TestHyva_RejectedTokenStaysOnTheFormAtTheTokenField(t *testing.T) {
	m, _ := submitWithHyva(t, "https://hyva-themes.repo.packagist.com/acme/", "wrong")

	m = sendMsg(m, hyvaVerifiedMsg{err: hyva.ErrRejected})

	if m.phase != phaseSetupConfig || m.verifyingHyva {
		t.Fatalf("expected to be back on the form, got phase %d verifying=%v", m.phase, m.verifyingHyva)
	}
	if !contains(m.View(), "Hyvä rejected the token") {
		t.Error("form should show why the check failed")
	}
	if !m.inTogglePhase || m.toggleFocus != hyvaAuthTokenField {
		t.Errorf("focus should be on the token field, got inTogglePhase=%v toggleFocus=%d", m.inTogglePhase, m.toggleFocus)
	}
}

// TestHyva_NoCheckWhenHyvaIsOff verifies the plain flow still goes straight to
// the preview.
func TestHyva_NoCheckWhenHyvaIsOff(t *testing.T) {
	m := advanceToSetupPreview(t)

	if m.verifyingHyva {
		t.Error("no Hyvä, no credential check")
	}
}

// --- store settings on the form ---

// TestStoreSettings_AreOnTheForm verifies the three fields show up with
// non-empty defaults.
func TestStoreSettings_AreOnTheForm(t *testing.T) {
	m := finishPage(advanceToSetupConfig(t))
	view := m.View()

	for _, want := range []string{"Locale", "Timezone", "Currency"} {
		if !contains(view, want) {
			t.Errorf("setup form should show a %s field", want)
		}
	}
	for _, field := range []int{localeField, timezoneField, currencyField} {
		if m.setupInputs[field].Value() == "" {
			t.Errorf("field %d should have a detected or fallback default", field)
		}
	}
}

// TestStoreSettings_DefaultsFollowTheMachine verifies the defaults come from
// locale detection rather than a hardcoded Dutch setup.
func TestStoreSettings_DefaultsFollowTheMachine(t *testing.T) {
	values := defaultSetupValues(locale.Defaults{Locale: "de_DE", Timezone: "Europe/Berlin", Currency: "EUR"})

	if values[localeField] != "de_DE" || values[timezoneField] != "Europe/Berlin" || values[currencyField] != "EUR" {
		t.Errorf("defaultSetupValues() = %q, expected the detected store settings in place", values)
	}
	if len(values) != len(setupFieldDefs) {
		t.Errorf("defaults have %d entries for %d fields", len(values), len(setupFieldDefs))
	}
}

// TestStoreSettings_ReachTheInstallCommand verifies what the user typed ends up
// in the setup:install flags of both environments.
func TestStoreSettings_ReachTheInstallCommand(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[localeField].SetValue("nl_NL")
	m.setupInputs[timezoneField].SetValue("Europe/Amsterdam")
	m.setupInputs[currencyField].SetValue("EUR")
	m = completeForm(m)
	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d (%s)", m.phase, m.setupError)
	}

	for name, d := range map[string]detector.Detector{"DDEV": &detector.DdevDetector{}, "Warden": &detector.WardenDetector{}} {
		flags := d.SetupInstallFlags(&m.installCfg)
		got := map[string]string{}
		for _, f := range flags {
			got[f.Flag] = f.Value
		}
		if got["--language"] != "nl_NL" || got["--timezone"] != "Europe/Amsterdam" || got["--currency"] != "EUR" {
			t.Errorf("[%s] flags = language %q timezone %q currency %q", name, got["--language"], got["--timezone"], got["--currency"])
		}
	}
}

// TestStoreSettings_BadTimezoneStaysOnTheForm verifies a zone Magento would
// refuse is caught here, with focus on the field.
func TestStoreSettings_BadTimezoneStaysOnTheForm(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[timezoneField].SetValue("Amsterdam")

	m = toOptionsPage(m) // the store screen refuses to be left

	if m.phase != phaseSetupConfig {
		t.Fatalf("expected to stay on the form, got phase %d", m.phase)
	}
	if !contains(m.setupError, "Timezone must be") || m.setupFocus != timezoneField {
		t.Errorf("setupError = %q focus = %d, expected the timezone rule with focus on the field", m.setupError, m.setupFocus)
	}
}

// TestStoreSettings_BadCurrencyStaysOnTheForm verifies the currency rule.
func TestStoreSettings_BadCurrencyStaysOnTheForm(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[currencyField].SetValue("euro")

	m = toOptionsPage(m) // the store screen refuses to be left

	if m.phase != phaseSetupConfig || m.setupFocus != currencyField {
		t.Errorf("expected to stay on the form at the currency field, got phase %d focus %d", m.phase, m.setupFocus)
	}
}

// --- resume across runs ---

var resumeSteps = []detector.Step{{Name: "Configure"}, {Name: "Start"}, {Name: "Install Mage-OS"}, {Name: "Verify installation"}}

// installingIn drives a fresh model into a running install in dir, with the
// mock detector reporting resumeSteps.
func installingIn(t *testing.T, dir string) Model {
	t.Helper()
	m := passChecks(pressEnter(New()))
	env := makeDetectedEnv("DDEV")
	env.Detector.(*mockDetector).steps = resumeSteps
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{env}})
	m.dirInput.SetValue(dir)
	m = pressEnter(m)
	m = completeForm(m)
	if m.phase != phaseSetupPreview {
		t.Fatalf("expected phaseSetupPreview, got %d (%s)", m.phase, m.setupError)
	}
	m.initInstallSteps()
	m.phase = phaseInstalling
	return m
}

// TestResume_ProgressIsSavedAfterEachStepWithoutThePassword verifies the
// state file tracks finished steps and carries settings but no secrets.
func TestResume_ProgressIsSavedAfterEachStepWithoutThePassword(t *testing.T) {
	dir := t.TempDir()
	m := installingIn(t, dir)

	m = sendMsg(m, stepDoneMsg{index: 0})
	m = sendMsg(m, stepDoneMsg{index: 1})

	state, found, err := resume.Load(dir)
	if err != nil || !found {
		t.Fatalf("expected a state file, found=%v err=%v", found, err)
	}
	if len(state.Completed) != 2 || state.Completed[1] != "Start" {
		t.Errorf("Completed = %q, expected the two finished steps", state.Completed)
	}
	if state.Fields["Admin user"] != "admin" || state.Environment != "DDEV" {
		t.Errorf("state should carry the settings, got %+v", state)
	}
	if _, saved := state.Fields["Admin password"]; saved {
		t.Error("the admin password must never be saved")
	}
}

// TestResume_AFinishedInstallForgetsTheState verifies nothing is left to
// resume after success.
func TestResume_AFinishedInstallForgetsTheState(t *testing.T) {
	dir := t.TempDir()
	m := installingIn(t, dir)
	m = sendMsg(m, stepDoneMsg{index: 0})

	m = sendMsg(m, installDoneMsg{})

	if _, found, _ := resume.Load(dir); found {
		t.Error("state file should be removed once the install completes")
	}
}

// unfinishedIn leaves the state of a run that stopped after two steps in dir.
func unfinishedIn(t *testing.T, dir string) {
	t.Helper()
	err := resume.Save(dir, resume.State{
		Environment: "DDEV", ProjectName: "shop",
		Fields:    map[string]string{"Admin user": "michiel", "Admin email": "m@example.com", "Admin firstname": "Michiel", "Admin lastname": "G"},
		InitGit:   true,
		Steps:     []string{"Configure", "Start", "Install Mage-OS", "Verify installation"},
		Completed: []string{"Configure", "Start"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// atResumePrompt drives a fresh model to the resume question for dir.
func atResumePrompt(t *testing.T, dir string) Model {
	t.Helper()
	m := passChecks(pressEnter(New()))
	env := makeDetectedEnv("DDEV")
	env.Detector.(*mockDetector).steps = resumeSteps
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{env}})
	m.dirInput.SetValue(dir)
	m = pressEnter(m)
	if m.phase != phaseResumePrompt {
		t.Fatalf("expected phaseResumePrompt, got %d", m.phase)
	}
	return m
}

// TestResume_IsOfferedWhenTheDirectoryHasAnUnfinishedRun verifies the prompt
// and what it says.
func TestResume_IsOfferedWhenTheDirectoryHasAnUnfinishedRun(t *testing.T) {
	dir := t.TempDir()
	unfinishedIn(t, dir)

	view := atResumePrompt(t, dir).View()

	for _, want := range []string{"earlier install of shop", "2 of 4 steps", "Install Mage-OS", "admin password is asked for again", "y to resume"} {
		if !contains(view, want) {
			t.Errorf("resume prompt should contain %q", want)
		}
	}
}

// TestResume_YesRestoresTheSettingsAndStartsAtTheNextStep verifies the form
// comes back filled, the password does not, and the install skips what is
// done.
func TestResume_YesRestoresTheSettingsAndStartsAtTheNextStep(t *testing.T) {
	dir := t.TempDir()
	unfinishedIn(t, dir)
	m := atResumePrompt(t, dir)

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}
	if m.setupInputs[adminUserField].Value() != "michiel" || m.setupInputs[adminEmailField].Value() != "m@example.com" {
		t.Error("admin fields should be restored from the earlier run")
	}
	if m.setupInputs[adminPasswordField].Value() != "" || m.setupFocus != adminPasswordField {
		t.Error("the password should be empty and focused, waiting for the user")
	}
	if !m.initGit {
		t.Error("toggles should be restored")
	}

	m.setupInputs[adminPasswordField].SetValue("Wachtwoord123")
	m = completeForm(m)

	if m.phase != phaseSetupPreview || m.installCfg.StartFromStep != 2 {
		t.Errorf("expected the preview with StartFromStep 2, got phase %d start %d (%s)", m.phase, m.installCfg.StartFromStep, m.setupError)
	}
	m.initInstallSteps()
	if m.installSteps[0].status != stepDone || m.installSteps[1].status != stepDone || m.installSteps[2].status != stepPending {
		t.Error("the finished steps should show as done and the rest as pending")
	}
}

// TestResume_NoStartsOverAndForgetsTheRun verifies declining clears the file
// and gives a default form.
func TestResume_NoStartsOverAndForgetsTheRun(t *testing.T) {
	dir := t.TempDir()
	unfinishedIn(t, dir)
	m := atResumePrompt(t, dir)

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if m.phase != phaseSetupConfig || m.resuming {
		t.Errorf("expected a fresh form, got phase %d resuming=%v", m.phase, m.resuming)
	}
	if m.setupInputs[adminUserField].Value() != "admin" {
		t.Error("the form should have its defaults, not the earlier run's values")
	}
	if _, found, _ := resume.Load(dir); found {
		t.Error("declining should remove the state file")
	}
}

// TestResume_NotOfferedWhenTheEnvironmentIsGone verifies a run made with an
// environment that is no longer detected falls back to the normal flow.
func TestResume_NotOfferedWhenTheEnvironmentIsGone(t *testing.T) {
	dir := t.TempDir()
	unfinishedIn(t, dir)
	m := passChecks(pressEnter(New()))
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("Warden")}})
	m.dirInput.SetValue(dir)

	m = pressEnter(m)

	if m.phase != phaseSetupConfig || m.earlierRun != nil {
		t.Errorf("expected the normal setup form, got phase %d earlierRun=%v", m.phase, m.earlierRun)
	}
}

// TestResume_NoStateMeansNoQuestion verifies the common case is untouched.
func TestResume_NoStateMeansNoQuestion(t *testing.T) {
	m := passChecks(pressEnter(New()))
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	m.dirInput.SetValue(t.TempDir())

	if m = pressEnter(m); m.phase != phaseSetupConfig {
		t.Errorf("expected phaseSetupConfig, got %d", m.phase)
	}
}

// --- flags pre-fill the screens ---

func givenOptions() options.Options {
	o := options.Defaults
	o.ProjectName = "flagged-shop"
	o.Directory = "/srv/flagged-shop"
	o.AdminUser = "michiel"
	o.AdminPassword = "Wachtwoord123"
	o.SampleData = true
	o.InitGit = false
	o.Hyva = true
	o.HyvaRepoURL = "https://hyva-themes.repo.packagist.com/acme/"
	o.HyvaAuthToken = "t0k3n"
	return o
}

// TestOptions_PrefillTheNameAndDirectory verifies --project and --dir land in
// the first two prompts.
func TestOptions_PrefillTheNameAndDirectory(t *testing.T) {
	m := NewWithOptions(givenOptions())
	if m.nameInput.Value() != "flagged-shop" {
		t.Errorf("name = %q", m.nameInput.Value())
	}

	m = pressEnter(m)

	if m.dirInput.Value() != "/srv/flagged-shop" {
		t.Errorf("dir = %q", m.dirInput.Value())
	}
}

// TestOptions_PrefillTheFormAndToggles verifies the admin fields, toggles
// and Hyvä credentials come from the flags, with screen defaults for the rest.
func TestOptions_PrefillTheFormAndToggles(t *testing.T) {
	m := passChecks(NewWithOptions(givenOptions()))
	m = pressEnter(m)
	m = sendMsg(m, detectionDoneMsg{envs: []detector.DetectedEnvironment{makeDetectedEnv("DDEV")}})
	m = pressEnter(m)
	if m.phase != phaseSetupConfig {
		t.Fatalf("expected phaseSetupConfig, got %d", m.phase)
	}

	if m.setupInputs[adminUserField].Value() != "michiel" || m.setupInputs[adminPasswordField].Value() != "Wachtwoord123" {
		t.Error("admin user and password should come from the flags")
	}
	if m.setupInputs[adminEmailField].Value() != options.Defaults.AdminEmail {
		t.Error("a field without a flag should keep its default")
	}
	if !m.installSampleData || m.initGit || !m.installHyva {
		t.Errorf("toggles should follow the flags: sample %v git %v hyva %v", m.installSampleData, m.initGit, m.installHyva)
	}
	if m.hyvaInputs[0].Value() != "https://hyva-themes.repo.packagist.com/acme/" || m.hyvaInputs[1].Value() != "t0k3n" {
		t.Error("Hyvä credentials should come from the flags")
	}
}

// TestOptions_NoFlagsMeansTheOldDefaults verifies New() behaves as before.
func TestOptions_NoFlagsMeansTheOldDefaults(t *testing.T) {
	m := advanceToSetupConfig(t)

	if m.setupInputs[adminPasswordField].Value() != setupFieldDefaults[adminPasswordField] || !m.initGit || m.installSampleData {
		t.Error("without flags the form should show its own defaults")
	}
}


// --- the form in three screens ---

// TestSetupScreens_StartOnTheAdminAccount verifies the first screen shows the
// account fields and nothing from the later screens.
func TestSetupScreens_StartOnTheAdminAccount(t *testing.T) {
	m := advanceToSetupConfig(t)
	view := m.View()

	for _, want := range []string{"Admin account", "step 1 of 4", "Admin password", "Enter to continue"} {
		if !contains(view, want) {
			t.Errorf("first screen should contain %q", want)
		}
	}
	for _, unwanted := range []string{"Timezone", "Install sample data", "Install Hyvä"} {
		if contains(view, unwanted) {
			t.Errorf("first screen should not contain %q", unwanted)
		}
	}
}

// TestSetupScreens_EnterOnTheLastFieldMovesOn verifies Enter walks the screen
// and then to the next one, landing on its first field.
func TestSetupScreens_EnterOnTheLastFieldMovesOn(t *testing.T) {
	m := advanceToSetupConfig(t)
	for i := 0; i < len(setupFieldDefs); i++ {
		if pageOf(i) != pageAdmin {
			break
		}
		m = pressEnter(m)
	}

	if m.setupPage != pageStore || m.setupFocus != localeField {
		t.Errorf("expected the store screen at its first field, got page %d focus %d", m.setupPage, m.setupFocus)
	}
	if !contains(m.View(), "step 2 of 4") {
		t.Error("second screen should say it is step 2 of 4")
	}
}

// TestSetupScreens_TabStaysOnTheScreen verifies Tab cycles within a screen
// rather than wandering onto the next.
func TestSetupScreens_TabStaysOnTheScreen(t *testing.T) {
	m := advanceToSetupConfig(t)
	for i := 0; i < 20; i++ {
		m = sendMsg(m, tea.KeyMsg{Type: tea.KeyTab})
	}

	if m.setupPage != pageAdmin {
		t.Errorf("Tab should never leave the screen, got page %d", m.setupPage)
	}
}

// TestSetupScreens_EscGoesBackAScreen verifies the way back keeps the values.
func TestSetupScreens_EscGoesBackAScreen(t *testing.T) {
	m := finishPage(advanceToSetupConfig(t))
	m.setupInputs[localeField].SetValue("nl_NL")

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.setupPage != pageAdmin {
		t.Errorf("Esc should return to the admin screen, got page %d", m.setupPage)
	}
	if m.setupInputs[localeField].Value() != "nl_NL" {
		t.Error("going back should keep what was typed on the later screen")
	}
	if m = finishPage(m); m.setupPage != pageStore {
		t.Error("the admin screen should still pass on the way forward again")
	}
}

// TestSetupScreens_EachScreenIsCheckedWhenLeft verifies a bad admin password
// keeps the user on screen one, and never shows screen two's problems early.
func TestSetupScreens_EachScreenIsCheckedWhenLeft(t *testing.T) {
	m := advanceToSetupConfig(t)
	m.setupInputs[adminPasswordField].SetValue("short")
	m.setupInputs[timezoneField].SetValue("Nowhere")

	m = finishPage(m)

	if m.setupPage != pageAdmin || !contains(m.setupError, "at least 12 characters") {
		t.Errorf("expected to stay on the admin screen with the password rule, got page %d error %q", m.setupPage, m.setupError)
	}
}

// TestSetupScreens_OptionsScreenHasSampleDataAndGit verifies the third screen
// holds the two plain toggles and hands over to the Hyvä screen.
func TestSetupScreens_OptionsScreenHasSampleDataAndGit(t *testing.T) {
	m := toOptionsPage(advanceToSetupConfig(t))
	view := m.View()
	for _, want := range []string{"Options", "step 3 of 4", "Install sample data", "Initialize Git", "Space to toggle", "Enter to continue"} {
		if !contains(view, want) {
			t.Errorf("options screen should contain %q", want)
		}
	}
	if contains(view, "Install Hyvä") {
		t.Error("Hyvä has its own screen and should not be on the options screen")
	}

	if m = finishPage(m); m.setupPage != pageHyva {
		t.Errorf("expected the Hyvä screen after the options screen, got page %d", m.setupPage)
	}
}

// TestSetupScreens_HyvaScreenSubmits verifies the last screen explains Hyvä,
// shows its credentials only when it is on, and leads to the preview.
func TestSetupScreens_HyvaScreenSubmits(t *testing.T) {
	m := toHyvaPage(advanceToSetupConfig(t))
	view := m.View()
	for _, want := range []string{"Hyvä theme", "step 4 of 4", "Install Hyvä", "hyva.io", "Enter to review command"} {
		if !contains(view, want) {
			t.Errorf("Hyvä screen should contain %q", want)
		}
	}
	if contains(view, "Repo URL") {
		t.Error("credentials should stay hidden while Hyvä is off")
	}

	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.installHyva || !contains(m.View(), "Repo URL") {
		t.Error("Space should switch Hyvä on and reveal its credential fields")
	}
	m = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	if m = finishPage(m); m.phase != phaseSetupPreview {
		t.Errorf("expected the preview after the Hyvä screen, got phase %d", m.phase)
	}
}
