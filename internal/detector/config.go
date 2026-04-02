package detector

// Config holds the options passed to an environment installer.
type Config struct {
	ProjectName string
	Directory   string
	// Log is called with each line of installer output as it arrives.
	// If nil, output is captured silently and included only in errors.
	Log func(string)
	// OnStepStart is called with the step index just before a step begins.
	OnStepStart func(index int)
	// OnStepDone is called with the step index after a step completes successfully.
	OnStepDone func(index int)
	// StartFromStep skips all steps with an index lower than this value.
	// Used to resume from the failed step on retry.
	StartFromStep int

	// Mage-OS admin credentials
	AdminUser      string
	AdminPassword  string
	AdminEmail     string
	AdminFirstname string
	AdminLastname  string

	// Sample data
	InstallSampleData bool

	// Hyvä theme installation
	InstallHyva   bool
	HyvaRepoURL   string
	HyvaAuthToken string
}
