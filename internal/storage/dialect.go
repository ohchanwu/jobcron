package storage

import "fmt"

// Dialect identifies the SQL backend a Store is using.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// Placeholder returns the nth bind placeholder for this SQL dialect.
func (d Dialect) Placeholder(n int) string {
	if d == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
