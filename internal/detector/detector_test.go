package detector

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllDetectorInfos_HasEntries(t *testing.T) {
	if len(AllDetectorInfos()) == 0 {
		t.Error("expected at least one detector to be registered")
	}
}

func TestAllDetectorInfos_AllHaveNames(t *testing.T) {
	for _, info := range AllDetectorInfos() {
		if info.Name == "" {
			t.Error("a registered detector has an empty Name")
		}
	}
}

func TestAllDetectorInfos_AllHaveInstallURLs(t *testing.T) {
	for _, info := range AllDetectorInfos() {
		if info.InstallURL == "" {
			t.Errorf("detector %q has an empty InstallURL", info.Name)
		}
	}
}

func TestAllDetectorInfos_CountMatchesAllDetectors(t *testing.T) {
	if len(AllDetectorInfos()) != len(allDetectors()) {
		t.Errorf("AllDetectorInfos() returned %d entries but allDetectors() has %d",
			len(AllDetectorInfos()), len(allDetectors()))
	}
}

// TestDetectors_DdevAndWardenBothRegistered verifies that both DDEV and Warden
// detectors are checked (acceptance criterion: check ddev and warden on PATH).
func TestDetectors_DdevAndWardenBothRegistered(t *testing.T) {
	infos := AllDetectorInfos()
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = strings.ToLower(info.Name)
	}

	found := func(want string) bool {
		for _, n := range names {
			if strings.Contains(n, want) {
				return true
			}
		}
		return false
	}

	if !found("ddev") {
		t.Error("DDEV detector not registered")
	}
	if !found("warden") {
		t.Error("Warden detector not registered")
	}
}

// TestDetectors_AllHaveVersionInEnvironment verifies that Detect returns an
// Environment with a non-empty Name (acceptance criterion: show name and version).
// This test only runs if the binary is present on PATH.
func TestDetectors_EnvironmentHasName(t *testing.T) {
	for _, d := range allDetectors() {
		env, err := d.Detect()
		if err != nil {
			// Binary not on PATH — skip, this is expected in CI.
			continue
		}
		if env == nil {
			t.Errorf("detector %q returned nil environment without error", d.Info().Name)
			continue
		}
		if env.Name == "" {
			t.Errorf("detector %q returned environment with empty Name", d.Info().Name)
		}
	}
}

// --- US-010: DDEV environment setup ---

// TestDdevBuildSteps_BaseStepsPresent verifies that the required installation
// steps are present when no optional features are enabled (AC1-AC4).
func TestDdevBuildSteps_BaseStepsPresent(t *testing.T) {
	d := &DdevDetector{}
	d.buildSteps(nil)

	wantSubstrings := []string{
		"Configure DDEV project",
		"OpenSearch",
		"Redis",
		"Cron",
		"RabbitMQ",
		"Start DDEV",
		"Mage-OS",
		"Verify installation",
	}

	names := make([]string, len(d.steps))
	for i, s := range d.steps {
		names[i] = s.Name
	}

	for _, want := range wantSubstrings {
		found := false
		for _, name := range names {
			if strings.Contains(name, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a step containing %q, got steps: %v", want, names)
		}
	}
}

// TestDdevBuildSteps_VerifyIsLastStep checks that "Verify installation" is always
// the last step, regardless of optional features (AC6, AC7).
func TestDdevBuildSteps_VerifyIsLastStep(t *testing.T) {
	cases := []struct {
		name   string
		config *Config
	}{
		{"base", nil},
		{"with sample data", &Config{InstallSampleData: true}},
		{"with Hyva", &Config{InstallHyva: true}},
		{"with both", &Config{InstallSampleData: true, InstallHyva: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &DdevDetector{}
			d.buildSteps(tc.config)
			last := d.steps[len(d.steps)-1]
			if last.Name != "Verify installation" {
				t.Errorf("expected last step to be 'Verify installation', got %q", last.Name)
			}
		})
	}
}

// TestDdevSetupInstallFlags_DatabaseConfig verifies that setup:install includes
// database connection flags (AC5).
func TestDdevSetupInstallFlags_DatabaseConfig(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{ProjectName: "test-project", AdminUser: "admin", AdminPassword: "pw"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--db-host":     "db",
		"--db-name":     "db",
		"--db-user":     "db",
		"--db-password": "db",
	}

	assertFlags(t, flags, wantFlags, "database")
}

// TestDdevSetupInstallFlags_RedisCacheConfig verifies Redis cache flags (AC5).
func TestDdevSetupInstallFlags_RedisCacheConfig(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--cache-backend":            "redis",
		"--cache-backend-redis-server": "redis",
		"--page-cache":               "redis",
		"--page-cache-redis-server":  "redis",
	}

	assertFlags(t, flags, wantFlags, "Redis cache")
}

