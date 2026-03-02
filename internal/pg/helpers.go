package pg

import (
	"fmt"
	"regexp"

	"github.com/lib/pq"
)

var validIdentifierRegex = regexp.MustCompile(`^[a-z][a-z0-9_\-]*$`)

// ValidateIdentifier checks if the provided name is a valid PostgreSQL identifier and returns the quoted version of it.
// Used for early validation of identifiers before executing queries.
// If the identifier is invalid, an error is returned.
func ValidateIdentifier(name string) (string, error) {
	// PostgreSQL identifiers must start with a letter and can contain letters, digits, underscores, and hyphens.
	// This regex also enforces lowercase letters to prevent case sensitivity issues, this is not strictly required by PostgreSQL.
	if valid := validIdentifierRegex.MatchString(name); !valid {
		return "", fmt.Errorf("invalid identifier: %s", name)
	}
	// PostgreSQL identifiers have a maximum length of 63 characters. If the identifier is longer than 63 characters,
	// it will be truncated by PostgreSQL, which can lead to unexpected behavior.
	if len(name) > 63 {
		return "", fmt.Errorf("identifier too long: %s", name)
	}
	return pq.QuoteIdentifier(name), nil
}
