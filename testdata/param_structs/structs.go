package structs

type FormModel struct {
	Foo string `form:"f" binding:"required" validate:"max=10"`
	// B is another field
	B bool
}

type AuthHeader struct {
	// Token is the auth token
	Token string `header:"X-Auth-Token" binding:"required"`
	// AnotherHeader is another header
	AnotherHeader int `validate:"gte=0,lte=10"`
}

type PathModel struct {
	// ID is the id
	Identifier int    `uri:"id" binding:"required"`
	Name       string `validate:"max=10"`
}

type OrderDirection string

const (
	OrderAsc  OrderDirection = "asc"
	OrderDesc OrderDirection = "desc"
)

type EnumQueryModel struct {
	// Direction to sort by
	Direction OrderDirection `form:"direction" default:"desc"`
	Status    OrderDirection `form:"status" example:"asc"`
}

type CSV[T any] struct {
	Values []T
}

type EnumArrayQueryModel struct {
	Directions CSV[OrderDirection] `form:"directions[]"`
	Names      CSV[string]         `form:"names[]"`
}

type EmbeddedBase struct {
	Alpha string `json:"alpha"`
	Beta  string `json:"beta"`
}

type Embedder struct {
	EmbeddedBase
	Gamma string `json:"gamma"`
}
