package pg

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

type Privilege string

const (
	CONNECT   Privilege = "CONNECT"
	CREATE    Privilege = "CREATE"
	TEMPORARY Privilege = "TEMPORARY"
)

func (p Privilege) String() string {
	return string(p)
}

func JoinPrivileges(privileges []Privilege) string {
	privilegeStrings := make([]string, 0, len(privileges))
	for _, priv := range privileges {
		privilegeStrings = append(privilegeStrings, priv.String())
	}
	return strings.Join(privilegeStrings, ", ")
}

func GrantPrivileges(
	handler *sql.DB,
	databaseName string,
	privileges []Privilege,
	role string,
) error {
	// If there are no privileges to grant, return early
	if len(privileges) == 0 {
		return nil
	}

	// Quote the role name, unless it's "PUBLIC"
	var quotedRole string = role
	if strings.ToUpper(role) != "PUBLIC" {
		quotedRole = pq.QuoteIdentifier(role)
	}

	quotedDatabaseName := pq.QuoteIdentifier(databaseName)
	privilegesList := JoinPrivileges(privileges)

	query := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", privilegesList, quotedDatabaseName, quotedRole)
	_, err := handler.Exec(query)
	return err
}

func RevokePrivileges(
	handler *sql.DB,
	databaseName string,
	privileges []Privilege,
	role string,
) error {
	// If there are no privileges to revoke, return early
	if len(privileges) == 0 {
		return nil
	}

	// Quote the role name, unless it's "PUBLIC"
	var quotedRole string = role
	if strings.ToUpper(role) != "PUBLIC" {
		quotedRole = pq.QuoteIdentifier(role)
	}

	quotedDatabaseName := pq.QuoteIdentifier(databaseName)
	privilegesList := JoinPrivileges(privileges)

	query := fmt.Sprintf("REVOKE %s ON DATABASE %s FROM %s", privilegesList, quotedDatabaseName, quotedRole)
	_, err := handler.Exec(query)
	return err
}
