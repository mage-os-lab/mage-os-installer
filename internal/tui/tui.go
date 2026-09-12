package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/magento"
)

type phase int

const (
	phaseNameInput phase = iota
	phaseDirectoryInput
	phaseDetecting
	phaseSelection
	phaseSetupConfig  // admin credentials form
	phaseSetupPreview // full command preview before running
	phaseInstalling
	phaseOpenBrowser // ask whether to open the site after install
	phaseInstallDone
	phaseError
)

// detectionDoneMsg is sent when environment detection completes.
type detectionDoneMsg struct {
	envs []detector.DetectedEnvironment
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
	name   string
	status stepStatus
}

// Model is the main bubbletea model for the installer TUI.
type Model struct {
	phase         phase
	spinner       spinner.Model
	nameInput     textinput.Model
	originalName  string // the default seeded into nameInput, used to compute the dir default
	dirInput      textinput.Model
	envs          []detector.DetectedEnvironment
	cursor        int
	selected      *detector.DetectedEnvironment
	setupInputs   []textinput.Model
	setupFocus    int
	setupError    string
	installCfg    detector.Config
	previewScroll int
	windowHeight  int
	windowWidth   int
	logCh         <-chan tea.Msg
	logLines      []string
	installSteps  []installStep
	installErr    error
	browserOpened bool

	// passwordRevealed shows the admin password in clear text on the form.
	passwordRevealed bool

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

// New creates the initial TUI model.
func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	defaultName := currentDirName()

	ti := textinput.New()
	ti.SetValue(defaultName)
	ti.Focus()
	ti.CharLimit = 64

	return Model{
		phase:        phaseNameInput,
		spinner:      s,
		nameInput:    ti,
		originalName: defaultName,
	}
}

func (m Model) Init() tea.Cmd {
	// Start detection in the background immediately so results are ready
	// by the time the user confirms the project name.
	return tea.Batch(m.spinner.Tick, detectEnvironments, textinput.Blink)
}

func detectEnvironments() tea.Msg {
	return detectionDoneMsg{envs: detector.DetectAll()}
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
	di.Focus()
	di.CharLimit = 256
	m.dirInput = di
	m.phase = phaseDirectoryInput
	return textinput.Blink
}

// initInstallSteps loads the step list from the selected detector.
func (m *Model) initInstallSteps() {
	m.installSteps = nil
	for _, s := range m.selected.Detector.Steps() {
		m.installSteps = append(m.installSteps, installStep{name: s.Name, status: stepPending})
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
)

var setupFieldDefaults = []string{"admin", "Admin123!Mage", "admin@example.com", "Admin", "User"}

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
		ti.SetValue(setupFieldDefaults[i])
		ti.EchoMode = f.echo
		ti.CharLimit = 128
		if i == 0 {
			ti.Focus()
		}
		m.setupInputs[i] = ti
	}
	m.setupFocus = 0

	// Initialize toggles and Hyvä inputs
	m.installSampleData = false
	m.initGit = true
	m.installHyva = false
	m.inTogglePhase = false
	m.toggleFocus = sampleDataToggle
	m.hyvaInputs = make([]textinput.Model, len(hyvaFieldDefs))
	for i, f := range hyvaFieldDefs {
		ti := textinput.New()
		ti.EchoMode = f.echo
		ti.CharLimit = 256
		m.hyvaInputs[i] = ti
	}
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
		InstallSampleData: m.installSampleData,
		InitGit:           m.initGit,
		InstallHyva:       m.installHyva,
	}
	if m.installHyva {
		cfg.HyvaRepoURL = m.hyvaInputs[0].Value()
		cfg.HyvaAuthToken = m.hyvaInputs[1].Value()
	}
	return cfg
}

// advanceFromDetection moves the model to the correct phase based on
// how many environments were detected. The caller must have already
// populated m.envs.
func (m *Model) advanceFromDetection() tea.Cmd {
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

func runInstall(d detector.Detector, cfg detector.Config) (<-chan tea.Msg, tea.Cmd) {
	ch := make(chan tea.Msg, 1000)

	go func() {
		if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
			ch <- installDoneMsg{err: fmt.Errorf("could not create directory %s: %w", cfg.Directory, err)}
			return
		}
		cfg.Log = func(line string) { ch <- logMsg(line) }
		cfg.OnStepStart = func(i int) { ch <- stepStartMsg{index: i} }
		cfg.OnStepDone = func(i int) { ch <- stepDoneMsg{index: i} }
		ch <- installDoneMsg{err: d.Install(&cfg)}
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
		return m, m.advanceFromDetection()

	case sudoCachedMsg:
		// sudo credentials are now cached (or failed); proceed with install.
		m.phase = phaseInstalling
		ch, cmd := runInstall(m.selected.Detector, m.installCfg)
		m.logCh = ch
		return m, tea.Batch(m.spinner.Tick, cmd)

	case logMsg:
		m.logLines = append(m.logLines, string(msg))
		return m, waitForLog(m.logCh)

	case stepStartMsg:
		if msg.index < len(m.installSteps) {
			m.installSteps[msg.index].status = stepRunning
		}
		return m, waitForLog(m.logCh)

	case stepDoneMsg:
		if msg.index < len(m.installSteps) {
			m.installSteps[msg.index].status = stepDone
		}
		return m, waitForLog(m.logCh)

	case installDoneMsg:
		m.installErr = msg.err
		if msg.err != nil {
			for i := range m.installSteps {
				if m.installSteps[i].status == stepRunning {
					m.installSteps[i].status = stepFailed
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
				// Detection may already be done (envs cached) or still running.
				if m.envs != nil {
					return m, m.advanceFromDetection()
				}
				m.phase = phaseDetecting
				return m, nil
			default:
				var cmd tea.Cmd
				m.dirInput, cmd = m.dirInput.Update(msg)
				return m, cmd
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
					m.installCfg = m.buildInstallConfig()
					m.selected.Detector.PrepareSteps(&m.installCfg)
					m.previewScroll = 0
					m.phase = phaseSetupPreview
					return m, nil
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
					ch, cmd := runInstall(m.selected.Detector, m.installCfg)
					m.logCh = ch
					return m, tea.Batch(m.spinner.Tick, cmd)
				}
			case "enter", "q", "esc":
				return m, tea.Quit
			}
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
		b.WriteString(fmt.Sprintf("%s Installing %s...\n", m.spinner.View(), m.selected.Env.Name))
		b.WriteString("\n")
		for _, step := range m.installSteps {
			switch step.status {
			case stepDone:
				b.WriteString(successStyle.Render("  ✓ " + step.name))
			case stepRunning:
				b.WriteString(selectedItemStyle.Render("  ▸ " + step.name + "..."))
			case stepFailed:
				b.WriteString(errorStyle.Render("  ✗ " + step.name))
			default:
				b.WriteString(dimStyle.Render("  • " + step.name))
			}
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
			lines = append(lines, "", dimStyle.Render("Press r to retry, enter/q to exit."))
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
