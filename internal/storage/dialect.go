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

func (d Dialect) Rebind(query string) string {
	if d != DialectPostgres {
		return query
	}
	out := make([]byte, 0, len(query)+8)
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] != '?' {
			out = append(out, query[i])
			continue
		}
		out = append(out, fmt.Sprintf("$%d", n)...)
		n++
	}
	return string(out)
}

func (s *Store) query(query string) string {
	return s.dialect.Rebind(query)
}
