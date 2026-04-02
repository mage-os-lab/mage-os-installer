package detector

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WardenDetector checks for the Warden development environment.
type WardenDetector struct {
	steps []Step
}

func (d *WardenDetector) Info() DetectorInfo {
	return DetectorInfo{
		Name:       "Warden",
		InstallURL: "https://docs.warden.dev/",
	}
}

func (d *WardenDetector) Steps() []Step {
	return d.steps
}

func (d *WardenDetector) PrepareSteps(config *Config) {
	d.buildSteps(config)
}

func (d *WardenDetector) buildSteps(config *Config) {
	d.steps = []Step{
		{Name: "Initialize Warden environment"},
		{Name: "Sign SSL certificates"},
		{Name: "Start environment"},
		{Name: "Copy auth.json"},
		{Name: "Create Mage-OS project"},
		{Name: "Create composer home directory"},
		{Name: "Copy auth.json to composer home"},
		{Name: "Install Mage-OS"},
		{Name: "Configure application"},
		{Name: "Set developer mode"},
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
}

func (d *WardenDetector) SetupCommandPrefix() string {
	return "warden env exec php-fpm bin/magento setup:install"
}

func (d *WardenDetector) BaseURL(projectName string) string {
	return "https://app." + projectName + ".test"
}

func (d *WardenDetector) SetupInstallFlags(config *Config) []SetupFlag {
	return []SetupFlag{
		{Flag: "--backend-frontname", Value: "backend"},
		{Flag: "--amqp-host", Value: "rabbitmq"},
		{Flag: "--amqp-port", Value: "5672"},
		{Flag: "--amqp-user", Value: "guest"},
		{Flag: "--amqp-password", Value: "guest"},
		{Flag: "--db-host", Value: "db"},
		{Flag: "--db-name", Value: "magento"},
		{Flag: "--db-user", Value: "magento"},
		{Flag: "--db-password", Value: "magento"},
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

func (d *WardenDetector) Install(config *Config) error {
	d.buildSteps(config)

	run := func(args ...string) error {
		logf(config, "▸ %s", strings.Join(args, " "))
		return runInDir(config.Directory, config.Log, args[0], args[1:]...)
	}

	baseURL := d.BaseURL(config.ProjectName) + "/"

	allSteps := []func() error{
		// 0: Initialize Warden environment
		func() error {
			os.Remove(filepath.Join(config.Directory, ".env"))
			return run("warden", "env-init", config.ProjectName, "magento2")
		},
		// 1: Sign SSL certificates
		func() error {
			if err := run("warden", "sign-certificate", config.ProjectName+".test"); err != nil {
				return err
			}
			return run("warden", "sign-certificate", "app."+config.ProjectName+".test")
		},
		// 2: Start environment
		func() error {
			return run("warden", "env", "up")
		},
		// 3: Copy auth.json
		func() error {
			return wardenCopyAuthJSON(config)
		},
		// 4: Create Mage-OS project
		func() error {
			// Wait for php-fpm entrypoint to finish setting up composer
			if err := run(
				"warden", "env", "exec", "php-fpm",
				"bash", "-c", "while ! command -v composer &>/dev/null; do sleep 1; done",
			); err != nil {
				return err
			}
			if err := run(
				"warden", "env", "exec", "php-fpm",
				"composer", "create-project",
				"--repository-url=https://repo.mage-os.org/",
				"mage-os/project-community-edition",
				"/tmp/mage-os-project",
			); err != nil {
				return err
			}
			return run(
				"warden", "env", "exec", "php-fpm",
				"bash", "-c", "cp -a /tmp/mage-os-project/. /var/www/html/",
			)
		},
		// 5: Create composer home directory
		func() error {
			return run("warden", "env", "exec", "php-fpm",
				"mkdir", "-p", "/var/www/html/var/composer_home")
		},
		// 6: Copy auth.json to composer home
		func() error {
			return wardenCopyAuthJSONComposerHome(config)
		},
		// 7: Install Mage-OS
		func() error {
			logf(config, "▸ bin/magento setup:install")
			args := []string{"env", "exec", "php-fpm", "bin/magento", "setup:install"}
			for _, f := range d.SetupInstallFlags(config) {
				args = append(args, f.Flag+"="+f.Value)
			}
			return runInDir(config.Directory, config.Log, "warden", args...)
		},
		// 8: Configure application
		func() error {
			configs := [][]string{
				{"config:set", "--lock-env", "web/unsecure/base_url", baseURL},
				{"config:set", "--lock-env", "web/secure/base_url", baseURL},
				{"config:set", "--lock-env", "web/secure/offloader_header", "X-Forwarded-Proto"},
				{"config:set", "--lock-env", "web/secure/use_in_frontend", "1"},
				{"config:set", "--lock-env", "web/secure/use_in_adminhtml", "1"},
				{"config:set", "--lock-env", "web/seo/use_rewrites", "1"},
				{"config:set", "--lock-env", "system/full_page_cache/caching_application", "2"},
				{"config:set", "--lock-env", "system/full_page_cache/ttl", "604800"},
				{"config:set", "--lock-env", "catalog/search/enable_eav_indexer", "1"},
				{"config:set", "--lock-env", "dev/static/sign", "0"},
			}
			for _, c := range configs {
				args := append([]string{"warden", "env", "exec", "php-fpm", "bin/magento"}, c...)
				if err := run(args...); err != nil {
					return err
				}
			}
			return nil
		},
		// 9: Set developer mode
		func() error {
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "deploy:mode:set", "-s", "developer"); err != nil {
				return err
			}
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "cache:disable", "block_html", "full_page"); err != nil {
				return err
			}
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "indexer:reindex"); err != nil {
				return err
			}
			return run("warden", "env", "exec", "php-fpm",
				"bin/magento", "cache:flush")
		},
	}

	if config.InstallSampleData {
		allSteps = append(allSteps, func() error {
			logf(config, "▸ Deploying sample data")
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "sampledata:deploy"); err != nil {
				return err
			}
			// If no Hyvä, run setup:upgrade + cache:flush now.
			// If Hyvä is also enabled, setup:upgrade runs in the Hyvä enable step.
			if !config.InstallHyva {
				if err := run("warden", "env", "exec", "php-fpm",
					"bin/magento", "setup:upgrade"); err != nil {
					return err
				}
				return run("warden", "env", "exec", "php-fpm",
					"bin/magento", "cache:flush")
			}
			return nil
		})
	}

	if config.InstallHyva {
		// Configure Hyvä repository
		allSteps = append(allSteps, func() error {
			logf(config, "▸ Configuring Hyvä Private Packagist repository")
			if err := run("warden", "env", "exec", "php-fpm",
				"composer", "config", "repositories.private-packagist",
				"composer", config.HyvaRepoURL); err != nil {
				return err
			}
			return run("warden", "env", "exec", "php-fpm",
				"composer", "config", "--auth",
				"http-basic."+extractHost(config.HyvaRepoURL),
				"token", config.HyvaAuthToken)
		})
		// Install Hyvä theme
		allSteps = append(allSteps, func() error {
			logf(config, "▸ Installing Hyvä theme")
			return run("warden", "env", "exec", "php-fpm",
				"composer", "require", "hyva-themes/magento2-default-theme")
		})
		// Enable Hyvä modules
		allSteps = append(allSteps, func() error {
			logf(config, "▸ Enabling Hyvä modules and setting up theme")
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "module:enable", "--all"); err != nil {
				return err
			}
			if err := run("warden", "env", "exec", "php-fpm",
				"bin/magento", "setup:upgrade"); err != nil {
				return err
			}
			// Look up the Hyvä theme ID and set it as default
			logf(config, "▸ Setting Hyvä as default storefront theme")
			if err := run("warden", "env", "exec", "php-fpm",
				"bash", "-c",
				`THEME_ID=$(mysql -h db -u magento -pmagento magento -N -e "SELECT theme_id FROM theme WHERE theme_path='Hyva/default'" 2>/dev/null) && bin/magento config:set design/theme/theme_id "$THEME_ID"`); err != nil {
				return err
			}
			return run("warden", "env", "exec", "php-fpm",
				"bin/magento", "cache:flush")
		})
	}

	for i, fn := range allSteps {
		if i < config.StartFromStep {
			continue
		}
		stepStart(config, i)
		if err := fn(); err != nil {
			return fmt.Errorf("step %q failed: %w", d.Steps()[i].Name, err)
		}
		stepDone(config, i)
	}

	return nil
}

