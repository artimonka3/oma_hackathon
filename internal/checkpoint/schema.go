package checkpoint

import _ "embed"

//go:embed schema.sql
var postgresSchema string
