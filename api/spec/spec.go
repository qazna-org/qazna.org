package spec

import _ "embed"

// OpenAPI holds the embedded OpenAPI YAML.
//
//go:embed openapi.yaml
var OpenAPI []byte
