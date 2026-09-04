package api

type Shape interface {
	isShape()
}

// Circle names an interface that does not exist, which must not pass
// silently: the variant would simply never appear in the spec.
// @oneOfMember Shpae
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
