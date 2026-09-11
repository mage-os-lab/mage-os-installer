package detector

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitignore is Magento's own .gitignore with the two changes asked for in
// https://github.com/mage-os-lab/mage-os-installer/issues/1: /dev is ignored,
// and app/etc/config.php is tracked so every checkout knows which modules to
// enable.
const gitignore = `/.buildpath
/.cache
/.metadata
/.project
/.settings
/.vscode
atlassian*
/nbproject
/robots.txt
/pub/robots.txt
/sitemap
/sitemap.xml
/pub/sitemap
/pub/sitemap.xml
/.idea
/.gitattributes
/app/config_sandbox
/app/etc/env.php
/dev
/app/code/Magento/TestModule*
/lib/internal/flex/uploader/.actionScriptProperties
/lib/internal/flex/uploader/.flexProperties
/lib/internal/flex/uploader/.project
/lib/internal/flex/uploader/.settings
/lib/internal/flex/varien/.actionScriptProperties
/lib/internal/flex/varien/.flexLibProperties
/lib/internal/flex/varien/.project
/lib/internal/flex/varien/.settings
/node_modules
/.grunt
/Gruntfile.js
/package.json
/.php_cs
/.php_cs.cache
/.php-cs-fixer.php
/.php-cs-fixer.cache
/grunt-config.json
/pub/media/*.*
!/pub/media/.htaccess
/pub/media/attribute/*
!/pub/media/attribute/.htaccess
/pub/media/analytics/*
/pub/media/catalog/*
!/pub/media/catalog/.htaccess
/pub/media/customer/*
!/pub/media/customer/.htaccess
/pub/media/downloadable/*
!/pub/media/downloadable/.htaccess
/pub/media/favicon/*
/pub/media/import/*
!/pub/media/import/.htaccess
/pub/media/logo/*
/pub/media/custom_options/*
!/pub/media/custom_options/.htaccess
/pub/media/theme/*
/pub/media/theme_customization/*
!/pub/media/theme_customization/.htaccess
/pub/media/wysiwyg/*
!/pub/media/wysiwyg/.htaccess
/pub/media/tmp/*
!/pub/media/tmp/.htaccess
/pub/media/captcha/*
/pub/media/sitemap/*
!/pub/media/sitemap/.htaccess
/pub/static/*
!/pub/static/.htaccess

/var/*
!/var/.htaccess
/vendor/*
!/vendor/.htaccess
/generated/*
!/generated/.htaccess
.DS_Store
`

// initGitRepository turns the project directory into a Git repository and gives
// it a .gitignore. Neither half overwrites what is already there.
func initGitRepository(config *Config) error {
	if err := initGitDirectory(config); err != nil {
		return err
	}
	return writeGitignore(config)
}

// initGitDirectory runs git init, unless there is nothing to gain by it.
func initGitDirectory(config *Config) error {
	if _, err := exec.LookPath("git"); err != nil {
		logf(config, "⚠ Git is not installed, skipping git init")
		return nil
	}
	if isInsideGitWorkTree(config.Directory) {
		logf(config, "▸ Already inside a Git repository, skipping git init")
		return nil
	}

	logf(config, "▸ git init")
	return runInDir(config.Directory, config.Log, "git", "init")
}

// isInsideGitWorkTree reports whether dir already belongs to a repository, so
// git init does not nest a second one inside it.
func isInsideGitWorkTree(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// writeGitignore adds the .gitignore, leaving one that already exists alone.
func writeGitignore(config *Config) error {
	path := filepath.Join(config.Directory, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		logf(config, "▸ .gitignore already exists, leaving it untouched")
		return nil
	}

	logf(config, "▸ Writing .gitignore")
	if err := os.WriteFile(path, []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("could not write .gitignore: %w", err)
	}
	return nil
}
