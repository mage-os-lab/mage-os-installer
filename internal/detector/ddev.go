package detector

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DdevDetector checks for the DDEV development environment.
type DdevDetector struct {
	// primaryURL is populated after ddev start by querying ddev describe.
	primaryURL string
	// steps is the list of installation steps, built dynamically based on config.
	steps []Step
}

func (d *DdevDetector) Info() DetectorInfo {
	return DetectorInfo{
		Name:       "DDEV",
		InstallURL: "https://ddev.readthedocs.io/en/stable/",
	}
}

func (d *DdevDetector) Steps() []Step {
	return d.steps
}

func (d *DdevDetector) PrepareSteps(config *Config) {
	d.buildSteps(config)
}

func (d *DdevDetector) buildSteps(config *Config) {
	d.steps = []Step{
		{Name: "Configure DDEV project"},
		{Name: "Install OpenSearch addon"},
		{Name: "Install Redis addon"},
		{Name: "Install Cron addon"},
		{Name: "Install RabbitMQ addon"},
		{Name: "Start DDEV"},
		{Name: "Apply RabbitMQ config"},
		{Name: "Create Mage-OS project"},
		{Name: "Copy project files"},
		{Name: "Create composer home directory"},
		{Name: "Copy auth.json"},
		{Name: "Install Mage-OS"},
	}
	if config != nil && config.InstallSampleData {
		d.steps = append(d.steps, Step{Name: "Install sample data"})
	}
	if config != nil && config.InstallHyva {
		d.steps = append(d.steps,
			Step{Name: "Configure Hyvä repository"},
			Step{Name: "Install Hyvä theme"},
			Step{Name: "Enable Hyvä modules"},
		)
	}
	d.steps = append(d.steps, Step{Name: "Verify installation"})
}

func (d *DdevDetector) SetupCommandPrefix() string {
	return "ddev exec bin/magento setup:install"
}

func (d *DdevDetector) BaseURL(projectName string) string {
	if d.primaryURL != "" {
		return d.primaryURL
	}
	return "https://" + projectName + ".ddev.site"
}

// SetupInstallFlags returns the full ordered list of flags for bin/magento setup:install.
func (d *DdevDetector) SetupInstallFlags(config *Config) []SetupFlag {
	return []SetupFlag{
		{Flag: "--backend-frontname", Value: "admin"},
		{Flag: "--amqp-host", Value: "rabbitmq"},
		{Flag: "--amqp-port", Value: "5672"},
		{Flag: "--amqp-user", Value: "rabbitmq"},
		{Flag: "--amqp-password", Value: "rabbitmq"},
		{Flag: "--db-host", Value: "db"},
		{Flag: "--db-name", Value: "db"},
		{Flag: "--db-user", Value: "db"},
		{Flag: "--db-password", Value: "db"},
		{Flag: "--search-engine", Value: "opensearch"},
		{Flag: "--opensearch-host", Value: "opensearch"},
		{Flag: "--opensearch-port", Value: "9200"},
		{Flag: "--opensearch-index-prefix", Value: "magento2"},
		{Flag: "--opensearch-enable-auth", Value: "0"},
		{Flag: "--opensearch-timeout", Value: "15"},
		{Flag: "--http-cache-hosts", Value: "varnish:80"},
		{Flag: "--session-save", Value: "redis"},
		{Flag: "--session-save-redis-host", Value: "redis"},
		{Flag: "--session-save-redis-port", Value: "6379"},
		{Flag: "--session-save-redis-db", Value: "2"},
		{Flag: "--session-save-redis-max-concurrency", Value: "20"},
		{Flag: "--cache-backend", Value: "redis"},
		{Flag: "--cache-backend-redis-server", Value: "redis"},
		{Flag: "--cache-backend-redis-db", Value: "0"},
		{Flag: "--cache-backend-redis-port", Value: "6379"},
		{Flag: "--page-cache", Value: "redis"},
		{Flag: "--page-cache-redis-server", Value: "redis"},
		{Flag: "--page-cache-redis-db", Value: "1"},
		{Flag: "--page-cache-redis-port", Value: "6379"},
		{Flag: "--base-url", Value: d.BaseURL(config.ProjectName)},
		{Flag: "--timezone", Value: "Europe/Amsterdam"},
		{Flag: "--currency", Value: "EUR"},
		{Flag: "--admin-user", Value: config.AdminUser, Editable: true},
		{Flag: "--admin-password", Value: config.AdminPassword, Editable: true},
		{Flag: "--admin-email", Value: config.AdminEmail, Editable: true},
		{Flag: "--admin-firstname", Value: config.AdminFirstname, Editable: true},
		{Flag: "--admin-lastname", Value: config.AdminLastname, Editable: true},
		{Flag: "--use-rewrites", Value: "1"},
	}
}

