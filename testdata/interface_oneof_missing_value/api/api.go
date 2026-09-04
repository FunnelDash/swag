package api

// Shape is a shape.
// @discriminator kind
type Shape interface {
	isShape()
}

// Circle declares no @discriminatorValue, so the mapping cannot be built.
// @oneOfMember Shape
type Circle struct {
	Kind string `json:"kind"`
}

func (Circle) isShape() {}

type Drawing struct {
	Shapes []Shape `json:"shapes"`
}

// GetDrawing godoc
// @Summary Get a drawing
// @Produce json
// @Success 200 {object} api.Drawing
// @Router /drawing [get]
func GetDrawing() {}
