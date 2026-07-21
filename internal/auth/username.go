package auth

import (
	"fmt"
	"regexp"
	"strings"
)

// usernamePattern is the username pattern modeled after Discord.
var usernamePattern = regexp.MustCompile(`^[a-z0-9_.]{2,32}$`)

// ValidateUsername trims and lowercases raw, then enforces the allowed charset, length, and period placement.
func ValidateUsername(raw string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(username) {
		return "", fmt.Errorf("username must be 2-32 characters and contain only lowercase letters, digits, underscore and period")
	}
	if strings.HasPrefix(username, ".") || strings.HasSuffix(username, ".") || strings.Contains(username, "..") {
		return "", fmt.Errorf("username can't start or end with a period, or contain consecutive periods")
	}
	return username, nil
}
