// Package selfupdate implements the --self-update flag logic.
// It checks the GitHub Releases API, downloads a newer binary if available,
// verifies its checksum, replaces the running executable, and re-execs.
package selfupdate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultAPIURL = "https://api.github.com/repos/mage-os/mage-os-install/releases/latest"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Updater performs self-updates. Its fields may be overridden for testing.
type Updater struct {
	// APIURL is the GitHub releases endpoint. Defaults to the mage-os/mage-os-install API.
	APIURL string
	// Client is the HTTP client used for all requests.
	Client *http.Client
	// ReplaceFunc replaces dst with the file at src. Defaults to replaceExecutable.
	ReplaceFunc func(dst, src string) error
	// ReexecFunc re-execs the binary at execPath with filtered args. Defaults to reexec.
	ReexecFunc func(execPath string) error
}

// New returns an Updater with default settings.
func New() *Updater {
	return &Updater{
		APIURL:      defaultAPIURL,
		Client:      http.DefaultClient,
		ReplaceFunc: replaceExecutable,
		ReexecFunc:  reexec,
	}
}

// Run checks the GitHub Releases API. If a newer version is available it
// downloads it, verifies the checksum, replaces the running binary, and
// re-execs. If already up-to-date it prints a message and returns nil.
func Run(currentVersion string) error {
	return New().Run(currentVersion)
}

// Run is the same as the package-level Run but uses the Updater's settings.
func (u *Updater) Run(currentVersion string) error {
	rel, err := u.fetchRelease()
	if err != nil {
		return fmt.Errorf("failed to reach GitHub API: %w", err)
	}

	latest := rel.TagName
	if versionsEqual(currentVersion, latest) {
		fmt.Printf("Already up to date: %s\n", latest)
		return nil
	}

	assetName := platformAssetName(latest)

	var binaryURL, checksumURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case assetName:
			binaryURL = a.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = a.BrowserDownloadURL
		}
	}

	if binaryURL == "" {
		return fmt.Errorf("no binary asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latest)
	}
	if checksumURL == "" {
		return fmt.Errorf("no checksums.txt found in release %s", latest)
	}

	// Download the new binary to a temp file.
	tmpBin, err := u.downloadToTemp(binaryURL)
	if err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}
	defer os.Remove(tmpBin)

	// Verify checksum before touching the existing binary.
	if err := u.verifyChecksum(checksumURL, tmpBin, assetName); err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	if err := u.ReplaceFunc(execPath, tmpBin); err != nil {
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	fmt.Printf("Updated to %s\n", latest)

	return u.ReexecFunc(execPath)
}

// fetchRelease downloads and decodes the latest GitHub release metadata.
func (u *Updater) fetchRelease() (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, u.APIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub API response: %w", err)
	}
	return &rel, nil
}

// versionsEqual compares two version strings, tolerating a leading "v".
func versionsEqual(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// platformAssetName returns the GoReleaser binary archive name for the current
// OS/arch. GoReleaser default template: {project}_{version}_{os}_{arch}[.exe]
// The version in the name strips the leading "v" (GoReleaser's {{ .Version }}).
func platformAssetName(tagName string) string {
	version := strings.TrimPrefix(tagName, "v")
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("mage-os-install_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

// downloadToTemp fetches url and writes the body to a new temporary file.
// The caller is responsible for removing the temp file.
func (u *Updater) downloadToTemp(url string) (string, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "mage-os-install-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// verifyChecksum downloads checksums.txt from checksumURL and checks that the
// SHA-256 of filePath matches the entry for assetName.
func (u *Updater) verifyChecksum(checksumURL, filePath, assetName string) error {
	resp, err := u.Client.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums.txt download returned HTTP %d", resp.StatusCode)
	}

	// Parse "sha256hash  filename" lines (sha256sum format).
	expected := ""
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 2 && parts[1] == assetName {
			expected = parts[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum found for %s in checksums.txt", assetName)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open downloaded file for verification: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to hash downloaded file: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expected, actual)
	}
	return nil
}

// replaceExecutable atomically replaces dst with the contents of src.
// src must be the path to the new binary.
func replaceExecutable(dst, src string) error {
	// Write to a temp file in the same directory so os.Rename is atomic.
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".mage-os-install-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Ensure temp file is cleaned up on failure.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()

	srcFile, err := os.Open(src)
	if err != nil {
		tmp.Close()
		return err
	}
	defer srcFile.Close()

	if _, err := io.Copy(tmp, srcFile); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	success = true
	return nil
}

// reexec replaces the current process with a fresh execution of execPath,
// passing through all original args except --self-update.
func reexec(execPath string) error {
	args := filterSelfUpdateFlag(os.Args[1:])
	cmd := exec.Command(execPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
	return nil // unreachable
}

// filterSelfUpdateFlag removes --self-update and -self-update from args.
func filterSelfUpdateFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a != "--self-update" && a != "-self-update" {
			filtered = append(filtered, a)
		}
	}
	return filtered
}
