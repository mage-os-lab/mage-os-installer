package locale

import (
	"os"
	"path/filepath"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestDetectFrom_ReadsLocaleAndCurrencyFromLANG(t *testing.T) {
	got := DetectFrom(env(map[string]string{"LANG": "nl_NL.UTF-8"}), "/nonexistent")

	if got.Locale != "nl_NL" || got.Currency != "EUR" {
		t.Errorf("DetectFrom() = %+v, expected nl_NL and EUR", got)
	}
}

func TestDetectFrom_PrefersLC_ALLAndMapsTheCurrency(t *testing.T) {
	got := DetectFrom(env(map[string]string{"LC_ALL": "en_GB.UTF-8", "LANG": "nl_NL.UTF-8"}), "/nonexistent")

	if got.Locale != "en_GB" || got.Currency != "GBP" {
		t.Errorf("DetectFrom() = %+v, expected en_GB and GBP", got)
	}
}

func TestDetectFrom_FallsBackWhenTheMachineSaysNothing(t *testing.T) {
	got := DetectFrom(env(map[string]string{"LANG": "C"}), "/nonexistent")

	if got != Fallback {
		t.Errorf("DetectFrom() = %+v, expected %+v", got, Fallback)
	}
}

func TestDetectFrom_ReadsTZ(t *testing.T) {
	got := DetectFrom(env(map[string]string{"TZ": "Europe/Amsterdam"}), "/nonexistent")

	if got.Timezone != "Europe/Amsterdam" {
		t.Errorf("Timezone = %q, expected Europe/Amsterdam", got.Timezone)
	}
}

func TestDetectFrom_ReadsTheLocaltimeSymlink(t *testing.T) {
	dir := t.TempDir()
	zone := filepath.Join(dir, "zoneinfo", "Europe", "Berlin")
	if err := os.MkdirAll(filepath.Dir(zone), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zone, nil, 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink(zone, link); err != nil {
		t.Fatal(err)
	}

	got := DetectFrom(env(map[string]string{}), link)

	if got.Timezone != "Europe/Berlin" {
		t.Errorf("Timezone = %q, expected Europe/Berlin", got.Timezone)
	}
}

func TestDetectFrom_IgnoresAnUnknownTZ(t *testing.T) {
	got := DetectFrom(env(map[string]string{"TZ": "Mars/Olympus"}), "/nonexistent")

	if got.Timezone != Fallback.Timezone {
		t.Errorf("Timezone = %q, expected the fallback", got.Timezone)
	}
}

func TestValidate_AcceptsWellFormedValues(t *testing.T) {
	if err := ValidateLocale("nl_NL"); err != nil {
		t.Error(err)
	}
	if err := ValidateTimezone("Europe/Amsterdam"); err != nil {
		t.Error(err)
	}
	if err := ValidateCurrency("EUR"); err != nil {
		t.Error(err)
	}
}

func TestValidate_RejectsMalformedValues(t *testing.T) {
	for _, locale := range []string{"", "nl", "NL_nl", "nl-NL", "english"} {
		if ValidateLocale(locale) == nil {
			t.Errorf("ValidateLocale(%q) should fail", locale)
		}
	}
	for _, tz := range []string{"", "Local", "Amsterdam", "CET+1"} {
		if ValidateTimezone(tz) == nil {
			t.Errorf("ValidateTimezone(%q) should fail", tz)
		}
	}
	for _, currency := range []string{"", "eur", "EURO", "€"} {
		if ValidateCurrency(currency) == nil {
			t.Errorf("ValidateCurrency(%q) should fail", currency)
		}
	}
}
