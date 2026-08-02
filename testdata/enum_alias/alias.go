package enum_alias

import "github.com/FunnelDash/swag/v2/testdata/enum_alias/target"

// Status aliases target.Status with local alias-consts — the shape that
// doubled the component enum before the ParseDefinitionV3 set-not-append fix.
type Status = target.Status

const (
	StatusOpen   = target.StatusOpen
	StatusClosed = target.StatusClosed
)

type Holder struct {
	Status Status `json:"status"`
}
