package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/hyva"

	"github.com/mage-os/mage-os-install/internal/locale"
	"github.com/mage-os/mage-os-install/internal/magento"
	"github.com/mage-os/mage-os-install/internal/prereq"

	"github.com/mage-os/mage-os-install/internal/resume"

	"github.com/mage-os/mage-os-install/internal/options"
)

type phase int

const (
	phaseNameInput phase = iota
	phaseDirectoryInput
	phaseDirectoryConfirm // the chosen directory is not empty

	phaseChecking     // prerequisite checks still running
	phaseChecksReview // a prerequisite failed or warned
	phaseDetecting
	phaseSelection
	phaseResumePrompt // an earlier install in this directory can be continued
	phaseSetupConfig  // admin credentials form
	phaseSetupPreview // full command preview before running
	phaseInstalling
	phaseOpenBrowser // ask whether to open the site after install
	phaseInstallDone
	phaseLogView // the whole install log, scrollable
	phaseError
)

// detectionDoneMsg is sent when environment detection completes.
type detectionDoneMsg struct {
	envs []detector.DetectedEnvironment
}

// checksDoneMsg is sent when the prerequisite checks complete.
type checksDoneMsg struct {
	results []prereq.Result
}

// hyvaVerifiedMsg is sent once the Hyvä credentials have been checked.
type hyvaVerifiedMsg struct {
	err error
}

// installDoneMsg is sent when installation completes.
type installDoneMsg struct {
	err error
}

// logMsg carries a single line of installer output.
type logMsg string

// stepStartMsg is sent when an installation step begins.
type stepStartMsg struct{ index int }

// sudoCachedMsg is sent after sudo -v completes (caching credentials).
type sudoCachedMsg struct{ err error }

// stepDoneMsg is sent when an installation step completes successfully.
type stepDoneMsg struct{ index int }

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepFailed
)

type installStep struct {
	name       string
	status     stepStatus
	startedAt  time.Time
	finishedAt time.Time
}

// Model is the main bubbletea model for the installer TUI.
type Model struct {
	opts          options.Options // answers given on the command line, pre-filling the screens
	phase         phase
	spinner       spinner.Model
	nameInput     textinput.Model
	originalName  string // the default seeded into nameInput, used to compute the dir default
	dirInput      textinput.Model
	dirContents   string // what is already in the chosen directory, if anything
	envs          []detector.DetectedEnvironment
	checks        []prereq.Result
	checksDone    bool
	cursor        int
	selected      *detector.DetectedEnvironment
	earlierRun    *resume.State // an unfinished install found in the directory
	resuming      bool          // the user chose to continue it
	setupInputs   []textinput.Model
	setupFocus    int
	setupError    string
	installCfg    detector.Config
	previewScroll int
	windowHeight  int
	windowWidth   int
	logCh         <-chan tea.Msg
	logLines      []string
	logScroll     int // first visible line in the log view
	installSteps  []installStep
	installStart  time.Time // when the current install run began
	installErr    error
	browserOpened bool

	// passwordRevealed shows the admin password in clear text on the form.
	passwordRevealed bool

	// verifyingHyva is true while the Hyvä credentials are being checked.
	verifyingHyva bool

	// Optional feature toggles and Hyvä credentials
	installSampleData bool
	initGit           bool
	installHyva       bool
	hyvaInputs        []textinput.Model
	// toggleFocus tracks which toggle/section has focus:
	//   sampleDataToggle / initGitToggle / hyvaToggle = a toggle
	//   0+ = hyva input field index
	toggleFocus   int
	inTogglePhase bool // true when focus is in the toggle/hyva section (not admin fields)
}

// currentDirName returns the base name of the working directory.
func currentDirName() string {
	if wd, err := os.Getwd(); err == nil {
		return filepath.Base(wd)
	}
	return "my-project"
}

// New creates the initial TUI model with nothing answered in advance.
func New() Model {
	return NewWithOptions(options.Defaults)
}

// NewWithOptions creates the initial TUI model with the command-line answers
// filled in, so a flag given once does not have to be typed again on screen.
func NewWithOptions(opts options.Options) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	defaultName := currentDirName()
	name := defaultName
	if opts.ProjectName != "" {
		name = opts.ProjectName
	}

	ti := textinput.New()
	ti.SetValue(name)
	ti.Focus()
	ti.CharLimit = 64

	return Model{
		opts:         opts,
		phase:        phaseNameInput,
		spinner:      s,
		nameInput:    ti,
		originalName: defaultName,
	}
}

func (m Model) Init() tea.Cmd {
	// Start detection and the prerequisite checks in the background
	// immediately so results are ready by the time the user confirms the
	// project name and directory.
	return tea.Batch(m.spinner.Tick, detectEnvironments, runPrerequisiteChecks, textinput.Blink)
}

func detectEnvironments() tea.Msg {
	return detectionDoneMsg{envs: detector.DetectAll()}
}

func runPrerequisiteChecks() tea.Msg {
	return checksDoneMsg{results: prereq.RunAll(prereq.Default())}
}

// defaultDirectory returns the install directory based on whether the user
// changed the project name from the default.
func (m *Model) defaultDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	if m.nameInput.Value() == m.originalName {
		return wd
	}
	return filepath.Join(wd, m.nameInput.Value())
}

// enterDirectoryPhase transitions to the directory input phase,
// pre-filling the input with the computed default.
func (m *Model) enterDirectoryPhase() tea.Cmd {
	di := textinput.New()
	di.SetValue(m.defaultDirectory())
	if m.opts.Directory != "" {
		di.SetValue(m.opts.Directory)
	}
	di.Focus()
	di.CharLimit = 256
	m.dirInput = di
	m.phase = phaseDirectoryInput
	return textinput.Blink
}

// initInstallSteps loads the step list from the selected detector.
func (m *Model) initInstallSteps() {
	m.installSteps = nil
	for i, s := range m.selected.Detector.Steps() {
		status := stepPending
		if i < m.installCfg.StartFromStep {
			status = stepDone // finished by the run being resumed
		}
		m.installSteps = append(m.installSteps, installStep{name: s.Name, status: status})
	}
}

