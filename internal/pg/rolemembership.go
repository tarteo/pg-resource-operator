package pg

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// IsMemberOf checks if memberName is a member of roleName. It returns true if memberName is a member of roleName, and false otherwise.
func IsMemberOf(handler *sql.DB, roleName string, memberName string) (bool, error) {
	query := `
		SELECT 1
		FROM pg_catalog.pg_auth_members m
		JOIN pg_catalog.pg_roles r ON m.roleid = r.oid
		JOIN pg_catalog.pg_roles u ON m.member = u.oid
		WHERE r.rolname = $1 AND u.rolname = $2
	`

	rows, err := handler.Query(query, roleName, memberName)
	if err != nil {
		return false, err
	}
	// Ensure rows are closed after processing
	// nolint:errcheck
	defer rows.Close()

	if rows.Next() {
		return true, nil
	}
	return false, nil
}

// GrantRole grants roleName to memberName, making memberName a member of roleName. It returns an error if the operation fails.
func GrantRole(handler *sql.DB, roleName string, memberName string) error {
	query := fmt.Sprintf("GRANT %s TO %s", pq.QuoteIdentifier(roleName), pq.QuoteIdentifier(memberName))
	_, err := handler.Exec(query)
	return err
}

// RevokeRole revokes roleName from memberName, removing memberName from roleName. It returns an error if the operation fails.
func RevokeRole(handler *sql.DB, roleName string, memberName string) error {
	query := fmt.Sprintf("REVOKE %s FROM %s", pq.QuoteIdentifier(roleName), pq.QuoteIdentifier(memberName))
	_, err := handler.Exec(query)
	return err
}
