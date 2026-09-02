// Package apispec embeds the OpenAPI 3.1 contract so the server can serve it
// without shipping a separate file. The spec (openapi.yaml) is the source of
// truth for the wire contract and for generated client SDKs.
package apispec

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