// setupFieldDefs defines the user-editable fields shown in the Mage-OS setup form.
var setupFieldDefs = []struct {
	label string
	echo  textinput.EchoMode
}{
	{label: "Admin user", echo: textinput.EchoNormal},
	{label: "Admin password", echo: textinput.EchoPassword},
	{label: "Admin email", echo: textinput.EchoNormal},
	{label: "Admin firstname", echo: textinput.EchoNormal},
	{label: "Admin lastname", echo: textinput.EchoNormal},
	{label: "Locale", echo: textinput.EchoNormal},
	{label: "Timezone", echo: textinput.EchoNormal},
	{label: "Currency", echo: textinput.EchoNormal},
}

// Positions of the toggles below the admin fields, in the order they appear on
// screen. They are negative to tell them apart from the Hyvä input fields,
// which use toggleFocus 0 and up.
const (
	sampleDataToggle = -3
	initGitToggle    = -2
	hyvaToggle       = -1
)

// toggleCount is how many toggles sit between the admin fields and the Hyvä
// credential fields.
const toggleCount = 3

// Positions of the fields in setupFieldDefs, setupFieldDefaults and setupInputs.
const (
	adminUserField = iota
	adminPasswordField
	adminEmailField
	adminFirstnameField
	adminLastnameField
	localeField
	timezoneField
	currencyField
)

// setupFieldDefaults pre-fill the form. The store settings come from the
// machine the installer runs on, so a Dutch developer is offered nl_NL,
// Europe/Amsterdam and EUR rather than having to know to change them.
var setupFieldDefaults = defaultSetupValues(locale.Detect())

// defaultSetupValues lists the defaults in setupFieldDefs order.
func defaultSetupValues(store locale.Defaults) []string {
	return []string{"admin", "Admin123!Mage", "admin@example.com", "Admin", "User",
		store.Locale, store.Timezone, store.Currency}
}

// Positions of the fields in hyvaFieldDefs and hyvaInputs.
const (
	hyvaRepoURLField = iota
	hyvaAuthTokenField
)

// hyvaFieldDefs defines the Hyvä credential fields.
var hyvaFieldDefs = []struct {
	label string
	echo  textinput.EchoMode
}{
	{label: "Repo URL", echo: textinput.EchoNormal},
	{label: "Auth token", echo: textinput.EchoPassword},
}

// initSetupInputs creates fresh text inputs for the Mage-OS setup form.
func (m *Model) initSetupInputs() {
	m.setupInputs = make([]textinput.Model, len(setupFieldDefs))
	for i, f := range setupFieldDefs {
		ti := textinput.New()
		ti.SetValue(m.setupFieldValue(i))
		ti.EchoMode = f.echo
		ti.CharLimit = 128
		if i == 0 {
			ti.Focus()
		}
		m.setupInputs[i] = ti
	}
	m.setupFocus = 0

	// Initialize toggles and Hyvä inputs
	m.installSampleData = m.opts.SampleData
	m.initGit = m.opts.InitGit
	m.installHyva = m.opts.Hyva
	m.inTogglePhase = false
	m.toggleFocus = sampleDataToggle
	m.hyvaInputs = make([]textinput.Model, len(hyvaFieldDefs))
	for i, f := range hyvaFieldDefs {
		ti := textinput.New()
		ti.EchoMode = f.echo
		ti.CharLimit = 256
		m.hyvaInputs[i] = ti
	}
	m.hyvaInputs[0].SetValue(m.opts.HyvaRepoURL)
	m.hyvaInputs[1].SetValue(m.opts.HyvaAuthToken)
}

// setupFieldValue is what a form field starts out with: the flag if one was
// given, otherwise the screen's own default.
func (m *Model) setupFieldValue(field int) string {
	given := []string{m.opts.AdminUser, m.opts.AdminPassword, m.opts.AdminEmail, m.opts.AdminFirstname, m.opts.AdminLastname}
	if field < len(given) && given[field] != "" {
		return given[field]
	}
	return setupFieldDefaults[field]
}

// focusSetupInput focuses the input at index and blurs all others.
func (m *Model) focusSetupInput(index int) {
	for i := range m.setupInputs {
		if i == index {
			m.setupInputs[i].Focus()
		} else {
			m.setupInputs[i].Blur()
		}
	}
	m.setupFocus = index
}

// focusAbsolutePos sets focus based on absolute position across all form fields.
// Layout: [admin fields...] [toggles...] [hyva fields if enabled...]
func (m *Model) focusAbsolutePos(pos int) {
	// Blur everything
	for i := range m.setupInputs {
		m.setupInputs[i].Blur()
	}
	for i := range m.hyvaInputs {
		m.hyvaInputs[i].Blur()
	}

	firstTogglePos := len(m.setupInputs)

	switch {
	case pos < firstTogglePos:
		// Admin field
		m.inTogglePhase = false
		m.setupFocus = pos
		m.setupInputs[pos].Focus()
	case pos < firstTogglePos+toggleCount:
		// Toggle, in screen order
		m.inTogglePhase = true
		m.toggleFocus = sampleDataToggle + (pos - firstTogglePos)
	default:
		// Hyva input field
		m.inTogglePhase = true
		m.toggleFocus = pos - firstTogglePos - toggleCount
		if m.toggleFocus < len(m.hyvaInputs) {
			m.hyvaInputs[m.toggleFocus].Focus()
		}
	}
}

// sudoPrompt replaces sudo's bare "Password:" so the request has a reason
// attached. Percent signs are sudo escapes, so the text avoids them.
const sudoPrompt = "Your computer login password (sudo) is needed to update /etc/hosts: "

// sudoRefreshCommand caches sudo credentials before the install starts, so
// steps like "ddev start" (which edits /etc/hosts) do not stop halfway to ask.
func sudoRefreshCommand() *exec.Cmd {
	return exec.Command("sudo", "-v", "-p", sudoPrompt)
}

// sudoWarningLines tell the user about that prompt while they can still read
// the screen, and separate it from the Mage-OS admin password they just chose.
func sudoWarningLines(envName string) []string {
	return []string{
		fmt.Sprintf("⚠ Enter may ask for your computer login password: %s needs sudo to", envName),
		"  add the project hostname to /etc/hosts. This is not the Mage-OS admin password.",
	}
}

