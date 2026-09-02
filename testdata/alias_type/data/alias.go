package data

import (
	"time"

	"github.com/FunnelDash/swag/v3/testdata/alias_type/types"
)

type TimeContainer struct {
	Name      types.StringAlias `json:"name"`
	Timestamp time.Time         `json:"timestamp"`
	CreatedAt types.DateOnly    `json:"created_at"`
}