func (d *DdevDetector) Install(config *Config) error {
	d.buildSteps(config)

	composerCreateProjectIdx := 7

	steps := [][]string{
		{
			"ddev", "config",
			"--project-name=" + config.ProjectName,
			"--docroot=pub",
			"--project-type=magento2",
			"--database=mysql:8.0",
			"--composer-version=2.2",
		},
		{"ddev", "add-on", "get", "ddev/ddev-opensearch"},
		{"ddev", "add-on", "get", "ddev/ddev-redis"},
		{"ddev", "add-on", "get", "ddev/ddev-cron"},
		{"ddev", "add-on", "get", "b13/ddev-rabbitmq"},
		{"ddev", "start"},
		{"ddev", "rabbitmq", "apply"},
		// 7: composer create-project — handled via runComposerCreateProject
		{
			"ddev", "exec", "composer", "create-project",
			"--repository-url=https://repo.mage-os.org/",
			"mage-os/project-community-edition",
			"/tmp/mage-os-project",
		},
		{
			"ddev", "exec", "bash", "-c",
			"cp -a /tmp/mage-os-project/. /var/www/html/",
		},
		{"ddev", "exec", "mkdir", "-p", "/var/www/html/var/composer_home/"},
	}

	for i, args := range steps {
		if i < config.StartFromStep {
			continue
		}
		stepStart(config, i)
		var err error
		if i == composerCreateProjectIdx {
			err = runComposerCreateProject(config, args[0], args[1:])
		} else {
			logf(config, "▸ %s", strings.Join(args, " "))
			err = runInDir(config.Directory, config.Log, args[0], args[1:]...)
		}
		if err != nil {
			return fmt.Errorf("step %q failed: %w", strings.Join(args, " "), err)
		}
		stepDone(config, i)
	}

	// After ddev start, query the actual primary URL so we use
	// the correct domain (e.g. .ddev.site) for Magento's base-url.
	if url, err := ddevPrimaryURL(config.Directory); err == nil {
		d.primaryURL = url
		logf(config, "▸ Detected primary URL: %s", url)
	}

	authIdx := len(steps)
	if authIdx >= config.StartFromStep {
		stepStart(config, authIdx)
		if err := copyAuthJSON(config); err != nil {
			return err
		}
		stepDone(config, authIdx)
	}

	magentoIdx := len(steps) + 1
	if magentoIdx >= config.StartFromStep {
		stepStart(config, magentoIdx)
		logf(config, "▸ ddev exec bin/magento setup:install")
		setupArgs := []string{"exec", "bin/magento", "setup:install"}
		for _, f := range d.SetupInstallFlags(config) {
			setupArgs = append(setupArgs, f.Flag+"="+f.Value)
		}
		if err := runInDir(config.Directory, config.Log, "ddev", setupArgs...); err != nil {
			return fmt.Errorf("setup:install failed: %w", err)
		}
		stepDone(config, magentoIdx)
	}

	nextIdx := magentoIdx + 1

	if config.InstallSampleData {
		sampleDataIdx := nextIdx
		if sampleDataIdx >= config.StartFromStep {
			stepStart(config, sampleDataIdx)
			logf(config, "▸ Deploying sample data")
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "bin/magento", "sampledata:deploy"); err != nil {
				return fmt.Errorf("sampledata:deploy failed: %w", err)
			}
			// If no Hyvä, run setup:upgrade + cache:flush now.
			// If Hyvä is also enabled, setup:upgrade runs in the Hyvä enable step.
			if !config.InstallHyva {
				if err := runInDir(config.Directory, config.Log,
					"ddev", "exec", "bin/magento", "setup:upgrade"); err != nil {
					return fmt.Errorf("setup:upgrade failed: %w", err)
				}
				if err := runInDir(config.Directory, config.Log,
					"ddev", "exec", "bin/magento", "cache:flush"); err != nil {
					return fmt.Errorf("cache:flush failed: %w", err)
				}
			}
			stepDone(config, sampleDataIdx)
		}
		nextIdx++
	}

	if config.InstallHyva {
		hyvaRepoIdx := nextIdx
		if hyvaRepoIdx >= config.StartFromStep {
			stepStart(config, hyvaRepoIdx)
			logf(config, "▸ Configuring Hyvä Private Packagist repository")
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "composer", "config", "repositories.private-packagist",
				"composer", config.HyvaRepoURL); err != nil {
				return fmt.Errorf("configure Hyvä repository failed: %w", err)
			}
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "composer", "config", "--auth",
				"http-basic."+extractHost(config.HyvaRepoURL),
				"token", config.HyvaAuthToken); err != nil {
				return fmt.Errorf("configure Hyvä auth failed: %w", err)
			}
			stepDone(config, hyvaRepoIdx)
		}
		nextIdx++

		hyvaInstallIdx := nextIdx
		if hyvaInstallIdx >= config.StartFromStep {
			stepStart(config, hyvaInstallIdx)
			logf(config, "▸ Installing Hyvä theme")
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "composer", "require", "hyva-themes/magento2-default-theme"); err != nil {
				return fmt.Errorf("install Hyvä theme failed: %w", err)
			}
			stepDone(config, hyvaInstallIdx)
		}
		nextIdx++

		hyvaEnableIdx := nextIdx
		if hyvaEnableIdx >= config.StartFromStep {
			stepStart(config, hyvaEnableIdx)
			logf(config, "▸ Enabling Hyvä modules and setting up theme")
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "bin/magento", "module:enable", "--all"); err != nil {
				return fmt.Errorf("enable Hyvä modules failed: %w", err)
			}
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "bin/magento", "setup:upgrade"); err != nil {
				return fmt.Errorf("setup:upgrade failed: %w", err)
			}
			// Look up the Hyvä theme ID and set it as default
			logf(config, "▸ Setting Hyvä as default storefront theme")
			themeID, err := ddevQueryThemeID(config.Directory, "Hyva/default")
			if err != nil {
				return fmt.Errorf("could not find Hyvä theme ID: %w", err)
			}
			logf(config, "▸ Hyvä theme ID: %s", themeID)
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "bin/magento", "config:set", "design/theme/theme_id", themeID); err != nil {
				return fmt.Errorf("set Hyvä theme failed: %w", err)
			}
			if err := runInDir(config.Directory, config.Log,
				"ddev", "exec", "bin/magento", "cache:flush"); err != nil {
				return fmt.Errorf("cache:flush failed: %w", err)
			}
			stepDone(config, hyvaEnableIdx)
		}
	}

	// verifyInstallation is always the last step regardless of optional steps.
	verifyIdx := len(d.steps) - 1
	if verifyIdx >= config.StartFromStep {
		stepStart(config, verifyIdx)
		baseURL := d.BaseURL(config.ProjectName)
		logf(config, "▸ Verifying store at %s", baseURL)
		if err := verifyInstallation(baseURL, config.Log); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
		stepDone(config, verifyIdx)
	}

	return nil
}