func (d *WardenDetector) Detect() (*Environment, error) {
	path, err := exec.LookPath("warden")
	if err != nil {
		return nil, err
	}

	version := "unknown"
	out, err := exec.Command("warden", "version").Output()
	if err == nil {
		version = parseWardenVersion(string(out))
	}

	return &Environment{
		Name:    "Warden",
		Version: version,
		Command: path,
	}, nil
}

func parseWardenVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "arden") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if len(p) > 0 && (p[0] >= '0' && p[0] <= '9') {
					return strings.TrimRight(p, ",")
				}
			}
		}
	}
	return "unknown"
}

// wardenPhpFpmContainer returns the Docker container ID for the php-fpm service.
// It generates a docker-compose.yml from the Warden env config and queries Docker Compose.
func wardenPhpFpmContainer(dir string) (string, error) {
	// Generate docker-compose.yml from Warden config
	cmd := exec.Command("warden", "env", "config")
	cmd.Dir = dir
	composeYml, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("warden env config failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), composeYml, 0644); err != nil {
		return "", fmt.Errorf("could not write docker-compose.yml: %w", err)
	}

	// Get container ID
	cmd = exec.Command("docker", "compose", "ps", "-q", "php-fpm")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker compose ps failed: %w", err)
	}

	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		return "", fmt.Errorf("php-fpm container not found")
	}
	return containerID, nil
}

