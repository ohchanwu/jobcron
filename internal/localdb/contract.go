// Package localdb provides the canonical local PostgreSQL definition.
package localdb

import _ "embed"

// ComposeYAML is the canonical Compose definition for managed local PostgreSQL.
//
//go:embed compose.yaml
var ComposeYAML []byte
