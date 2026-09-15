// Package options holds the answers to the installer's questions, whether
// they came from flags or from the screens, so a run can be scripted.
package options

import (
	"errors"
	"flag"
	"path/filepath"
	"strings"

	"github.com/mage-os/mage-os-install/internal/detector"
	"github.com/mage-os/mage-os-install/internal/magento"
)

// Options are the installer's inputs.
type Options struct {
	ProjectName string
	Directory   string
	Environment string // "ddev" or "warden"; empty picks the only one detected

	AdminUser      string
	AdminPassword  string
	AdminEmail     string
	AdminFirstname string
	AdminLastname  string

	SampleData    bool
	InitGit       bool
	Hyva          bool
	HyvaRepoURL   string
	HyvaAuthToken string

	// Yes runs the install without the TUI, taking every answer from the
	// flags. Anything missing is an error rather than a question.
	Yes bool
}

// Defaults are what the TUI pre-fills and what --yes falls back on.
var Defaults = Options{
	AdminUser:      "admin",
	AdminEmail:     "admin@example.com",
	AdminFirstname: "Admin",
	AdminLastname:  "User",
	InitGit:        true,
}

// Bind registers the installer's flags on fs. The returned Options are filled
// in once fs.Parse has run.
func Bind(fs *flag.FlagSet) *Options {
	o := Defaults
	fs.StringVar(&o.ProjectName, "project", "", "project name (default: the current directory's name)")
	fs.StringVar(&o.Directory, "dir", "", "install directory (default: the current directory, or ./<project> when --project is given)")
	fs.StringVar(&o.Environment, "env", "", "environment to use: ddev or warden (default: the only one installed)")
	fs.StringVar(&o.AdminUser, "admin-user", o.AdminUser, "admin user name")
	fs.StringVar(&o.AdminPassword, "admin-password", "", "admin password: at least 12 characters, with a letter and a digit")
	fs.StringVar(&o.AdminEmail, "admin-email", o.AdminEmail, "admin e-mail address")
	fs.StringVar(&o.AdminFirstname, "admin-firstname", o.AdminFirstname, "admin first name")
	fs.StringVar(&o.AdminLastname, "admin-lastname", o.AdminLastname, "admin last name")
	fs.BoolVar(&o.SampleData, "sample-data", o.SampleData, "install the sample data")
	fs.BoolVar(&o.InitGit, "git", o.InitGit, "run git init in the project directory")
	fs.BoolVar(&o.Hyva, "hyva", o.Hyva, "install the Hyvä theme (needs --hyva-repo-url and --hyva-auth-token)")
	fs.StringVar(&o.HyvaRepoURL, "hyva-repo-url", "", "Hyvä Private Packagist repository URL")
	fs.StringVar(&o.HyvaAuthToken, "hyva-auth-token", "", "Hyvä Private Packagist auth token")
	fs.BoolVar(&o.Yes, "yes", false, "run without the interface, taking every answer from the flags")
	return &o
}

// WithDefaults fills the project name and directory the way the screens
// would: the working directory's name, and the working directory itself, or
// a folder named after the project when a name was given.
func (o Options) WithDefaults(workingDirectory string) Options {
	if o.ProjectName == "" {
		o.ProjectName = filepath.Base(workingDirectory)
		if o.Directory == "" {
			o.Directory = workingDirectory
		}
	}
	if o.Directory == "" {
		o.Directory = filepath.Join(workingDirectory, o.ProjectName)
	}
	return o
}

// Validate checks everything a non-interactive run needs, since there is
// nobody to ask.
func (o Options) Validate() error {
	var problems []string
	for name, value := range map[string]string{
		"--project": o.ProjectName, "--dir": o.Directory, "--admin-user": o.AdminUser,
		"--admin-email": o.AdminEmail, "--admin-firstname": o.AdminFirstname, "--admin-lastname": o.AdminLastname,
	} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, name+" is required")
		}
	}
	if err := magento.ValidateAdminPassword(o.AdminPassword); err != nil {
		problems = append(problems, "--admin-password: "+err.Error())
	}
	if o.Hyva {
		if !strings.HasPrefix(o.HyvaRepoURL, "https://") {
			problems = append(problems, "--hyva-repo-url must be a full https URL")
		}
		if strings.TrimSpace(o.HyvaAuthToken) == "" {
			problems = append(problems, "--hyva-auth-token is required with --hyva")
		}
	}
	if env := strings.ToLower(o.Environment); env != "" && env != "ddev" && env != "warden" {
		problems = append(problems, "--env must be ddev or warden")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "\n"))
}

// Config turns the options into what a detector installs from.
func (o Options) Config() detector.Config {
	return detector.Config{
		ProjectName:       o.ProjectName,
		Directory:         o.Directory,
		AdminUser:         o.AdminUser,
		AdminPassword:     o.AdminPassword,
		AdminEmail:        o.AdminEmail,
		AdminFirstname:    o.AdminFirstname,
		AdminLastname:     o.AdminLastname,
		InstallSampleData: o.SampleData,
		InitGit:           o.InitGit,
		InstallHyva:       o.Hyva,
		HyvaRepoURL:       o.HyvaRepoURL,
		HyvaAuthToken:     o.HyvaAuthToken,
	}
}

// WantsEnvironment reports whether a detected environment is the one asked
// for; with no --env, any environment will do.
func (o Options) WantsEnvironment(name string) bool {
	return o.Environment == "" || strings.EqualFold(o.Environment, name)
}
