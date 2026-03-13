package pg

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

func RoleExists(handler *sql.DB, roleName string) (bool, error) {
	rows, err := handler.Query("SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1", roleName)
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

func CreateRole(handler *sql.DB, roleName string, password *string, attributes []string) error {
	quotedRoleName := pq.QuoteIdentifier(roleName)
	query := fmt.Sprintf("CREATE ROLE %s", quotedRoleName)
	if password != nil {
		passwordEscaped := pq.QuoteLiteral(*password)
		query = fmt.Sprintf("%s WITH LOGIN PASSWORD %s", query, passwordEscaped)
	} else {
		query = fmt.Sprintf("%s WITH NOLOGIN", query)
	}
	if len(attributes) > 0 {
		query = fmt.Sprintf("%s %s", query, strings.Join(attributes, " "))
	}
	_, err := handler.Exec(query)
	return err
}

func DropRole(handler *sql.DB, roleName string) error {
	quotedRoleName := pq.QuoteIdentifier(roleName)
	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", quotedRoleName)
	_, err := handler.Exec(query)
	return err
}

func AlterRole(handler *sql.DB, roleName string, newPassword *string, attributes []string) error {
	quotedRoleName := pq.QuoteIdentifier(roleName)
	query := fmt.Sprintf("ALTER ROLE %s", quotedRoleName)
	if newPassword != nil {
		newPasswordEscaped := pq.QuoteLiteral(*newPassword)
		query = fmt.Sprintf("%s WITH LOGIN PASSWORD %s", query, newPasswordEscaped)
	} else {
		query = fmt.Sprintf("%s WITH NOLOGIN PASSWORD NULL", query)
	}
	if len(attributes) > 0 {
		query = fmt.Sprintf("%s %s", query, strings.Join(attributes, " "))
	}
	_, err := handler.Exec(query)
	return err
}
