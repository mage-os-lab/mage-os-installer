// Package hyva checks Hyvä's Private Packagist credentials the way composer
// will use them, so a typo in the URL or a wrong token fails on the setup form
// instead of ten minutes into the install.
package hyva

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenUser is the HTTP basic auth user name Private Packagist expects, with
// the token as the password. The installer writes the same pair into
// composer's auth.json.
const TokenUser = "token"

// packagesFile is what composer fetches first from any repository.
const packagesFile = "packages.json"

const timeout = 10 * time.Second

// ErrRejected means the repository answered, and said no to the token.
var ErrRejected = errors.New("Hyvä rejected the token: check the auth token and that it belongs to this repository")

// ValidateRepositoryURL checks the shape of the repository URL before anything
// is sent to it.
func ValidateRepositoryURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("Repo URL must be a full URL, such as https://hyva-themes.repo.packagist.com/your-company/")
	}
	if parsed.Scheme != "https" {
		return errors.New("Repo URL must use https")
	}
	return nil
}

// VerifyCredentials fetches the repository's packages.json with the token, as
// composer would, and reports a token the repository rejects or a host that
// cannot be reached.
func VerifyCredentials(ctx context.Context, repoURL, token string) error {
	if err := ValidateRepositoryURL(repoURL); err != nil {
		return err
	}
	return verifyAgainst(ctx, repoURL, token)
}

// verifyAgainst does the request itself, whatever the URL's scheme.
func verifyAgainst(ctx context.Context, repoURL, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packagesURL(repoURL), nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(TokenUser, strings.TrimSpace(token))

	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", hostOf(repoURL), err)
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return ErrRejected
	case response.StatusCode >= http.StatusBadRequest:
		return fmt.Errorf("%s answered HTTP %d for %s", hostOf(repoURL), response.StatusCode, packagesFile)
	}
	return nil
}

// packagesURL is the repository URL with packages.json appended.
func packagesURL(repoURL string) string {
	return strings.TrimRight(strings.TrimSpace(repoURL), "/") + "/" + packagesFile
}

// hostOf names the host for error messages.
func hostOf(repoURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return repoURL
	}
	return parsed.Host
}
