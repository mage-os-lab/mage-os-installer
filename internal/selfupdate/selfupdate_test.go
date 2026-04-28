package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

// --- helpers -----------------------------------------------------------------

// newTestUpdater returns an Updater pointed at the given test server, with no-op
// Replace and Reexec so unit tests don't touch the filesystem or re-exec.
func newTestUpdater(server *httptest.Server) *Updater {
	return &Updater{
		APIURL:      server.URL + "/releases/latest",
		Client:      server.Client(),
		ReplaceFunc: func(dst, src string) error { return nil },
		ReexecFunc:  func(execPath string) error { return nil },
	}
}

// sha256hex returns the hex-encoded SHA-256 of data.
func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --- versionsEqual -----------------------------------------------------------

func TestVersionsEqual_SameWithV(t *testing.T) {
	if !versionsEqual("v1.2.3", "v1.2.3") {
		t.Fatal("expected v1.2.3 == v1.2.3")
	}
}

func TestVersionsEqual_WithAndWithoutV(t *testing.T) {
	if !versionsEqual("v1.2.3", "1.2.3") {
		t.Fatal("expected v1.2.3 == 1.2.3")
	}
}

func TestVersionsEqual_Different(t *testing.T) {
	if versionsEqual("v1.0.0", "v1.0.1") {
		t.Fatal("expected v1.0.0 != v1.0.1")
	}
}

func TestVersionsEqual_DevNeverEqualsRelease(t *testing.T) {
	if versionsEqual("dev", "v1.0.0") {
		t.Fatal("expected dev != v1.0.0")
	}
}

// --- platformAssetName -------------------------------------------------------

func TestPlatformAssetName_OmitsVersion(t *testing.T) {
	name := platformAssetName("v1.2.3")
	if strings.Contains(name, "1.2.3") {
		t.Errorf("asset name should not contain version, got %q", name)
	}
	if !strings.HasPrefix(name, "mage-os-install_") {
		t.Errorf("asset name should start with binary name, got %q", name)
	}
}

func TestPlatformAssetName_ContainsGOOS(t *testing.T) {
	name := platformAssetName("v1.0.0")
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("asset name %q should contain GOOS %q", name, runtime.GOOS)
	}
}

func TestPlatformAssetName_ContainsGOARCH(t *testing.T) {
	name := platformAssetName("v1.0.0")
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("asset name %q should contain GOARCH %q", name, runtime.GOARCH)
	}
}

func TestPlatformAssetName_WindowsHasExeExtension(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	name := platformAssetName("v1.0.0")
	if !strings.HasSuffix(name, ".exe") {
		t.Errorf("windows asset should end with .exe, got %q", name)
	}
}

func TestPlatformAssetName_NonWindowsNoExe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows test")
	}
	name := platformAssetName("v1.0.0")
	if strings.HasSuffix(name, ".exe") {
		t.Errorf("non-windows asset should not end with .exe, got %q", name)
	}
}

// --- filterSelfUpdateFlag ----------------------------------------------------

