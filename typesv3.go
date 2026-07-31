package swag

import base "github.com/pb33f/libopenapi/datamodel/high/base"

// Schema parsed schema.
type Schema struct {
	*base.Schema        // embedded libopenapi schema
	PkgPath      string // package import path used to rename Name of a definition int case of conflict
	Name         string // Name in definitions
}
