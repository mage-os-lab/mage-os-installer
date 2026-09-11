package detector

import (
	"fmt"
	"os"
	"path/filepath"
)

// deploymentConfig marks a directory as already installed: Magento treats any
// install date in it as proof the store is there
// (Magento\Framework\App\DeploymentConfig::isAvailable). A setup:install that
// fails after writing it leaves the next attempt booting against a schema that
// does not exist yet, which dies with "Some commands failed to load. The
// default website isn't defined" instead of installing anything.
var deploymentConfig = filepath.Join("app", "etc", "env.php")

// staleInstallArtifacts are the files a failed setup:install leaves behind.
var staleInstallArtifacts = []string{
	deploymentConfig,
	filepath.Join("app", "etc", "config.php"),
	filepath.Join("var", ".maintenance.flag"),
}

// findStaleInstallArtifacts lists the leftovers of an earlier attempt in dir.
// Without a deployment config there is nothing stale to find: the Mage-OS
// project skeleton ships no env.php, so its presence means an earlier
// setup:install wrote it.
func findStaleInstallArtifacts(dir string) []string {
	if _, err := os.Stat(filepath.Join(dir, deploymentConfig)); err != nil {
		return nil
	}

	var found []string
	for _, artifact := range staleInstallArtifacts {
		if _, err := os.Stat(filepath.Join(dir, artifact)); err == nil {
			found = append(found, artifact)
		}
	}
	return found
}

// clearStaleInstallArtifacts removes those leftovers so setup:install starts
// from a clean slate, and says so rather than deleting anything quietly.
func clearStaleInstallArtifacts(config *Config) error {
	found := findStaleInstallArtifacts(config.Directory)
	if found == nil {
		return nil
	}

	logf(config, "⚠ Found %s from an earlier attempt: installing over it would fail, removing it", deploymentConfig)
	for _, artifact := range found {
		logf(config, "▸ Removing %s", artifact)
		if err := os.Remove(filepath.Join(config.Directory, artifact)); err != nil {
			return fmt.Errorf("could not remove %s: %w", artifact, err)
		}
	}
	return nil
}