// successSummaryLines tell the user where the store is and how to get into it.
// The admin password is masked on the form, so someone who accepted the
// default may never have seen it, and the admin URL depends on the backend
// front name the environment chose.
func (m *Model) successSummaryLines() []string {
	const labelWidth = 11
	row := func(label, value string) string {
		return fmt.Sprintf("  %-*s %s", labelWidth, label, value)
	}
	baseURL := m.selected.Detector.BaseURL(m.installCfg.ProjectName)
	return []string{
		successStyle.Render("✓ " + m.selected.Env.Name + " installed successfully!"),
		"",
		row("Storefront", highlightStyle.Render(baseURL)),
		row("Admin", highlightStyle.Render(m.adminURL(baseURL))),
		row("Login", m.installCfg.AdminUser+" / "+m.installCfg.AdminPassword),
		row("Project", m.installCfg.Directory),
		"",
		dimStyle.Render("  Run Magento commands from the project directory with:"),
		"  " + m.selected.Detector.MagentoCommand() + " <command>",
	}
}

// adminURL is the storefront URL plus the backend front name the store was
// installed with.
func (m *Model) adminURL(baseURL string) string {
	flags := m.selected.Detector.SetupInstallFlags(&m.installCfg)
	return strings.TrimRight(baseURL, "/") + "/" + setupFlagValue(flags, detector.BackendFrontnameFlag)
}

// setupFlagValue looks a flag up by name in the setup:install flag list.
func setupFlagValue(flags []detector.SetupFlag, name string) string {
	for _, flag := range flags {
		if flag.Flag == name {
			return flag.Value
		}
	}
	return ""
}

// revealPasswordKey toggles the admin password between masked and clear text.
const revealPasswordKey = "ctrl+r"

// togglePasswordReveal flips the password field between masked and clear text.
// The value is a development login the user typed (or accepted) a moment ago,
// and the command preview prints it anyway, so hiding it is a courtesy for
// shared screens rather than a security boundary.
func (m *Model) togglePasswordReveal() {
	m.passwordRevealed = !m.passwordRevealed
	if m.passwordRevealed {
		m.setupInputs[adminPasswordField].EchoMode = textinput.EchoNormal
		return
	}
	m.setupInputs[adminPasswordField].EchoMode = textinput.EchoPassword
}

// passwordHints explain the rules, name the default while it is still in the
// field (a masked default is otherwise unknowable), and say how to reveal it.
func (m *Model) passwordHints() []string {
	hints := []string{magento.AdminPasswordHint()}

	value := m.setupInputs[adminPasswordField].Value()
	usage := revealPasswordKey + " to show"
	if m.passwordRevealed {
		usage = revealPasswordKey + " to hide"
	}
	if value == setupFieldDefaults[adminPasswordField] && !m.passwordRevealed {
		usage = "default: " + value + " · " + usage
	}
	return append(hints, usage)
}

// previewLines are the scrollable body of the preview screen: the steps the
// install will run, with a duration hint where the wait is long enough to
// look like a hang, followed by the setup:install command itself.
func (m *Model) previewLines() []string {
	var lines []string

	steps := m.selected.Detector.Steps()
	nameWidth := 0
	for _, step := range steps {
		nameWidth = max(nameWidth, len([]rune(step.Name)))
	}
	for i, step := range steps {
		line := fmt.Sprintf("  %2d. %-*s", i+1, nameWidth, step.Name)
		if step.Estimate != "" {
			line += "  " + dimStyle.Render(step.Estimate)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "", "Then run:", "", m.selected.Detector.SetupCommandPrefix()+" \\")
	flags := m.selected.Detector.SetupInstallFlags(&m.installCfg)
	for i, f := range flags {
		suffix := dimStyle.Render(" \\")
		if i == len(flags)-1 {
			suffix = ""
		}
		if f.Editable {
			lines = append(lines, "  "+dimStyle.Render(f.Flag+"=")+highlightStyle.Render(f.Value)+suffix)
			continue
		}
		lines = append(lines, "  "+dimStyle.Render(f.Flag+"="+f.Value)+suffix)
	}
	return lines
}

// previewMaxVisible is how many preview lines fit between the banner and the
// footer at the current terminal height. When the terminal never reported a
// size there is nothing to fit into, so everything is shown.
func (m *Model) previewMaxVisible() int {
	const chrome = 14 // banner, title, warning and key hints
	if m.windowHeight <= 0 {
		return len(m.previewLines())
	}
	return max(5, m.windowHeight-chrome)
}

// stepLine renders one install step with how long it is taking, or took. The
// spinner already redraws the screen several times a second, which is what
// keeps the running step's timer moving.
func (m *Model) stepLine(step installStep) string {
	switch step.status {
	case stepDone:
		return successStyle.Render("  ✓ "+step.name) + dimStyle.Render("  "+formatElapsed(step.elapsed()))
	case stepRunning:
		return selectedItemStyle.Render("  ▸ "+step.name+"...") + dimStyle.Render("  "+formatElapsed(step.elapsed()))
	case stepFailed:
		return errorStyle.Render("  ✗ "+step.name) + dimStyle.Render("  "+formatElapsed(step.elapsed()))
	default:
		return dimStyle.Render("  • " + step.name)
	}
}

// viewLogKey opens the whole install log from the failure screen.
const viewLogKey = "l"

// logViewLines are the install output lines as the log view shows them:
// colour codes stripped and long lines wrapped to the terminal, so nothing is
// cut off.
func (m *Model) logViewLines() []string {
	width := 0
	if m.windowWidth > 0 {
		width = m.windowWidth - 2
	}
	var lines []string
	for _, line := range m.logLines {
		lines = append(lines, strings.Split(wrapLine(stripANSI(line), width), "\n")...)
	}
	return lines
}

// logViewRows is how many log lines fit under the banner and above the footer.
func (m *Model) logViewRows() int {
	const chrome = 14 // banner, title, blank lines and footer
	if m.windowHeight <= 0 {
		return 20
	}
	return max(5, m.windowHeight-chrome)
}

// logViewBottom is the scroll position that shows the last line.
func (m *Model) logViewBottom() int {
	return max(0, len(m.logViewLines())-m.logViewRows())
}

// continueAfterDirectory moves on once the directory is settled: first past
// the prerequisite checks, then on to the environment.
func (m *Model) continueAfterDirectory() tea.Cmd {
	if !m.checksDone {
		m.phase = phaseChecking
		return nil
	}
	if prereq.AnyFailed(m.checks) || prereq.AnyWarned(m.checks) {
		m.phase = phaseChecksReview
		return nil
	}
	return m.continueToEnvironment()
}

// continueToEnvironment picks the detected environment up, or waits for the
// detection that is still running.
func (m *Model) continueToEnvironment() tea.Cmd {
	if m.envs != nil {
		return m.advanceFromDetection()
	}
	m.phase = phaseDetecting
	return nil
}

// checksReviewView lists every prerequisite with its outcome. A failure
// blocks the install until it is fixed and re-checked; a warning only asks
// to be read.
func (m *Model) checksReviewView() string {
	failed := prereq.AnyFailed(m.checks)
	title := highlightStyle.Render("⚠ Before we start")
	if failed {
		title = errorStyle.Render("✗ Some prerequisites are missing")
	}
	lines := []string{title, ""}

	nameWidth := 0
	for _, result := range m.checks {
		nameWidth = max(nameWidth, len([]rune(result.Name)))
	}
	const marker = 6 // indent, symbol and the gap before the detail
	detailWidth := 0 // an unknown terminal width leaves the detail on one line
	if m.windowWidth > 0 {
		detailWidth = m.errorLineWidth() - nameWidth - marker
	}
	for _, result := range m.checks {
		lines = append(lines, checkLine(result, nameWidth, detailWidth))
	}

	lines = append(lines, "")
	if failed {
		lines = append(lines, dimStyle.Render("Fix the items marked ✗, then press r to check again · q to quit"))
	} else {
		lines = append(lines, dimStyle.Render("Enter to continue · r to check again · q to quit"))
	}
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)) + "\n"
}

