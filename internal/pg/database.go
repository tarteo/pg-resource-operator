package pg

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

func DatabaseExists(handler *sql.DB, name string) (bool, error) {
	rows, err := handler.Query("SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1", name)
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

func DropDatabase(handler *sql.DB, name string, force bool) error {
	quotedName := pq.QuoteIdentifier(name)
	var query = fmt.Sprintf("DROP DATABASE IF EXISTS %s", quotedName)
	if force {
		query += " WITH (FORCE)"
	}
	_, err := handler.Exec(query)
	return err
}

func CreateDatabase(handler *sql.DB, name string, owner string, template string, encoding string) error {
	quotedName := pq.QuoteIdentifier(name)
	quotedOwner := pq.QuoteIdentifier(owner)
	quotedTemplate := pq.QuoteIdentifier(template)
	quotedEncoding := pq.QuoteLiteral(encoding)

	query := fmt.Sprintf(
		"CREATE DATABASE %s WITH OWNER %s TEMPLATE %s ENCODING %s",
		quotedName, quotedOwner, quotedTemplate, quotedEncoding,
	)
	_, err := handler.Exec(query)
	return err
}

func ChownDatabase(handler *sql.DB, name string, owner string) error {
	quotedName := pq.QuoteIdentifier(name)
	quotedOwner := pq.QuoteIdentifier(owner)

	query := fmt.Sprintf(
		"ALTER DATABASE %s OWNER TO %s",
		quotedName, quotedOwner,
	)
	_, err := handler.Exec(query)
	return err
}
