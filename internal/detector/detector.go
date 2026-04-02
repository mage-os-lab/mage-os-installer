package detector

// Environment represents a detected development environment.
type Environment struct {
	Name    string
	Version string
	Command string
}

// DetectorInfo holds static metadata about a detector (used for install hints).
type DetectorInfo struct {
	Name       string
	InstallURL string
}

// Step represents a single named installation step.
type Step struct {
	Name string
}

// SetupFlag represents one --flag=value argument for bin/magento setup:install.
type SetupFlag struct {
	Flag     string
	Value    string
	Editable bool // shown highlighted in the command preview
}

// Detector checks for a specific development environment.
type Detector interface {
	Info() DetectorInfo
	Steps() []Step
	// PrepareSteps builds the step list based on the given config.
	// Must be called before Steps() to get accurate results.
	PrepareSteps(config *Config)
	Detect() (*Environment, error)
	Install(config *Config) error
	// SetupInstallFlags returns the ordered list of flags for bin/magento setup:install.
	SetupInstallFlags(config *Config) []SetupFlag
	// SetupCommandPrefix returns the command prefix shown in the setup preview
	// (e.g. "ddev exec bin/magento setup:install").
	SetupCommandPrefix() string
	// BaseURL returns the base URL for the installed Magento site.
	BaseURL(projectName string) string
}

func allDetectors() []Detector {
	return []Detector{
		&DdevDetector{},
		&WardenDetector{},
	}
}

// AllDetectorInfos returns static info for every known detector.
func AllDetectorInfos() []DetectorInfo {
	var infos []DetectorInfo
	for _, d := range allDetectors() {
		infos = append(infos, d.Info())
	}
	return infos
}

// DetectedEnvironment pairs a detected Environment with the Detector that found it.
type DetectedEnvironment struct {
	Env      Environment
	Detector Detector
}

// DetectAll runs all known detectors and returns available environments.
func DetectAll() []DetectedEnvironment {
	var results []DetectedEnvironment
	for _, d := range allDetectors() {
		env, err := d.Detect()
		if err == nil && env != nil {
			results = append(results, DetectedEnvironment{Env: *env, Detector: d})
		}
	}
	return results
}