// checkLine renders one prerequisite result, wrapping a long detail under
// itself so the box fits the terminal.
func checkLine(result prereq.Result, nameWidth, detailWidth int) string {
	name := fmt.Sprintf("%-*s", nameWidth, result.Name)
	indent := "\n" + strings.Repeat(" ", nameWidth+6)
	detail := strings.ReplaceAll(wrapLine(result.Detail, detailWidth), "\n", indent)
	switch result.Status {
	case prereq.Fail:
		return errorStyle.Render("  ✗ "+name) + "  " + detail
	case prereq.Warn:
		return highlightStyle.Render("  ⚠ "+name) + "  " + detail
	default:
		return successStyle.Render("  ✓ "+name) + dimStyle.Render("  "+detail)
	}
}

// verifyHyva checks the URL shape at once and the credentials in the
// background, so a typo or a wrong token fails here rather than at "Install
// Hyvä theme" ten minutes in.
func (m Model) verifyHyva() (tea.Model, tea.Cmd) {
	repoURL := m.hyvaInputs[hyvaRepoURLField].Value()
	if err := hyva.ValidateRepositoryURL(repoURL); err != nil {
		m.setupError = err.Error()
		m.focusAbsolutePos(len(m.setupInputs) + toggleCount + hyvaRepoURLField)
		return m, textinput.Blink
	}
	m.verifyingHyva = true
	token := m.hyvaInputs[hyvaAuthTokenField].Value()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		return hyvaVerifiedMsg{err: hyva.VerifyCredentials(context.Background(), repoURL, token)}
	})
}

// showPreview freezes the form into an install config and moves on to the
// command preview.
func (m Model) showPreview() (tea.Model, tea.Cmd) {
	m.installCfg = m.buildInstallConfig()
	m.selected.Detector.PrepareSteps(&m.installCfg)
	if m.resuming {
		m.installCfg.StartFromStep = resume.StartIndex(m.earlierRun.Completed, m.selected.Detector.Steps())
	}
	m.previewScroll = 0
	m.phase = phaseSetupPreview
	return m, nil
}

// invalidStoreSetting checks locale, timezone and currency the way
// setup:install will, and names the first field that would fail.
func (m *Model) invalidStoreSetting() (int, error) {
	checks := []struct {
		field    int
		validate func(string) error
	}{
		{localeField, locale.ValidateLocale},
		{timezoneField, locale.ValidateTimezone},
		{currencyField, locale.ValidateCurrency},
	}
	for _, check := range checks {
		if err := check.validate(strings.TrimSpace(m.setupInputs[check.field].Value())); err != nil {
			return check.field, err
		}
	}
	return 0, nil
}

// errorLineWidth is how much room a line has inside the bordered box:
// the window minus its border and padding.
func (m *Model) errorLineWidth() int {
	const boxChrome = 6 // border (2) + padding (4)
	width := m.windowWidth - boxChrome
	if width < 40 {
		width = 40
	}
	return width
}

// buildInstallConfig builds a Config from the current form and input values.
func (m *Model) buildInstallConfig() detector.Config {
	cfg := detector.Config{
		ProjectName:       m.nameInput.Value(),
		Directory:         m.dirInput.Value(),
		AdminUser:         m.setupInputs[adminUserField].Value(),
		AdminPassword:     m.setupInputs[adminPasswordField].Value(),
		AdminEmail:        m.setupInputs[adminEmailField].Value(),
		AdminFirstname:    m.setupInputs[adminFirstnameField].Value(),
		AdminLastname:     m.setupInputs[adminLastnameField].Value(),
		Locale:            m.setupInputs[localeField].Value(),
		Timezone:          m.setupInputs[timezoneField].Value(),
		Currency:          m.setupInputs[currencyField].Value(),
		InstallSampleData: m.installSampleData,
		InitGit:           m.initGit,
		InstallHyva:       m.installHyva,
	}
	if m.installHyva {
		cfg.HyvaRepoURL = m.hyvaInputs[hyvaRepoURLField].Value()
		cfg.HyvaAuthToken = m.hyvaInputs[hyvaAuthTokenField].Value()
	}
	return cfg
}

// advanceFromDetection moves the model to the correct phase based on
// how many environments were detected. The caller must have already
// populated m.envs.
func (m *Model) advanceFromDetection() tea.Cmd {
	if env := m.environmentOfEarlierRun(); env != nil {
		m.selected = env
		m.phase = phaseResumePrompt
		return nil
	}
	m.earlierRun = nil // the run's environment is gone; nothing to continue with

	switch len(m.envs) {
	case 0:
		m.phase = phaseError
	case 1:
		m.selected = &m.envs[0]
		m.initSetupInputs()
		m.phase = phaseSetupConfig
		return textinput.Blink
	default:
		m.phase = phaseSelection
	}
	return nil
}

// environmentOfEarlierRun finds the detected environment an unfinished run
// used, if there is one to continue.
func (m *Model) environmentOfEarlierRun() *detector.DetectedEnvironment {
	if m.earlierRun == nil {
		return nil
	}
	for i := range m.envs {
		if m.envs[i].Env.Name == m.earlierRun.Environment {
			return &m.envs[i]
		}
	}
	return nil
}

