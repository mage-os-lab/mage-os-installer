// Package magento mirrors the validation Mage-OS itself applies, so the
// installer can reject input while the user can still fix it, instead of
// letting bin/magento fail halfway through an install.
package magento

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MinimumAdminPasswordLength follows the PCI DSS 4 ruleset that
// Aligent_Pci4Compatibility applies, which raises the Mage-OS default of 7.
const MinimumAdminPasswordLength = 12

// alphanumericPassword is the rule from
// Magento\User\Model\UserValidationRules::addPasswordRules.
var alphanumericPassword = regexp.MustCompile(`(?i)[a-z][^\n]*[0-9]|[0-9][^\n]*[a-z]`)

// ValidateAdminPassword reports every rule the admin password breaks, worded
// the way bin/magento setup:install words it.
func ValidateAdminPassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("Password is required field.")
	}

	var messages []string
	if utf8.RuneCountInString(password) < MinimumAdminPasswordLength {
		messages = append(messages, fmt.Sprintf(
			"Your password must be at least %d characters.", MinimumAdminPasswordLength))
	}
	if !alphanumericPassword.MatchString(password) {
		messages = append(messages, "Your password must include both numeric and alphabetic characters.")
	}
	if messages == nil {
		return nil
	}

	return errors.New(strings.Join(messages, " "))
}

// AdminPasswordHint describes the rules in one line, for use next to the input.
func AdminPasswordHint() string {
	return fmt.Sprintf("at least %d characters, including a letter and a number",
		MinimumAdminPasswordLength)
}