// wardenCopyAuthJSON copies ~/.composer/auth.json into the Warden php-fpm container.
// If the file does not exist it is silently skipped.
func wardenCopyAuthJSON(config *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	authSrc := filepath.Join(home, ".composer", "auth.json")
	if _, err := os.Stat(authSrc); os.IsNotExist(err) {
		return nil // nothing to copy
	}

	// Ensure target directory exists
	logf(config, "▸ creating /home/www-data/.composer/ in container")
	if err := runInDir(config.Directory, config.Log,
		"warden", "env", "exec", "php-fpm",
		"mkdir", "-p", "/home/www-data/.composer/"); err != nil {
		return fmt.Errorf("could not create .composer directory: %w", err)
	}

	containerID, err := wardenPhpFpmContainer(config.Directory)
	if err != nil {
		return fmt.Errorf("could not get php-fpm container: %w", err)
	}

	logf(config, "▸ docker cp auth.json → container")
	return runInDir(config.Directory, config.Log,
		"docker", "cp", authSrc, containerID+":/home/www-data/.composer/auth.json")
}

// wardenCopyAuthJSONComposerHome copies ~/.composer/auth.json to the Mage-OS
// var/composer_home directory inside the container and fixes ownership.
func wardenCopyAuthJSONComposerHome(config *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	authSrc := filepath.Join(home, ".composer", "auth.json")
	if _, err := os.Stat(authSrc); os.IsNotExist(err) {
		return nil // nothing to copy
	}

	containerID, err := wardenPhpFpmContainer(config.Directory)
	if err != nil {
		return fmt.Errorf("could not get php-fpm container: %w", err)
	}

	logf(config, "▸ docker cp auth.json → composer_home")
	if err := runInDir(config.Directory, config.Log,
		"docker", "cp", authSrc, containerID+":/var/www/html/var/composer_home/auth.json"); err != nil {
		return err
	}

	return runInDir(config.Directory, config.Log,
		"warden", "env", "exec", "--user", "root", "php-fpm",
		"chown", "www-data:www-data", "/var/www/html/var/composer_home/auth.json")
}
