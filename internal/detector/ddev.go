package detector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// DdevDetector checks for the DDEV development environment.
type DdevDetector struct{}

func (d *DdevDetector) Info() DetectorInfo {
	return DetectorInfo{
		Name:       "DDEV",
		InstallURL: "https://ddev.readthedocs.io/en/stable/",
	}
}

func (d *DdevDetector) Steps() []Step {
	return []Step{
		{Name: "Configure DDEV project"},
		{Name: "Install OpenSearch addon"},
		{Name: "Install Redis addon"},
		{Name: "Install Cron addon"},
		{Name: "Install RabbitMQ addon"},
		{Name: "Start DDEV"},
		{Name: "Apply RabbitMQ config"},
		{Name: "Create Mage-OS project"},
		{Name: "Create composer home directory"},
		{Name: "Copy auth.json"},
		{Name: "Install Mage-OS"},
	}
}

func (d *DdevDetector) SetupCommandPrefix() string {
	return "ddev exec bin/magento setup:install"
}

func (d *DdevDetector) BaseURL(projectName string) string {
	return "https://" + projectName + ".test"
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
		{Flag: "--base-url", Value: "https://" + config.ProjectName + ".test"},
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
		{
			"ddev", "exec", "composer", "create-project",
			"--repository-url=https://repo.mage-os.org/",
			"mage-os/project-community-edition",
			".",
		},
		{"ddev", "exec", "mkdir", "-p", "/var/www/html/var/composer_home/"},
	}

	for i, args := range steps {
		if i < config.StartFromStep {
			continue
		}
		stepStart(config, i)
		logf(config, "▸ %s", strings.Join(args, " "))
		if err := runInDir(config.Directory, config.Log, args[0], args[1:]...); err != nil {
			return fmt.Errorf("step %q failed: %w", strings.Join(args, " "), err)
		}
		stepDone(config, i)
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

	return nil
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
		Services map[string]struct {
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
