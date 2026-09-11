package magento

import (
	"strings"
	"testing"
)

func TestValidateAdminPassword_AcceptsLettersAndDigitsAtMinimumLength(t *testing.T) {
	for _, password := range []string{
		"Admin123!Mage",
		"123456789012a",
		"a12345678901",
		"Se$cret 12345",
	} {
		if err := ValidateAdminPassword(password); err != nil {
			t.Errorf("ValidateAdminPassword(%q) = %v, expected no error", password, err)
		}
	}
}

func TestValidateAdminPassword_RejectsPasswordWithoutDigits(t *testing.T) {
	err := ValidateAdminPassword("OnlyLettersHere")
	if err == nil {
		t.Fatal("expected an error for a password without digits")
	}
	if !strings.Contains(err.Error(), "numeric and alphabetic") {
		t.Errorf("got %q, expected it to mention numeric and alphabetic characters", err)
	}
}

func TestValidateAdminPassword_RejectsPasswordWithoutLetters(t *testing.T) {
	err := ValidateAdminPassword("1234567890123")
	if err == nil {
		t.Fatal("expected an error for a password without letters")
	}
	if !strings.Contains(err.Error(), "numeric and alphabetic") {
		t.Errorf("got %q, expected it to mention numeric and alphabetic characters", err)
	}
}

func TestValidateAdminPassword_RejectsPasswordShorterThanTheMinimum(t *testing.T) {
	err := ValidateAdminPassword("Admin123!")
	if err == nil {
		t.Fatal("expected an error for a password shorter than the minimum")
	}
	if !strings.Contains(err.Error(), "at least 12 characters") {
		t.Errorf("got %q, expected it to mention the minimum length", err)
	}
}

func TestValidateAdminPassword_ReportsEveryBrokenRuleAtOnce(t *testing.T) {
	err := ValidateAdminPassword("letters")
	if err == nil {
		t.Fatal("expected an error for a short password without digits")
	}
	for _, want := range []string{"at least 12 characters", "numeric and alphabetic"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q, expected it to mention %q", err, want)
		}
	}
}

func TestValidateAdminPassword_RejectsEmptyAndBlankPasswords(t *testing.T) {
	for _, password := range []string{"", "   "} {
		err := ValidateAdminPassword(password)
		if err == nil {
			t.Fatalf("expected an error for %q", password)
		}
		if !strings.Contains(err.Error(), "required") {
			t.Errorf("got %q, expected it to say the password is required", err)
		}
	}
}

func TestValidateAdminPassword_CountsCharactersNotBytes(t *testing.T) {
	if err := ValidateAdminPassword("wachtwoord1é"); err != nil {
		t.Errorf("ValidateAdminPassword(%q) = %v, expected 12 characters to be accepted", "wachtwoord1é", err)
	}
	if err := ValidateAdminPassword("wachtwoord1"); err == nil {
		t.Error("expected 11 characters to be rejected")
	}
}
