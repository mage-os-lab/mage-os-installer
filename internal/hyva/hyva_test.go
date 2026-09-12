package hyva

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateRepositoryURL_AcceptsAPackagistURL(t *testing.T) {
	if err := ValidateRepositoryURL("https://hyva-themes.repo.packagist.com/acme/"); err != nil {
		t.Errorf("expected a valid URL, got %v", err)
	}
}

func TestValidateRepositoryURL_RejectsThingsThatAreNotURLs(t *testing.T) {
	for _, raw := range []string{"", "acme", "hyva-themes.repo.packagist.com/acme", "ftp://x", "http://hyva-themes.repo.packagist.com/acme/"} {
		if err := ValidateRepositoryURL(raw); err == nil {
			t.Errorf("ValidateRepositoryURL(%q) should fail", raw)
		}
	}
}

// packagist fakes Private Packagist: packages.json for the right token,
// 401 for anything else.
func packagist(t *testing.T, token string) (*httptest.Server, *string) {
	t.Helper()
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		user, password, ok := r.BasicAuth()
		if !ok || user != TokenUser || password != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"packages":{}}`))
	}))
	t.Cleanup(server.Close)
	return server, &seenPath
}

func TestVerifyCredentials_ChecksTheURLBeforeSendingAnything(t *testing.T) {
	err := VerifyCredentials(context.Background(), "not a url", "s3cret")

	if err == nil || !strings.Contains(err.Error(), "Repo URL must be") {
		t.Errorf("expected the URL rule to fire first, got %v", err)
	}
}

func TestVerifyCredentials_FetchesPackagesJSONWithTheToken(t *testing.T) {
	server, seenPath := packagist(t, "s3cret")

	err := verifyAgainst(context.Background(), server.URL+"/acme/", "s3cret")

	if err != nil {
		t.Fatalf("expected the right token to be accepted, got %v", err)
	}
	if *seenPath != "/acme/packages.json" {
		t.Errorf("fetched %q, expected /acme/packages.json", *seenPath)
	}
}

func TestVerifyCredentials_ReportsARejectedToken(t *testing.T) {
	server, _ := packagist(t, "s3cret")

	err := verifyAgainst(context.Background(), server.URL+"/acme/", "wrong")

	if !errors.Is(err, ErrRejected) {
		t.Errorf("expected ErrRejected, got %v", err)
	}
}

func TestVerifyCredentials_NamesAHostItCannotReach(t *testing.T) {
	server, _ := packagist(t, "s3cret")
	url := server.URL
	server.Close()

	err := verifyAgainst(context.Background(), url+"/acme/", "s3cret")

	if err == nil || !strings.Contains(err.Error(), "could not reach 127.0.0.1") {
		t.Errorf("expected an error naming the host, got %v", err)
	}
}
