package options

import (
	"flag"
	"strings"
	"testing"
)

func parse(t *testing.T, args ...string) Options {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o := Bind(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return *o
}

func TestBind_StartsFromTheDefaults(t *testing.T) {
	o := parse(t)

	if o.AdminUser != "admin" || !o.InitGit || o.SampleData || o.Yes {
		t.Errorf("parse() = %+v, expected the defaults", o)
	}
}

func TestBind_ReadsEveryFlag(t *testing.T) {
	o := parse(t, "--project=shop", "--dir=/tmp/shop", "--env=warden",
		"--admin-user=michiel", "--admin-password=Wachtwoord123", "--admin-email=m@example.com",
		"--admin-firstname=Michiel", "--admin-lastname=G",
		"--sample-data", "--git=false", "--hyva", "--hyva-repo-url=https://hyva-themes.repo.packagist.com/acme/", "--hyva-auth-token=t0k3n",
		"--yes")

	if o.ProjectName != "shop" || o.Directory != "/tmp/shop" || o.Environment != "warden" {
		t.Errorf("project flags = %+v", o)
	}
	if o.AdminUser != "michiel" || o.AdminPassword != "Wachtwoord123" || o.AdminLastname != "G" {
		t.Errorf("admin flags = %+v", o)
	}
	if !o.SampleData || o.InitGit || !o.Hyva || o.HyvaAuthToken != "t0k3n" || !o.Yes {
		t.Errorf("toggles = %+v", o)
	}
}

func TestValidate_PassesACompleteSetOfAnswers(t *testing.T) {
	o := parse(t, "--project=shop", "--dir=/tmp/shop", "--admin-password=Wachtwoord123")

	if err := o.Validate(); err != nil {
		t.Errorf("Validate() = %v, expected nil", err)
	}
}

func TestValidate_ListsEverythingMissingAtOnce(t *testing.T) {
	err := parse(t, "--admin-password=short", "--env=docker", "--hyva").Validate()

	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"--project is required", "--dir is required", "--admin-password: ", "at least 12 characters", "--env must be ddev or warden", "--hyva-repo-url", "--hyva-auth-token is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() should mention %q, got:\n%s", want, err)
		}
	}
}

func TestConfig_CarriesEverythingTheDetectorNeeds(t *testing.T) {
	cfg := parse(t, "--project=shop", "--dir=/tmp/shop", "--admin-password=Wachtwoord123", "--sample-data", "--git=false").Config()

	if cfg.ProjectName != "shop" || cfg.Directory != "/tmp/shop" || cfg.AdminPassword != "Wachtwoord123" {
		t.Errorf("Config() = %+v", cfg)
	}
	if !cfg.InstallSampleData || cfg.InitGit {
		t.Errorf("Config() toggles = sample %v git %v", cfg.InstallSampleData, cfg.InitGit)
	}
}

func TestWantsEnvironment_IsCaseInsensitiveAndOpenByDefault(t *testing.T) {
	if !parse(t).WantsEnvironment("Warden") {
		t.Error("no --env should accept any environment")
	}
	if !parse(t, "--env=ddev").WantsEnvironment("DDEV") {
		t.Error("--env=ddev should accept DDEV")
	}
	if parse(t, "--env=ddev").WantsEnvironment("Warden") {
		t.Error("--env=ddev should refuse Warden")
	}
}

func TestWithDefaults_UsesTheWorkingDirectoryWhenNothingIsGiven(t *testing.T) {
	o := parse(t).WithDefaults("/home/me/shop")

	if o.ProjectName != "shop" || o.Directory != "/home/me/shop" {
		t.Errorf("WithDefaults() = %+v", o)
	}
}

func TestWithDefaults_MakesAFolderForANamedProject(t *testing.T) {
	o := parse(t, "--project=other").WithDefaults("/home/me/shop")

	if o.Directory != "/home/me/shop/other" {
		t.Errorf("Directory = %q, expected /home/me/shop/other", o.Directory)
	}
}

func TestWithDefaults_LeavesAnExplicitDirectoryAlone(t *testing.T) {
	o := parse(t, "--project=other", "--dir=/srv/x").WithDefaults("/home/me/shop")

	if o.Directory != "/srv/x" {
		t.Errorf("Directory = %q, expected /srv/x", o.Directory)
	}
}