// verifyInstallation makes an HTTP GET request to baseURL and checks that the
// response returns HTTP 200 and includes the "x-dist: Mage-OS" header.
// DDEV uses self-signed TLS certificates, so TLS verification is skipped.
func verifyInstallation(baseURL string, logFn func(string)) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	resp, err := client.Get(baseURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("HTTP request to %s failed: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected HTTP 200 from %s, got %d", baseURL, resp.StatusCode)
	}

	dist := resp.Header.Get("x-dist")
	if !strings.Contains(dist, "Mage-OS") {
		return fmt.Errorf("expected x-dist header to contain 'Mage-OS', got %q", dist)
	}

	if logFn != nil {
		logFn(fmt.Sprintf("▸ Verified: %s → HTTP 200, x-dist: %s", baseURL, dist))
	}
	return nil
}

// extractHost extracts the hostname from a URL string.
func extractHost(rawURL string) string {
	// Strip protocol prefix
	host := rawURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	// Strip path
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	return host
}

// logf sends a formatted message to config.Log if it is set.
func logf(config *Config, format string, args ...any) {
	if config.Log != nil {
		config.Log(fmt.Sprintf(format, args...))
	}
}

func stepStart(config *Config, index int) {
	if config.OnStepStart != nil {
		config.OnStepStart(index)
	}
}

func stepDone(config *Config, index int) {
	if config.OnStepDone != nil {
		config.OnStepDone(index)
	}
}

// runInDir runs a command in dir, streaming each output line to logFn.
// If logFn is nil the output is captured silently and included only in errors.
func runInDir(dir string, logFn func(string), name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	if logFn == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			logFn(scanner.Text())
		}
	}()

	err := cmd.Wait()
	pw.Close()
	<-scanDone
	pr.Close()

	return err
}