func TestFilterSelfUpdateFlag_RemovesLongFlag(t *testing.T) {
	result := filterSelfUpdateFlag([]string{"--self-update", "--other"})
	if len(result) != 1 || result[0] != "--other" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFilterSelfUpdateFlag_RemovesShortFlag(t *testing.T) {
	result := filterSelfUpdateFlag([]string{"-self-update"})
	if len(result) != 0 {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFilterSelfUpdateFlag_KeepsOtherArgs(t *testing.T) {
	result := filterSelfUpdateFlag([]string{"--foo", "--bar"})
	if len(result) != 2 {
		t.Errorf("unexpected result: %v", result)
	}
}

// --- API unreachable ---------------------------------------------------------

func TestRun_APIUnreachable(t *testing.T) {
	// Use a server that immediately closes the connection.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force connection close without writing a response.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", 500)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	u := newTestUpdater(srv)
	err := u.Run("v1.0.0")
	if err == nil {
		t.Fatal("expected error when API is unreachable, got nil")
	}
	if !strings.Contains(err.Error(), "failed to reach GitHub API") {
		t.Errorf("expected 'failed to reach GitHub API' in error, got: %v", err)
	}
}

// --- Already up to date ------------------------------------------------------

func TestRun_AlreadyUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := newTestUpdater(srv)

	// Capture stdout by redirecting.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := u.Run("v1.0.0")

	w.Close()
	os.Stdout = old

	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(output, "Already up to date") {
		t.Errorf("expected 'Already up to date' message, got: %q", output)
	}
	if !strings.Contains(output, "v1.0.0") {
		t.Errorf("expected version in message, got: %q", output)
	}
}

func TestRun_AlreadyUpToDate_DevVersion(t *testing.T) {
	// "dev" is never equal to a real release tag, so an update should proceed.
	binaryContent := []byte("binary-content")
	assetName := platformAssetName("v1.0.0")
	checksumContent := sha256hex(binaryContent) + "  " + assetName + "\n"

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Write(binaryContent)
		case "/checksums":
			w.Write([]byte(checksumContent))
		}
	}))
	defer assetSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"tag_name":"v1.0.0",
			"assets":[
				{"name":%q,"browser_download_url":"%s/binary"},
				{"name":"checksums.txt","browser_download_url":"%s/checksums"}
			]
		}`, assetName, assetSrv.URL, assetSrv.URL)
	}))
	defer apiSrv.Close()

	var replaced bool
	u := &Updater{
		APIURL:      apiSrv.URL + "/releases/latest",
		Client:      http.DefaultClient,
		ReplaceFunc: func(dst, src string) error { replaced = true; return nil },
		ReexecFunc:  func(execPath string) error { return nil },
	}

	err := u.Run("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !replaced {
		t.Error("expected binary to be replaced when running dev version")
	}
}

// --- Successful update -------------------------------------------------------

func TestRun_SuccessfulUpdate(t *testing.T) {
	binaryContent := []byte("new-binary-content")
	assetName := platformAssetName("v2.0.0")
	checksumContent := sha256hex(binaryContent) + "  " + assetName + "\n"

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Write(binaryContent)
		case "/checksums":
			w.Write([]byte(checksumContent))
		}
	}))
	defer assetSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"tag_name":"v2.0.0",
			"assets":[
				{"name":%q,"browser_download_url":"%s/binary"},
				{"name":"checksums.txt","browser_download_url":"%s/checksums"}
			]
		}`, assetName, assetSrv.URL, assetSrv.URL)
	}))
	defer apiSrv.Close()

	var replaced bool
	var reexeced bool
	u := &Updater{
		APIURL:      apiSrv.URL + "/releases/latest",
		Client:      http.DefaultClient,
		ReplaceFunc: func(dst, src string) error { replaced = true; return nil },
		ReexecFunc:  func(execPath string) error { reexeced = true; return nil },
	}

	err := u.Run("v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !replaced {
		t.Error("expected ReplaceFunc to be called")
	}
	if !reexeced {
		t.Error("expected ReexecFunc to be called after successful update")
	}
}

// --- Checksum mismatch -------------------------------------------------------

func TestRun_ChecksumMismatch(t *testing.T) {
	binaryContent := []byte("new-binary-content")
	assetName := platformAssetName("v2.0.0")
	// Intentionally wrong checksum.
	badChecksum := "0000000000000000000000000000000000000000000000000000000000000000"
	checksumContent := badChecksum + "  " + assetName + "\n"

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Write(binaryContent)
		case "/checksums":
			w.Write([]byte(checksumContent))
		}
	}))
	defer assetSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"tag_name":"v2.0.0",
			"assets":[
				{"name":%q,"browser_download_url":"%s/binary"},
				{"name":"checksums.txt","browser_download_url":"%s/checksums"}
			]
		}`, assetName, assetSrv.URL, assetSrv.URL)
	}))
	defer apiSrv.Close()

	var replaced bool
	u := &Updater{
		APIURL:      apiSrv.URL + "/releases/latest",
		Client:      http.DefaultClient,
		ReplaceFunc: func(dst, src string) error { replaced = true; return nil },
		ReexecFunc:  func(execPath string) error { return nil },
	}

	err := u.Run("v1.0.0")
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected 'checksum mismatch' in error, got: %v", err)
	}
	if replaced {
		t.Error("ReplaceFunc should NOT be called when checksum fails")
	}
}

// --- No asset for platform ---------------------------------------------------

func TestRun_NoAssetForPlatform(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a release with no assets matching the current platform.
		fmt.Fprintln(w, `{"tag_name":"v2.0.0","assets":[{"name":"wrong-os_amd64","browser_download_url":"http://example.com/wrong"}]}`)
	}))
	defer apiSrv.Close()

	u := newTestUpdater(apiSrv)
	err := u.Run("v1.0.0")
	if err == nil {
		t.Fatal("expected error for missing platform asset")
	}
	if !strings.Contains(err.Error(), "no binary asset found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- GitHub API non-200 response --------------------------------------------

func TestRun_APIReturnsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := newTestUpdater(srv)
	err := u.Run("v1.0.0")
	if err == nil {
		t.Fatal("expected error for non-200 API response")
	}
	if !strings.Contains(err.Error(), "failed to reach GitHub API") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- No checksums.txt in release ---------------------------------------------

func TestRun_NoChecksumFile(t *testing.T) {
	assetName := platformAssetName("v2.0.0")
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"tag_name":"v2.0.0",
			"assets":[
				{"name":%q,"browser_download_url":"http://example.com/binary"}
			]
		}`, assetName)
	}))
	defer apiSrv.Close()

	u := newTestUpdater(apiSrv)
	err := u.Run("v1.0.0")
	if err == nil {
		t.Fatal("expected error for missing checksums.txt")
	}
	if !strings.Contains(err.Error(), "no checksums.txt") {
		t.Errorf("unexpected error: %v", err)
	}
}
