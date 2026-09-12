// Package locale works out sensible store defaults from the machine the
// installer runs on, and checks what the user makes of them.
package locale

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Defaults are the store settings a fresh install is offered.
type Defaults struct {
	Locale   string
	Timezone string
	Currency string
}

// Fallback is used for anything the machine does not reveal.
var Fallback = Defaults{Locale: "en_US", Timezone: "UTC", Currency: "EUR"}

// localtimeLink is where Unix systems point at the current zone.
const localtimeLink = "/etc/localtime"

var (
	localePattern   = regexp.MustCompile(`^[a-z]{2,3}_[A-Z]{2}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// currencyByCountry covers the countries a locale is likely to name.
var currencyByCountry = map[string]string{
	"US": "USD", "GB": "GBP", "CA": "CAD", "AU": "AUD", "NZ": "NZD", "JP": "JPY",
	"CH": "CHF", "SE": "SEK", "NO": "NOK", "DK": "DKK", "PL": "PLN", "CZ": "CZK",
	"HU": "HUF", "IN": "INR", "BR": "BRL", "MX": "MXN", "ZA": "ZAR", "TR": "TRY",
	"NL": "EUR", "DE": "EUR", "FR": "EUR", "BE": "EUR", "AT": "EUR", "ES": "EUR",
	"IT": "EUR", "IE": "EUR", "FI": "EUR", "PT": "EUR", "GR": "EUR", "LU": "EUR",
}

// Detect reads the environment and the system zone.
func Detect() Defaults {
	return DetectFrom(os.Getenv, localtimeLink)
}

// DetectFrom is Detect with its inputs handed in, for tests.
func DetectFrom(getenv func(string) string, localtime string) Defaults {
	defaults := Fallback
	if locale := localeFromEnv(getenv); locale != "" {
		defaults.Locale = locale
		if currency, ok := currencyByCountry[countryOf(locale)]; ok {
			defaults.Currency = currency
		}
	}
	if timezone := timezoneFrom(getenv, localtime); timezone != "" {
		defaults.Timezone = timezone
	}
	return defaults
}

// localeFromEnv turns "nl_NL.UTF-8" into "nl_NL", trying the variables in the
// order a shell would.
func localeFromEnv(getenv func(string) string) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := getenv(name)
		if dot := strings.IndexAny(value, ".@"); dot >= 0 {
			value = value[:dot]
		}
		if localePattern.MatchString(value) {
			return value
		}
	}
	return ""
}

// timezoneFrom prefers TZ, then the zone /etc/localtime points at. Either
// may name UTC in one of its aliases; Magento only knows the plain one.
func timezoneFrom(getenv func(string) string, localtime string) string {
	if tz := normalizeZone(getenv("TZ")); ValidateTimezone(tz) == nil {
		return tz
	}
	target, err := filepath.EvalSymlinks(localtime)
	if err != nil {
		return ""
	}
	zone := normalizeZone(zoneFromPath(target))
	if ValidateTimezone(zone) != nil {
		return ""
	}
	return zone
}

// utcAliases are the names systems use for UTC that Magento does not accept.
// Linux images typically link /etc/localtime to Etc/UTC.
var utcAliases = map[string]bool{
	"Etc/UTC": true, "Etc/GMT": true, "Etc/Universal": true, "Etc/Zulu": true, "Etc/UCT": true,
	"GMT": true, "Universal": true, "Zulu": true, "UCT": true, "Z": true,
}

// normalizeZone maps every spelling of UTC to the one Magento knows.
func normalizeZone(zone string) string {
	if utcAliases[zone] {
		return "UTC"
	}
	return zone
}

// zoneFromPath extracts "Europe/Amsterdam" from a zoneinfo path.
func zoneFromPath(path string) string {
	const marker = "zoneinfo/"
	if i := strings.LastIndex(path, marker); i >= 0 {
		return path[i+len(marker):]
	}
	return ""
}

// countryOf is the part of a locale after the underscore.
func countryOf(locale string) string {
	if i := strings.Index(locale, "_"); i >= 0 {
		return locale[i+1:]
	}
	return ""
}

// ValidateLocale accepts Magento locale codes such as en_US or nl_NL.
func ValidateLocale(locale string) error {
	if !localePattern.MatchString(locale) {
		return errors.New("Locale must look like en_US or nl_NL")
	}
	return nil
}

// ValidateTimezone accepts what setup:install accepts: PHP's zone list, which
// is every Region/City zone plus plain UTC, and none of the Etc/ or legacy
// aliases Go would happily load.
func ValidateTimezone(timezone string) error {
	invalid := errors.New("Timezone must be a zone such as Europe/Amsterdam, or UTC")
	if timezone == "UTC" {
		return nil
	}
	if !strings.Contains(timezone, "/") || strings.HasPrefix(timezone, "Etc/") {
		return invalid
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return invalid
	}
	return nil
}

// ValidateCurrency accepts an ISO 4217 code.
func ValidateCurrency(currency string) error {
	if !currencyPattern.MatchString(currency) {
		return errors.New("Currency must be a three-letter code such as EUR")
	}
	return nil
}