// TestDdevSetupInstallFlags_RedisSessionConfig verifies Redis session flags (AC5).
func TestDdevSetupInstallFlags_RedisSessionConfig(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--session-save":           "redis",
		"--session-save-redis-host": "redis",
		"--session-save-redis-port": "6379",
	}

	assertFlags(t, flags, wantFlags, "Redis session")
}

// TestDdevSetupInstallFlags_OpenSearchConfig verifies OpenSearch flags (AC5).
func TestDdevSetupInstallFlags_OpenSearchConfig(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--search-engine":   "opensearch",
		"--opensearch-host": "opensearch",
		"--opensearch-port": "9200",
	}

	assertFlags(t, flags, wantFlags, "OpenSearch")
}

// TestDdevSetupInstallFlags_RabbitMQConfig verifies RabbitMQ flags (AC5).
func TestDdevSetupInstallFlags_RabbitMQConfig(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--amqp-host": "rabbitmq",
		"--amqp-port": "5672",
		"--amqp-user": "rabbitmq",
	}

	assertFlags(t, flags, wantFlags, "RabbitMQ")
}

// TestDdevSetupInstallFlags_AdminCredentials verifies admin credential flags (AC5).
func TestDdevSetupInstallFlags_AdminCredentials(t *testing.T) {
	d := &DdevDetector{}
	cfg := &Config{
		ProjectName:    "test-project",
		AdminUser:      "myadmin",
		AdminPassword:  "secret123",
		AdminEmail:     "admin@example.com",
		AdminFirstname: "John",
		AdminLastname:  "Doe",
	}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--admin-user":      "myadmin",
		"--admin-password":  "secret123",
		"--admin-email":     "admin@example.com",
		"--admin-firstname": "John",
		"--admin-lastname":  "Doe",
	}

	assertFlags(t, flags, wantFlags, "admin credentials")
}

// TestDdevBaseURL_UsesProjectName verifies the default base URL uses the DDEV
// .ddev.site domain for the given project name (AC6).
func TestDdevBaseURL_UsesProjectName(t *testing.T) {
	d := &DdevDetector{}
	url := d.BaseURL("my-shop")
	if !strings.Contains(url, "my-shop") {
		t.Errorf("BaseURL(%q) = %q, expected to contain project name", "my-shop", url)
	}
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("BaseURL(%q) = %q, expected to start with https://", "my-shop", url)
	}
	if !strings.Contains(url, "ddev.site") {
		t.Errorf("BaseURL(%q) = %q, expected to contain 'ddev.site'", "my-shop", url)
	}
}

// TestVerifyInstallation_Success verifies that verifyInstallation passes when
// the server returns HTTP 200 with the x-dist: Mage-OS header (AC6, AC7).
func TestVerifyInstallation_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-dist", "Mage-OS")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := verifyInstallation(srv.URL, nil); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestVerifyInstallation_Non200 verifies that verifyInstallation fails when
// the server returns a non-200 status code (AC6).
func TestVerifyInstallation_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-dist", "Mage-OS")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := verifyInstallation(srv.URL, nil); err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}

// TestVerifyInstallation_MissingXDistHeader verifies that verifyInstallation fails
// when the x-dist: Mage-OS header is absent (AC7).
func TestVerifyInstallation_MissingXDistHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := verifyInstallation(srv.URL, nil); err == nil {
		t.Error("expected error for missing x-dist header, got nil")
	}
}

// TestVerifyInstallation_WrongXDistHeader verifies that verifyInstallation fails
// when x-dist does not contain 'Mage-OS' (AC7).
func TestVerifyInstallation_WrongXDistHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-dist", "Adobe Commerce")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := verifyInstallation(srv.URL, nil); err == nil {
		t.Error("expected error for wrong x-dist header, got nil")
	}
}

// --- US-011: Warden environment setup ---

