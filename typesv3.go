package swag

import "github.com/sv-tools/openapi/spec"

// Schema parsed schema.
type Schema struct {
	*spec.Schema        //
	PkgPath      string // package import path used to rename Name of a definition int case of conflict
	Name         string // Name in definitions
}