// runComposerCreateProject runs a composer create-project command. If it fails,
// the command is retried with --no-audit in case the failure was caused by
// Composer's security advisory blocking (Composer 2.7+). If the retry also
// fails, the original error is returned.
func runComposerCreateProject(config *Config, name string, args []string) error {
	logf(config, "▸ %s %s", name, strings.Join(args, " "))
	err := runInDir(config.Directory, config.Log, name, args...)
	if err == nil {
		return nil
	}

	// The target directory is the last argument (e.g. /tmp/mage-os-project).
	// Clean it up before retrying so composer doesn't fail with "directory not empty".
	targetDir := args[len(args)-1]
	// Build a cleanup command using the same exec prefix (everything before "composer").
	var cleanupArgs []string
	for _, a := range args {
		if a == "composer" {
			break
		}
		cleanupArgs = append(cleanupArgs, a)
	}
	cleanupArgs = append(cleanupArgs, "rm", "-rf", targetDir)
	_ = runInDir(config.Directory, nil, name, cleanupArgs...)

	// Insert --no-audit right after "create-project" in the args.
	var retryArgs []string
	for i, a := range args {
		retryArgs = append(retryArgs, a)
		if a == "create-project" {
			retryArgs = append(retryArgs, "--no-audit")
			retryArgs = append(retryArgs, args[i+1:]...)
			break
		}
	}

	logf(config, "⚠ Retrying with --no-audit in case a security advisory is blocking installation")
	logf(config, "▸ %s %s", name, strings.Join(retryArgs, " "))
	if retryErr := runInDir(config.Directory, config.Log, name, retryArgs...); retryErr != nil {
		// Retry also failed; return the original error since it's more informative.
		return err
	}
	return nil
}

// copyAuthJSON copies ~/.composer/auth.json into the running DDEV web container.
// If the file does not exist it is silently skipped.
func copyAuthJSON(config *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	authSrc := home + "/.composer/auth.json"
	if _, err := os.Stat(authSrc); os.IsNotExist(err) {
		return nil // nothing to copy
	}

	containerName, err := ddevWebContainerName(config.Directory)
	if err != nil {
		return fmt.Errorf("could not get DDEV web container name: %w", err)
	}

	logf(config, "▸ docker cp auth.json → container")
	authDst := containerName + ":/var/www/html/var/composer_home/auth.json"
	return runInDir(config.Directory, config.Log, "docker", "cp", authSrc, authDst)
}

// ddevDescribeOutput is the relevant subset of `ddev describe --json-output`.
type ddevDescribeOutput struct {
	Raw struct {
		PrimaryURL string `json:"primary_url"`
		Services   map[string]struct {
			FullName string `json:"full_name"`
		} `json:"services"`
	} `json:"raw"`
}

// ddevWebContainerName returns the full Docker container name for the web service.
func ddevWebContainerName(dir string) (string, error) {
	cmd := exec.Command("ddev", "describe", "--json-output")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ddev describe failed: %w", err)
	}

	var desc ddevDescribeOutput
	if err := json.Unmarshal(out, &desc); err != nil {
		return "", fmt.Errorf("could not parse ddev describe output: %w", err)
	}

	web, ok := desc.Raw.Services["web"]
	if !ok || web.FullName == "" {
		return "", fmt.Errorf("web service not found in ddev describe output")
	}

	return web.FullName, nil
}

// ddevPrimaryURL returns the primary URL of the running ddev project.
func ddevPrimaryURL(dir string) (string, error) {
	cmd := exec.Command("ddev", "describe", "--json-output")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ddev describe failed: %w", err)
	}

	var desc ddevDescribeOutput
	if err := json.Unmarshal(out, &desc); err != nil {
		return "", fmt.Errorf("could not parse ddev describe output: %w", err)
	}

	if desc.Raw.PrimaryURL == "" {
		return "", fmt.Errorf("primary_url not found in ddev describe output")
	}

	return desc.Raw.PrimaryURL, nil
}

// ddevQueryThemeID queries the database via ddev mysql for the given theme path
// and returns its numeric ID as a string.
func ddevQueryThemeID(dir, themePath string) (string, error) {
	cmd := exec.Command("ddev", "mysql", "-N", "-e",
		fmt.Sprintf("SELECT theme_id FROM theme WHERE theme_path='%s'", themePath))
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ddev mysql query failed: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("theme %q not found in database", themePath)
	}
	return id, nil
}

func (d *DdevDetector) Detect() (*Environment, error) {
	path, err := exec.LookPath("ddev")
	if err != nil {
		return nil, err
	}

	version := "unknown"
	out, err := exec.Command("ddev", "version", "-j").Output()
	if err == nil {
		version = parseVersion(string(out), "ddev")
	}
	if version == "unknown" {
		out, err = exec.Command("ddev", "version").Output()
		if err == nil {
			version = parseDdevVersionPlain(string(out))
		}
	}

	return &Environment{
		Name:    "DDEV",
		Version: version,
		Command: path,
	}, nil
}

func parseDdevVersionPlain(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DDEV version") || strings.HasPrefix(line, "ddev version") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return strings.TrimRight(parts[len(parts)-1], ",")
			}
		}
		if strings.HasPrefix(line, "version:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return "unknown"
}