// TestWardenBuildSteps_BaseStepsPresent verifies that the required installation
// steps are present when no optional features are enabled.
func TestWardenBuildSteps_BaseStepsPresent(t *testing.T) {
	d := &WardenDetector{}
	d.buildSteps(nil)

	wantSubstrings := []string{
		"Initialize Warden",
		"SSL",
		"Start",
		"auth.json",
		"Mage-OS",
		"Configure application",
		"Verify installation",
	}

	names := make([]string, len(d.steps))
	for i, s := range d.steps {
		names[i] = s.Name
	}

	for _, want := range wantSubstrings {
		found := false
		for _, name := range names {
			if strings.Contains(name, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a step containing %q, got steps: %v", want, names)
		}
	}
}

// TestWardenBuildSteps_VerifyIsLastStep checks that "Verify installation" is always
// the last step, regardless of optional features.
func TestWardenBuildSteps_VerifyIsLastStep(t *testing.T) {
	cases := []struct {
		name   string
		config *Config
	}{
		{"base", nil},
		{"with sample data", &Config{InstallSampleData: true}},
		{"with Hyva", &Config{InstallHyva: true}},
		{"with both", &Config{InstallSampleData: true, InstallHyva: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &WardenDetector{}
			d.buildSteps(tc.config)
			last := d.steps[len(d.steps)-1]
			if last.Name != "Verify installation" {
				t.Errorf("expected last step to be 'Verify installation', got %q", last.Name)
			}
		})
	}
}

// TestWardenBaseURL_UsesAppSubdomain verifies that the Warden base URL uses the
// app.<project>.test format (AC8).
func TestWardenBaseURL_UsesAppSubdomain(t *testing.T) {
	d := &WardenDetector{}
	url := d.BaseURL("my-shop")
	if !strings.Contains(url, "my-shop") {
		t.Errorf("BaseURL(%q) = %q, expected to contain project name", "my-shop", url)
	}
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("BaseURL(%q) = %q, expected to start with https://", "my-shop", url)
	}
	if !strings.Contains(url, "app.") {
		t.Errorf("BaseURL(%q) = %q, expected to contain 'app.' subdomain", "my-shop", url)
	}
	if !strings.HasSuffix(url, ".test") {
		t.Errorf("BaseURL(%q) = %q, expected to end with '.test'", "my-shop", url)
	}
}

// TestWardenSetupInstallFlags_BaseURL verifies that setup:install includes the
// base URL flag using the app.<project>.test domain (AC6).
func TestWardenSetupInstallFlags_BaseURL(t *testing.T) {
	d := &WardenDetector{}
	cfg := &Config{ProjectName: "my-shop"}
	flags := d.SetupInstallFlags(cfg)

	flagMap := make(map[string]string, len(flags))
	for _, f := range flags {
		flagMap[f.Flag] = f.Value
	}

	val, ok := flagMap["--base-url"]
	if !ok {
		t.Fatal("--base-url flag not found in SetupInstallFlags")
	}
	if !strings.Contains(val, "app.my-shop") {
		t.Errorf("--base-url = %q, expected to contain 'app.my-shop'", val)
	}
	if !strings.HasPrefix(val, "https://") {
		t.Errorf("--base-url = %q, expected to start with https://", val)
	}
}

// TestWardenSetupInstallFlags_DatabaseConfig verifies database connection flags.
func TestWardenSetupInstallFlags_DatabaseConfig(t *testing.T) {
	d := &WardenDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--db-host":     "db",
		"--db-name":     "magento",
		"--db-user":     "magento",
		"--db-password": "magento",
	}

	assertFlags(t, flags, wantFlags, "Warden database")
}

// TestWardenSetupInstallFlags_RedisCacheConfig verifies Redis cache flags.
func TestWardenSetupInstallFlags_RedisCacheConfig(t *testing.T) {
	d := &WardenDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--cache-backend":              "redis",
		"--cache-backend-redis-server": "redis",
		"--page-cache":                 "redis",
		"--page-cache-redis-server":    "redis",
	}

	assertFlags(t, flags, wantFlags, "Warden Redis cache")
}

// TestWardenSetupInstallFlags_OpenSearchConfig verifies OpenSearch flags.
func TestWardenSetupInstallFlags_OpenSearchConfig(t *testing.T) {
	d := &WardenDetector{}
	cfg := &Config{ProjectName: "test-project"}
	flags := d.SetupInstallFlags(cfg)

	wantFlags := map[string]string{
		"--search-engine":   "opensearch",
		"--opensearch-host": "opensearch",
		"--opensearch-port": "9200",
	}

	assertFlags(t, flags, wantFlags, "Warden OpenSearch")
}

// --- US-004: Sample data ---

