package spec

import _ "embed"

// OpenAPI holds the embedded OpenAPI v3 spec.

//go:embed openapi.yaml
var OpenAPI []byte