// resumeEarlierRun fills the form from the saved run. The admin password and
// the Hyvä token were never saved, so those fields wait for the user.
func (m *Model) resumeEarlierRun() {
	m.initSetupInputs()
	for i, f := range setupFieldDefs {
		if value, ok := m.earlierRun.Fields[f.label]; ok {
			m.setupInputs[i].SetValue(value)
		}
	}
	m.setupInputs[adminPasswordField].SetValue("")
	m.installSampleData = m.earlierRun.SampleData
	m.initGit = m.earlierRun.InitGit
	m.installHyva = m.earlierRun.Hyva
	m.hyvaInputs[0].SetValue(m.earlierRun.HyvaRepoURL)
	m.resuming = true
	m.focusSetupInput(adminPasswordField)
	m.phase = phaseSetupConfig
}

// unfinishedRun loads the state of an earlier install in dir, if any. A state
// that cannot be read is treated as absent: starting over is the safe default.
func unfinishedRun(dir string) *resume.State {
	state, found, err := resume.Load(dir)
	if err != nil || !found {
		return nil
	}
	return &state
}

// progress captures the current run for a later one to continue.
func (m *Model) progress() resume.State {
	fields := map[string]string{}
	for i, f := range setupFieldDefs {
		if i != adminPasswordField {
			fields[f.label] = m.setupInputs[i].Value()
		}
	}
	state := resume.State{
		Environment: m.selected.Env.Name,
		ProjectName: m.installCfg.ProjectName,
		Fields:      fields,
		SampleData:  m.installCfg.InstallSampleData,
		InitGit:     m.installCfg.InitGit,
		Hyva:        m.installCfg.InstallHyva,
		HyvaRepoURL: m.installCfg.HyvaRepoURL,
	}
	for _, step := range m.installSteps {
		state.Steps = append(state.Steps, step.name)
		if step.status == stepDone {
			state.Completed = append(state.Completed, step.name)
		}
	}
	return state
}

// saveProgress writes the state after every finished step. Failing to write
// it costs nothing but a resume, so it is not allowed to fail the install.
func (m *Model) saveProgress() {
	if m.selected == nil || m.installCfg.Directory == "" {
		return
	}
	_ = resume.Save(m.installCfg.Directory, m.progress())
}

func runInstall(d detector.Detector, cfg detector.Config) (<-chan tea.Msg, tea.Cmd) {
	ch := make(chan tea.Msg, 1000)

	go func() {
		if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
			ch <- installDoneMsg{err: fmt.Errorf("could not create directory %s: %w", cfg.Directory, err)}
			return
		}
		log, err := openInstallLog(cfg.Directory)
		if err != nil {
			ch <- logMsg("⚠ Could not write " + installLogPath(cfg.Directory) + ": " + err.Error())
		}
		defer log.Close()

		stepName := func(i int) string {
			if steps := d.Steps(); i < len(steps) {
				return steps[i].Name
			}
			return fmt.Sprintf("step %d", i)
		}
		cfg.Log = func(line string) {
			log.Write(line)
			ch <- logMsg(line)
		}
		cfg.OnStepStart = func(i int) {
			log.Write("--- " + stepName(i))
			ch <- stepStartMsg{index: i}
		}
		cfg.OnStepDone = func(i int) {
			log.Write("--- " + stepName(i) + ": done")
			ch <- stepDoneMsg{index: i}
		}
		err = d.Install(&cfg)
		if err != nil {
			log.Write("=== failed: " + err.Error())
		} else {
			log.Write("=== installed")
		}
		ch <- installDoneMsg{err: err}
	}()

	return ch, waitForLog(ch)
}