// TestDdevBuildSteps_SampleDataStep verifies that the "Install sample data" step is
// added to the DDEV step list only when InstallSampleData is true.
func TestDdevBuildSteps_SampleDataStep(t *testing.T) {
	hasSampleDataStep := func(d *DdevDetector) bool {
		for _, s := range d.steps {
			if strings.Contains(strings.ToLower(s.Name), "sample") {
				return true
			}
		}
		return false
	}

	// Without sample data: step should NOT be present
	d := &DdevDetector{}
	d.buildSteps(&Config{InstallSampleData: false})
	if hasSampleDataStep(d) {
		t.Error("DDEV: expected no sample data step when InstallSampleData=false")
	}

	// With sample data: step SHOULD be present
	d = &DdevDetector{}
	d.buildSteps(&Config{InstallSampleData: true})
	if !hasSampleDataStep(d) {
		t.Error("DDEV: expected a sample data step when InstallSampleData=true")
	}
}

// TestWardenBuildSteps_SampleDataStep verifies that the "Install sample data" step is
// added to the Warden step list only when InstallSampleData is true.
func TestWardenBuildSteps_SampleDataStep(t *testing.T) {
	hasSampleDataStep := func(d *WardenDetector) bool {
		for _, s := range d.steps {
			if strings.Contains(strings.ToLower(s.Name), "sample") {
				return true
			}
		}
		return false
	}

	// Without sample data: step should NOT be present
	w := &WardenDetector{}
	w.buildSteps(&Config{InstallSampleData: false})
	if hasSampleDataStep(w) {
		t.Error("Warden: expected no sample data step when InstallSampleData=false")
	}

	// With sample data: step SHOULD be present
	w = &WardenDetector{}
	w.buildSteps(&Config{InstallSampleData: true})
	if !hasSampleDataStep(w) {
		t.Error("Warden: expected a sample data step when InstallSampleData=true")
	}
}

// --- US-005: Hyvä theme ---

// TestDdevBuildSteps_HyvaSteps verifies that the Hyvä installation steps are
// added to the DDEV step list only when InstallHyva is true (AC3).
func TestDdevBuildSteps_HyvaSteps(t *testing.T) {
	hasHyvaStep := func(d *DdevDetector) bool {
		for _, s := range d.steps {
			if strings.Contains(strings.ToLower(s.Name), "hyv") {
				return true
			}
		}
		return false
	}

	// Without Hyvä: steps should NOT be present
	d := &DdevDetector{}
	d.buildSteps(&Config{InstallHyva: false})
	if hasHyvaStep(d) {
		t.Error("DDEV: expected no Hyvä steps when InstallHyva=false")
	}

	// With Hyvä: steps SHOULD be present
	d = &DdevDetector{}
	d.buildSteps(&Config{InstallHyva: true})
	if !hasHyvaStep(d) {
		t.Error("DDEV: expected Hyvä steps when InstallHyva=true")
	}
}

// TestWardenBuildSteps_HyvaSteps verifies that the Hyvä installation steps are
// added to the Warden step list only when InstallHyva is true (AC3).
func TestWardenBuildSteps_HyvaSteps(t *testing.T) {
	hasHyvaStep := func(d *WardenDetector) bool {
		for _, s := range d.steps {
			if strings.Contains(strings.ToLower(s.Name), "hyv") {
				return true
			}
		}
		return false
	}

	// Without Hyvä: steps should NOT be present
	w := &WardenDetector{}
	w.buildSteps(&Config{InstallHyva: false})
	if hasHyvaStep(w) {
		t.Error("Warden: expected no Hyvä steps when InstallHyva=false")
	}

	// With Hyvä: steps SHOULD be present
	w = &WardenDetector{}
	w.buildSteps(&Config{InstallHyva: true})
	if !hasHyvaStep(w) {
		t.Error("Warden: expected Hyvä steps when InstallHyva=true")
	}
}

// assertFlags checks that all wantFlags are present in the flags slice.
func assertFlags(t *testing.T, flags []SetupFlag, wantFlags map[string]string, context string) {
	t.Helper()
	flagMap := make(map[string]string, len(flags))
	for _, f := range flags {
		flagMap[f.Flag] = f.Value
	}
	for flag, wantVal := range wantFlags {
		gotVal, ok := flagMap[flag]
		if !ok {
			t.Errorf("[%s] flag %q not found in SetupInstallFlags", context, flag)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("[%s] flag %q = %q, want %q", context, flag, gotVal, wantVal)
		}
	}
}