// waitForLog returns a Cmd that reads the next message from the log channel.
func waitForLog(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// openBrowser opens url in the default system browser without blocking.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case detectionDoneMsg:
		m.envs = msg.envs
		// Cache results while the user is still filling in the input steps;
		// we'll advance when they confirm the directory.
		if m.phase == phaseNameInput || m.phase == phaseDirectoryInput {
			return m, nil
		}
		// The prerequisite checks come first; they will pick the envs up.
		if m.phase == phaseChecking || m.phase == phaseChecksReview {
			return m, nil
		}
		return m, m.advanceFromDetection()

	case checksDoneMsg:
		m.checks = msg.results
		m.checksDone = true
		if m.phase == phaseChecking {
			return m, m.continueAfterDirectory()
		}
		return m, nil

	case sudoCachedMsg:
		// sudo credentials are now cached (or failed); proceed with install.
		m.phase = phaseInstalling
		m.installStart = now()
		ch, cmd := runInstall(m.selected.Detector, m.installCfg)
		m.logCh = ch
		return m, tea.Batch(m.spinner.Tick, cmd)

	case logMsg:
		m.logLines = append(m.logLines, string(msg))
		return m, waitForLog(m.logCh)

	case stepStartMsg:
		if msg.index < len(m.installSteps) {
			m.installSteps[msg.index].status = stepRunning
			m.installSteps[msg.index].startedAt = now()
			m.installSteps[msg.index].finishedAt = time.Time{}
		}
		return m, waitForLog(m.logCh)

	case stepDoneMsg:
		if msg.index < len(m.installSteps) {
			m.installSteps[msg.index].status = stepDone
			m.installSteps[msg.index].finishedAt = now()
		}
		m.saveProgress()
		return m, waitForLog(m.logCh)

	case hyvaVerifiedMsg:
		m.verifyingHyva = false
		if msg.err != nil {
			m.setupError = msg.err.Error()
			m.focusAbsolutePos(len(m.setupInputs) + toggleCount + hyvaAuthTokenField)
			return m, textinput.Blink
		}
		return m.showPreview()

	case installDoneMsg:
		m.installErr = msg.err
		if msg.err == nil {
			_ = resume.Clear(m.installCfg.Directory)
		}
		if msg.err != nil {
			for i := range m.installSteps {
				if m.installSteps[i].status == stepRunning {
					m.installSteps[i].status = stepFailed
					m.installSteps[i].finishedAt = now()
					break
				}
			}
			m.phase = phaseInstallDone
		} else {
			m.browserOpened = false
			m.phase = phaseOpenBrowser
		}
		return m, nil
	}

	// Handle name-input phase
	if m.phase == phaseNameInput {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				return m, m.enterDirectoryPhase()
			default:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			}
		}
	}

	// Handle directory-input phase
	if m.phase == phaseDirectoryInput {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.earlierRun = unfinishedRun(m.dirInput.Value())
				// Installing over an existing directory rewrites its
				// environment config and copies Mage-OS into it, so a
				// directory with content gets a look before anything runs.
				// A directory holding an install we can pick up is the
				// exception: the resume prompt covers that.
				if m.earlierRun == nil {
					if m.dirContents = existingContents(m.dirInput.Value()); m.dirContents != "" {
						m.phase = phaseDirectoryConfirm
						return m, nil
					}
				}
				return m, m.continueAfterDirectory()
			default:
				var cmd tea.Cmd
				m.dirInput, cmd = m.dirInput.Update(msg)
				return m, cmd
			}
		}
	}

	if m.phase == phaseDirectoryConfirm {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y":
				return m, m.continueAfterDirectory()
			case "n", "esc", "backspace":
				m.phase = phaseDirectoryInput
				return m, textinput.Blink
			}
		}
	}

	if m.phase == phaseChecksReview {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "r":
				m.checksDone = false
				m.phase = phaseChecking
				return m, runPrerequisiteChecks
			case "enter":
				if prereq.AnyFailed(m.checks) {
					return m, nil
				}
				return m, m.continueToEnvironment()
			}
		}
	}

	if m.phase == phaseResumePrompt {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y", "enter":
				m.resumeEarlierRun()
				return m, textinput.Blink
			case "n":
				_ = resume.Clear(m.dirInput.Value())
				m.earlierRun = nil
				m.initSetupInputs()
				m.phase = phaseSetupConfig
				return m, textinput.Blink
			case "q", "esc":
				return m, tea.Quit
			}
		}
	}

	// Handle selection-phase keys
	if m.phase == phaseSelection {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.envs)-1 {
					m.cursor++
				}
			case "enter":
				m.selected = &m.envs[m.cursor]
				m.initSetupInputs()
				m.phase = phaseSetupConfig
				return m, textinput.Blink
			}
		}
	}

	// Handle setup config form
	if m.phase == phaseSetupConfig {
		if _, isKey := msg.(tea.KeyMsg); isKey && m.verifyingHyva {
			return m, nil
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// Total fields: admin fields + toggles + (hyva fields if enabled)
			totalFields := len(m.setupInputs) + toggleCount
			if m.installHyva {
				totalFields += len(m.hyvaInputs)
			}

			// Calculate current absolute position
			absPos := m.setupFocus
			if m.inTogglePhase {
				if m.toggleFocus < 0 {
					absPos = len(m.setupInputs) + (m.toggleFocus - sampleDataToggle)
				} else {
					absPos = len(m.setupInputs) + toggleCount + m.toggleFocus
				}
			}

			switch keyMsg.String() {
			case revealPasswordKey:
				m.togglePasswordReveal()
				return m, nil
			case "tab", "down":
				newPos := absPos + 1
				if newPos >= totalFields {
					newPos = 0
				}
				m.focusAbsolutePos(newPos)
				return m, textinput.Blink
			case "shift+tab", "up":
				newPos := absPos - 1
				if newPos < 0 {
					newPos = totalFields - 1
				}
				m.focusAbsolutePos(newPos)
				return m, textinput.Blink
			case "enter":
				// On toggles, Enter advances to the next field (use Space to toggle)
				if m.inTogglePhase && m.toggleFocus < 0 {
					// If this toggle is the last field, submit
					if absPos == totalFields-1 {
						// fall through to submit logic below
					} else {
						m.focusAbsolutePos(absPos + 1)
						return m, textinput.Blink
					}
				}
				// On the last field, submit
				if absPos == totalFields-1 {
					// Validate admin fields
					for i, f := range setupFieldDefs {
						if strings.TrimSpace(m.setupInputs[i].Value()) == "" {
							m.setupError = f.label + " is required"
							m.focusAbsolutePos(i)
							return m, textinput.Blink
						}
					}
					// Reject an admin password Mage-OS would reject too, while
					// the user can still fix it.
					if err := magento.ValidateAdminPassword(m.setupInputs[adminPasswordField].Value()); err != nil {
						m.setupError = err.Error()
						m.focusAbsolutePos(adminPasswordField)
						return m, textinput.Blink
					}
					if field, err := m.invalidStoreSetting(); err != nil {
						m.setupError = err.Error()
						m.focusAbsolutePos(field)
						return m, textinput.Blink
					}
					// Validate Hyva fields if enabled
					if m.installHyva {
						for i, f := range hyvaFieldDefs {
							if strings.TrimSpace(m.hyvaInputs[i].Value()) == "" {
								m.setupError = "Hyvä " + f.label + " is required"
								m.focusAbsolutePos(len(m.setupInputs) + toggleCount + i)
								return m, textinput.Blink
							}
						}
					}
					m.setupError = ""
					if m.installHyva {
						return m.verifyHyva()
					}
					return m.showPreview()
				}
				// Move to next field
				m.focusAbsolutePos(absPos + 1)
				return m, textinput.Blink
			case " ":
				// Space on toggles
				if m.inTogglePhase {
					switch m.toggleFocus {
					case sampleDataToggle:
						m.installSampleData = !m.installSampleData
						return m, nil
					case initGitToggle:
						m.initGit = !m.initGit
						return m, nil
					case hyvaToggle:
						m.installHyva = !m.installHyva
						return m, nil
					}
				}
				// Fall through to default for text input
				fallthrough
			default:
				if m.inTogglePhase && m.toggleFocus >= 0 {
					var cmd tea.Cmd
					m.hyvaInputs[m.toggleFocus], cmd = m.hyvaInputs[m.toggleFocus].Update(msg)
					return m, cmd
				}
				if !m.inTogglePhase {
					var cmd tea.Cmd
					m.setupInputs[m.setupFocus], cmd = m.setupInputs[m.setupFocus].Update(msg)
					return m, cmd
				}
			}
		}
		// Forward non-key messages (e.g. blink) to focused input
		if m.inTogglePhase && m.toggleFocus >= 0 {
			var cmd tea.Cmd
			m.hyvaInputs[m.toggleFocus], cmd = m.hyvaInputs[m.toggleFocus].Update(msg)
			return m, cmd
		}
		if !m.inTogglePhase {
			var cmd tea.Cmd
			m.setupInputs[m.setupFocus], cmd = m.setupInputs[m.setupFocus].Update(msg)
			return m, cmd
		}
	}

	// Handle setup preview
	if m.phase == phaseSetupPreview {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "up", "k":
				if m.previewScroll > 0 {
					m.previewScroll--
				}
			case "down", "j":
				if maxScroll := len(m.previewLines()) - m.previewMaxVisible(); m.previewScroll < maxScroll {
					m.previewScroll++
				}
			case "enter":
				m.initInstallSteps()
				return m, tea.ExecProcess(sudoRefreshCommand(), func(err error) tea.Msg {
					return sudoCachedMsg{err: err}
				})
			case "b", "esc", "backspace":
				m.phase = phaseSetupConfig
				return m, textinput.Blink
			}
		}
	}

	if m.phase == phaseOpenBrowser {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y":
				url := m.selected.Detector.BaseURL(m.installCfg.ProjectName)
				_ = openBrowser(url)
				m.browserOpened = true
				m.phase = phaseInstallDone
				return m, nil
			case "n", "enter", "esc":
				m.phase = phaseInstallDone
				return m, nil
			}
		}
	}

	if m.phase == phaseInstallDone || m.phase == phaseError {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "r":
				if m.phase == phaseInstallDone && m.installErr != nil {
					// Find the failed step so we can resume from there.
					failedIdx := 0
					for i := range m.installSteps {
						if m.installSteps[i].status == stepFailed {
							failedIdx = i
							break
						}
					}
					// Keep completed steps green; reset failed + pending to pending.
					for i := range m.installSteps {
						if m.installSteps[i].status != stepDone {
							m.installSteps[i].status = stepPending
						}
					}
					m.logLines = nil
					m.installErr = nil
					m.installCfg.StartFromStep = failedIdx
					m.phase = phaseInstalling
					m.installStart = now()
					ch, cmd := runInstall(m.selected.Detector, m.installCfg)
					m.logCh = ch
					return m, tea.Batch(m.spinner.Tick, cmd)
				}
			case viewLogKey:
				if m.phase == phaseInstallDone {
					m.logScroll = m.logViewBottom()
					m.phase = phaseLogView
					return m, nil
				}
			case "enter", "q", "esc":
				return m, tea.Quit
			}
		}
	}

	if m.phase == phaseLogView {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "up", "k":
				m.logScroll--
			case "down", "j":
				m.logScroll++
			case "pgup":
				m.logScroll -= m.logViewRows()
			case "pgdown", " ":
				m.logScroll += m.logViewRows()
			case "g", "home":
				m.logScroll = 0
			case "G", "end":
				m.logScroll = m.logViewBottom()
			case "esc", "q", viewLogKey:
				m.phase = phaseInstallDone
				return m, nil
			}
			m.logScroll = max(0, min(m.logScroll, m.logViewBottom()))
		}
	}

	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(bannerStyle.Render(banner))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Mage-OS Installer"))
	b.WriteString("\n\n")

	switch m.phase {
	case phaseNameInput:
		b.WriteString("Project name:\n\n")
		b.WriteString(m.nameInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Enter to confirm, ctrl+c to quit"))

	case phaseDirectoryInput:
		b.WriteString("Install directory:\n\n")
		b.WriteString(m.dirInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Enter to confirm, ctrl+c to quit"))

	case phaseDirectoryConfirm:
		b.WriteString("Install directory:\n\n")
		b.WriteString(m.dirInput.View())
		b.WriteString("\n\n")
		b.WriteString(highlightStyle.Render("⚠ This directory is not empty: " + m.dirContents + "."))
		b.WriteString("\n")
		b.WriteString(highlightStyle.Render("  The installer copies Mage-OS over it, rewrites its environment config,"))
		b.WriteString("\n")
		b.WriteString(highlightStyle.Render("  and removes a leftover app/etc/env.php. Install here anyway?"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("y to continue · n to choose another directory · ctrl+c to quit"))

	case phaseChecking:
		b.WriteString(fmt.Sprintf("%s Checking prerequisites...\n", m.spinner.View()))

	case phaseChecksReview:
		b.WriteString(m.checksReviewView())

	case phaseDetecting:
		b.WriteString(fmt.Sprintf("%s Detecting development environments...\n", m.spinner.View()))

	case phaseSelection:
		b.WriteString("Multiple environments detected. Please select one:\n\n")
		for i, env := range m.envs {
			label := fmt.Sprintf("%s (%s)", env.Env.Name, env.Env.Version)
			if i == m.cursor {
				b.WriteString(selectedItemStyle.Render("▸ " + label))
			} else {
				b.WriteString(itemStyle.Render("  " + label))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to move, enter to select, q to quit"))

	case phaseResumePrompt:
		run := m.earlierRun
		lines := []string{
			highlightStyle.Render(fmt.Sprintf("An earlier install of %s in this directory did not finish.", run.ProjectName)),
			"",
			fmt.Sprintf("  %d of %d steps were done; the next one is %s.", len(run.Completed), len(run.Steps), highlightStyle.Render(run.NextStep())),
			"",
			"Resume from there? Your settings are kept; the admin password is asked for again.",
			"",
			dimStyle.Render("y to resume · n to start over · q to quit"),
		}
		b.WriteString(boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
		b.WriteString("\n")

	case phaseSetupConfig:
		b.WriteString("Configure Mage-OS:\n\n")
		const labelWidth = 18
		for i, f := range setupFieldDefs {
			b.WriteString(fmt.Sprintf("  %-*s  ", labelWidth, f.label))
			b.WriteString(m.setupInputs[i].View())
			b.WriteString("\n")
			if i == adminPasswordField {
				for _, hint := range m.passwordHints() {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  %-*s  %s", labelWidth, "", hint)))
					b.WriteString("\n")
				}
			}
			if i == adminLastnameField {
				b.WriteString("\n")
			}
			if i == currencyField {
				b.WriteString(dimStyle.Render(fmt.Sprintf("  %-*s  %s",
					labelWidth, "", "detected from this machine; the store's language, clock and currency")))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		// Sample data toggle
		sdCheckbox := "[ ]"
		if m.installSampleData {
			sdCheckbox = "[x]"
		}
		sdStyle := dimStyle
		if m.inTogglePhase && m.toggleFocus == sampleDataToggle {
			sdStyle = selectedItemStyle
		}
		b.WriteString(sdStyle.Render(fmt.Sprintf("  %-*s  %s", labelWidth, "Install sample data", sdCheckbox)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      Adds demo products, categories, and customers so you can"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      explore a fully populated store right away. Recommended"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      for your first install to see how everything works."))
		b.WriteString("\n\n")
		// Git toggle
		gitCheckbox := "[ ]"
		if m.initGit {
			gitCheckbox = "[x]"
		}
		gitStyle := dimStyle
		if m.inTogglePhase && m.toggleFocus == initGitToggle {
			gitStyle = selectedItemStyle
		}
		b.WriteString(gitStyle.Render(fmt.Sprintf("  %-*s  %s", labelWidth, "Initialize Git", gitCheckbox)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      Runs git init and writes a .gitignore for Mage-OS, so the"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      project is ready to commit. An existing repository or"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      .gitignore is left alone."))
		b.WriteString("\n\n")
		// Hyva toggle
		hyvaCheckbox := "[ ]"
		if m.installHyva {
			hyvaCheckbox = "[x]"
		}
		hyvaStyle := dimStyle
		if m.inTogglePhase && m.toggleFocus == hyvaToggle {
			hyvaStyle = selectedItemStyle
		}
		b.WriteString(hyvaStyle.Render(fmt.Sprintf("  %-*s  %s", labelWidth, "Install Hyvä", hyvaCheckbox)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      Hyvä is a modern, fast frontend theme that replaces the"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      default Luma theme. It uses Tailwind CSS and Alpine.js,"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      making frontend development much more enjoyable."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      ⚠ Hyvä requires a (free) license. Register at hyva.io and have"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("                      your repo URL and auth token ready before continuing."))
		b.WriteString("\n")
		// Hyva credential fields (shown only when enabled)
		if m.installHyva {
			for i, f := range hyvaFieldDefs {
				b.WriteString(fmt.Sprintf("  %-*s  ", labelWidth, "    "+f.label))
				b.WriteString(m.hyvaInputs[i].View())
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		if m.verifyingHyva {
			b.WriteString(fmt.Sprintf("  %s Checking the Hyvä credentials...\n\n", m.spinner.View()))
		}
		if m.setupError != "" {
			b.WriteString(errorStyle.Render("  ✗ "+m.setupError) + "\n\n")
		}
		b.WriteString(dimStyle.Render("Tab/↑↓/Enter to move · Space to toggle options · " + revealPasswordKey + " to show/hide password · Enter to review command · ctrl+c to quit"))

	case phaseSetupPreview:
		b.WriteString("Review what will happen:\n\n")
		lines := m.previewLines()
		maxVisible := m.previewMaxVisible()
		start := m.previewScroll
		if start >= len(lines) {
			start = max(0, len(lines)-1)
		}
		end := start + maxVisible
		if end > len(lines) {
			end = len(lines)
		}
		for _, line := range lines[start:end] {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
		for _, line := range sudoWarningLines(m.selected.Env.Name) {
			b.WriteString(highlightStyle.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to scroll · Enter to run · Backspace to go back"))

	case phaseInstalling:
		b.WriteString(fmt.Sprintf("%s Installing %s... %s\n", m.spinner.View(), m.selected.Env.Name,
			dimStyle.Render(formatElapsed(now().Sub(m.installStart))+" elapsed")))
		b.WriteString("\n")
		for _, step := range m.installSteps {
			b.WriteString(m.stepLine(step))
			b.WriteString("\n")
		}
		if len(m.logLines) > 0 {
			// Compute available height for the log box.
			// Header: 8 (banner) + 1 (blank) + 1 (title) + 2 (blank) +
			//         1 (installing line) + 1 (blank) + N (steps) = 14 + N
			// Box border adds 2 rows; log box also has MarginTop(1).
			wh := m.windowHeight
			if wh <= 0 {
				wh = 24
			}
			headerLines := 14 + len(m.installSteps)
			// 2 for box borders, 1 for margin top
			innerHeight := wh - headerLines - 3
			if innerHeight < 3 {
				innerHeight = 3
			}

			ww := m.windowWidth
			if ww <= 0 {
				ww = 80
			}
			// 4 accounts for the border (2) + padding (2)
			innerWidth := ww - 4
			if innerWidth < 40 {
				innerWidth = 40
			}

			start := len(m.logLines) - innerHeight
			if start < 0 {
				start = 0
			}
			style := logBoxStyle.Width(innerWidth).Height(innerHeight)
			b.WriteString(style.Render(strings.Join(m.logLines[start:], "\n")))
			b.WriteString("\n")
		}

	case phaseOpenBrowser:
		lines := append(m.successSummaryLines(),
			"",
			"Open the storefront in your browser?",
			"",
			dimStyle.Render("y to open · n/enter to skip"),
		)
		b.WriteString(boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
		b.WriteString("\n")

	case phaseInstallDone:
		if m.installErr != nil {
			width := m.errorLineWidth()
			lines := []string{
				errorStyle.Render("✗ Installation failed!"),
				"",
				wrapLine(m.installErr.Error(), width),
			}
			if summary := installErrorSummary(m.logLines); len(summary) > 0 {
				lines = append(lines, "", dimStyle.Render("What went wrong:"))
				for _, line := range summary {
					lines = append(lines, wrapLine(line, width))
				}
			}
			lines = append(lines, "", dimStyle.Render("Full log: "+installLogPath(m.installCfg.Directory)))
			lines = append(lines, "", dimStyle.Render("Press r to retry, "+viewLogKey+" to view the full log, enter/q to exit."))
			b.WriteString(boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
		} else {
			lines := m.successSummaryLines()
			if m.browserOpened {
				lines = append(lines, "", "Opened the storefront in your browser.")
			}
			lines = append(lines, "", dimStyle.Render("Press enter to exit."))
			b.WriteString(boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
		}
		b.WriteString("\n")

	case phaseLogView:
		lines := m.logViewLines()
		start := max(0, min(m.logScroll, m.logViewBottom()))
		end := min(len(lines), start+m.logViewRows())
		b.WriteString(fmt.Sprintf("Install log · %s\n\n", installLogPath(m.installCfg.Directory)))
		for _, line := range lines[start:end] {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("lines %d-%d of %d · ↑/↓ PgUp/PgDn g/G to scroll · esc to go back",
			start+1, end, len(lines))))
		b.WriteString("\n")

	case phaseError:
		lines := []string{
			errorStyle.Render("No supported environments found!"),
			"",
			"Please install one of the following:",
			"",
		}
		for _, info := range detector.AllDetectorInfos() {
			lines = append(lines, "  "+selectedItemStyle.Render(info.Name)+"  "+info.InstallURL)
		}
		lines = append(lines, "", dimStyle.Render("Press q to quit."))
		b.WriteString(boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
		b.WriteString("\n")
	}

	return b.String()
}
