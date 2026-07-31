package swag

import (
	"go/ast"
	"testing"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/stretchr/testify/assert"
	yaml "go.yaml.in/yaml/v4"
)

// fpArrayOf builds an array schema proxy with a primitive item type — the
// libopenapi equivalent of the old spec.NewBoolOrSchema(false, ...) + item
// type dance.
func fpArrayOf(itemType string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{
		Type:  []string{ARRAY},
		Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: PrimitiveSchema(itemType)},
	})
}

// fpNodeVal decodes a *yaml.Node (libopenapi's storage for Example/Default/Enum
// and extension values) back into a plain Go value so assertions can compare
// against the original literals.
func fpNodeVal(n *yaml.Node) any {
	var v any
	_ = n.Decode(&v)
	return v
}

func fpNodeVals(ns []*yaml.Node) []any {
	out := make([]any, 0, len(ns))
	for _, n := range ns {
		out = append(out, fpNodeVal(n))
	}
	return out
}

func TestDefaultFieldParser(t *testing.T) {
	t.Run("Example tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" example:"one"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "one", fpNodeVal(got.Schema().Example))

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" example:""`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "", fpNodeVal(got.Schema().Example))

		schema = PrimitiveSchema("float")
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" example:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)
	})

	t.Run("Format tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" format:"csv"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "csv", got.Schema().Format)
	})

	t.Run("Required tag", func(t *testing.T) {
		t.Parallel()

		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" binding:"required"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.Equal(t, true, got)

		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.Equal(t, true, got)
	})

	t.Run("Default required tag", func(t *testing.T) {
		t.Parallel()

		got, err := newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("Optional tag", func(t *testing.T) {
		t.Parallel()

		got, err := newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" binding:"optional"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.False(t, got)

		got, err = newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"optional"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.False(t, got)

		got, err = newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test,omitempty"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.False(t, got)

		got, err = newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test,omitzero"`,
			}},
		).IsRequired()
		assert.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("Pointer type is optional", func(t *testing.T) {
		t.Parallel()

		got, err := newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{
				Type: &ast.StarExpr{X: &ast.Ident{Name: "string"}},
				Tag: &ast.BasicLit{
					Value: `json:"test"`,
				},
			},
		).IsRequired()
		assert.NoError(t, err)
		assert.False(t, got)

		// Explicit required tag should override pointer optionality
		got, err = newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{
				Type: &ast.StarExpr{X: &ast.Ident{Name: "string"}},
				Tag: &ast.BasicLit{
					Value: `json:"test" validate:"required"`,
				},
			},
		).IsRequired()
		assert.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("Skipped tag", func(t *testing.T) {
		t.Parallel()

		got, err := newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"-"`,
			}},
		).FieldNames()
		assert.NoError(t, err)
		assert.Empty(t, got)

		got, err = newTagBaseFieldParser(
			&Parser{
				RequiredByDefault: true,
			},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `form:"-"`,
			}},
		).FieldNames()
		assert.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("Extensions tag", func(t *testing.T) {
		t.Parallel()

		schema := base.CreateSchemaProxy(&base.Schema{Type: []string{INTEGER}})
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" extensions:"x-nullable,x-abc=def,!x-omitempty,x-example=[0, 9],x-example2={çãíœ, (bar=(abc, def)), [0,9]}"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, true, fpNodeVal(got.Schema().Extensions.GetOrZero("x-nullable")))
		assert.Equal(t, "def", fpNodeVal(got.Schema().Extensions.GetOrZero("x-abc")))
		assert.Equal(t, false, fpNodeVal(got.Schema().Extensions.GetOrZero("x-omitempty")))
		assert.Equal(t, "[0, 9]", fpNodeVal(got.Schema().Extensions.GetOrZero("x-example")))
		assert.Equal(t, "{çãíœ, (bar=(abc, def)), [0,9]}", fpNodeVal(got.Schema().Extensions.GetOrZero("x-example2")))
	})

	t.Run("Enums tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"a,b,c"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"a", "b", "c"}, fpNodeVals(got.Schema().Enum))

		schema = PrimitiveSchema("float")
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"a,b,c"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)
	})

	t.Run("Enums tag twice", func(t *testing.T) {
		t.Parallel()

		schema := base.CreateSchemaProxy(&base.Schema{Type: []string{STRING}})

		parser := &Parser{}
		fieldParser := newTagBaseFieldParser(
			parser,
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"a,b,c"`,
			}},
		)
		got, err := fieldParser.ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"a", "b", "c"}, fpNodeVals(got.Schema().Enum))

		fieldParser2 := newTagBaseFieldParser(
			parser,
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"d,e,f"`,
			}},
		)
		got2, _ := fieldParser2.ComplementSchema(schema)
		assert.Equal(t, []interface{}{"a", "b", "c", "d", "e", "f"}, fpNodeVals(got2.Schema().Enum))

	})

	t.Run("EnumVarNames tag", func(t *testing.T) {
		t.Parallel()

		schema := base.CreateSchemaProxy(&base.Schema{Type: []string{INTEGER}})
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"0,1,2" x-enum-varnames:"Daily,Weekly,Monthly"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"Daily", "Weekly", "Monthly"}, fpNodeVal(got.Schema().Extensions.GetOrZero("x-enum-varnames")))

		schema = base.CreateSchemaProxy(&base.Schema{Type: []string{INTEGER}})
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"0,1,2,3" x-enum-varnames:"Daily,Weekly,Monthly"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)

		// Test for an array of enums
		schema = fpArrayOf(INTEGER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" enums:"0,1,2" x-enum-varnames:"Daily,Weekly,Monthly"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"Daily", "Weekly", "Monthly"}, fpNodeVal(got.Schema().Items.A.Schema().Extensions.GetOrZero("x-enum-varnames")))
		assert.Zero(t, orderedmap.Len(got.Schema().Extensions))
	})

	t.Run("Default tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" default:"pass"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "pass", fpNodeVal(got.Schema().Default))

		schema = PrimitiveSchema("float")
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" default:"pass"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)
	})

	t.Run("Numeric value", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(INTEGER)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maximum:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		maxV := float64(1)
		assert.Equal(t, &maxV, got.Schema().Maximum)

		schema = PrimitiveSchema(INTEGER)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maximum:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)

		schema = PrimitiveSchema(NUMBER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maximum:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		maxV = float64(1)
		assert.Equal(t, &maxV, got.Schema().Maximum)

		schema = PrimitiveSchema(NUMBER)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maximum:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)

		schema = PrimitiveSchema(NUMBER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" multipleOf:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		multipleOf := float64(1)
		assert.Equal(t, &multipleOf, got.Schema().MultipleOf)

		schema = PrimitiveSchema(NUMBER)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" multipleOf:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)

		schema = PrimitiveSchema(INTEGER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" minimum:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		minV := float64(1)
		assert.Equal(t, &minV, got.Schema().Minimum)

		schema = PrimitiveSchema(INTEGER)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" minimum:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)
	})

	t.Run("String value", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maxLength:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		maxL := int64(1)
		assert.Equal(t, &maxL, got.Schema().MaxLength)

		schema = PrimitiveSchema(STRING)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" maxLength:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" minLength:"1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		minL := int64(1)
		assert.Equal(t, &minL, got.Schema().MinLength)

		schema = PrimitiveSchema(STRING)
		_, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" minLength:"one"`,
			}},
		).ComplementSchema(schema)
		assert.Error(t, err)
	})

	t.Run("Readonly tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" readonly:"true"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.True(t, *got.Schema().ReadOnly)
	})

	t.Run("OneOf tag", func(t *testing.T) {
		t.Parallel()

		schema := base.CreateSchemaProxy(&base.Schema{Type: []string{ANY}})
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" oneOf:"string,float64"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Len(t, got.Schema().OneOf, 2)
		assert.Equal(t, []string{STRING}, got.Schema().OneOf[0].Schema().Type)
		assert.Equal(t, []string{NUMBER}, got.Schema().OneOf[1].Schema().Type)
	})
}

func TestValidTags(t *testing.T) {
	t.Run("Required with max/min tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=10,min=1"`,
			}},
		).ComplementSchema(schema)
		maxL := int64(10)
		minL := int64(1)
		assert.NoError(t, err)
		assert.Equal(t, &maxL, got.Schema().MaxLength)
		assert.Equal(t, &minL, got.Schema().MinLength)

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=10,gte=1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, &maxL, got.Schema().MaxLength)
		assert.Equal(t, &minL, got.Schema().MinLength)

		schema = PrimitiveSchema(INTEGER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=10,min=1"`,
			}},
		).ComplementSchema(schema)
		maxFloat64 := float64(10)
		minFloat64 := float64(1)
		assert.NoError(t, err)
		assert.Equal(t, &maxFloat64, got.Schema().Maximum)
		assert.Equal(t, &minFloat64, got.Schema().Minimum)

		schema = fpArrayOf(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=10,min=1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, &maxL, got.Schema().MaxItems)
		assert.Equal(t, &minL, got.Schema().MinItems)

		// wrong validate tag will be ignored.
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=ten,min=1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Empty(t, got.Schema().MaxItems)
		assert.Equal(t, &minL, got.Schema().MinItems)
	})
	t.Run("Required with oneof tag", func(t *testing.T) {
		t.Parallel()

		schema := PrimitiveSchema(STRING)

		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof='red book' 'green book'"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"red book", "green book"}, fpNodeVals(got.Schema().Enum))

		schema = PrimitiveSchema(INTEGER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof=1 2 3"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{1, 2, 3}, fpNodeVals(got.Schema().Enum))

		schema = fpArrayOf(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof=red green yellow"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"red", "green", "yellow"}, fpNodeVals(got.Schema().Items.A.Schema().Enum))

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof='red green' blue 'c0x2Cc' 'd0x7Cd'"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"red green", "blue", "c,c", "d|d"}, fpNodeVals(got.Schema().Enum))

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof='c0x9Ab' book"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"c0x9Ab", "book"}, fpNodeVals(got.Schema().Enum))

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" binding:"oneof=foo bar" validate:"required,oneof=foo bar" enums:"a,b,c"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"a", "b", "c"}, fpNodeVals(got.Schema().Enum))

		schema = PrimitiveSchema(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" binding:"oneof=aa bb" validate:"required,oneof=foo bar"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, []interface{}{"aa", "bb"}, fpNodeVals(got.Schema().Enum))
	})
	t.Run("Required with unique tag", func(t *testing.T) {
		t.Parallel()

		schema := fpArrayOf(STRING)

		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,unique"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.True(t, *got.Schema().UniqueItems)
	})

	t.Run("All tag", func(t *testing.T) {
		t.Parallel()
		schema := fpArrayOf(STRING)

		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,unique,max=10,min=1,oneof=a0x2Cc 'c0x7Cd book',omitempty,dive,max=1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.True(t, *got.Schema().UniqueItems)

		maxI := int64(10)
		minI := int64(1)
		assert.Equal(t, &maxI, got.Schema().MaxItems)
		assert.Equal(t, &minI, got.Schema().MinItems)
		assert.Equal(t, []interface{}{"a,c", "c|d book"}, fpNodeVals(got.Schema().Items.A.Schema().Enum))

		schema = fpArrayOf(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof=,max=10=90,min=1"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Empty(t, got.Schema().UniqueItems)
		assert.Empty(t, got.Schema().MaxItems)
		assert.Equal(t, &minI, got.Schema().MinItems)

		schema = fpArrayOf(STRING)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,max=10,min=one"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, &maxI, got.Schema().MaxItems)
		assert.Empty(t, got.Schema().MinItems)

		schema = PrimitiveSchema(INTEGER)
		got, err = newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" validate:"required,oneof=one two"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Empty(t, got.Schema().Enum)
	})

	t.Run("Pattern tag", func(t *testing.T) {
		t.Parallel()

		schema := fpArrayOf(STRING)
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" pattern:"^[a-zA-Z0-9_]*$"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "^[a-zA-Z0-9_]*$", got.Schema().Items.A.Schema().Pattern)
	})

	t.Run("Pattern tag array", func(t *testing.T) {
		t.Parallel()

		schema := base.CreateSchemaProxy(&base.Schema{Type: typeString})
		got, err := newTagBaseFieldParser(
			&Parser{},
			&ast.File{Name: &ast.Ident{Name: "test"}},
			&ast.Field{Tag: &ast.BasicLit{
				Value: `json:"test" pattern:"^[a-zA-Z0-9_]*$"`,
			}},
		).ComplementSchema(schema)
		assert.NoError(t, err)
		assert.Equal(t, "^[a-zA-Z0-9_]*$", got.Schema().Pattern)
	})
}

func TestFieldNamesMultiName(t *testing.T) {
	t.Parallel()

	// e.g. encoding/xml.Name: `Space, Local string` — one declaration, two fields.
	field := &ast.Field{Names: []*ast.Ident{{Name: "Space"}, {Name: "Local"}}}
	names, err := newTagBaseFieldParser(&Parser{}, &ast.File{Name: &ast.Ident{Name: "test"}}, field).FieldNames()
	assert.NoError(t, err)
	assert.Equal(t, []string{"space", "local"}, names)

	names, err = newTagBaseFieldParser(&Parser{PropNamingStrategy: PascalCase}, &ast.File{Name: &ast.Ident{Name: "test"}}, field).FieldNames()
	assert.NoError(t, err)
	assert.Equal(t, []string{"Space", "Local"}, names)

	single := &ast.Field{
		Names: []*ast.Ident{{Name: "Field"}},
		Tag:   &ast.BasicLit{Value: `json:"renamed"`},
	}
	names, err = newTagBaseFieldParser(&Parser{}, &ast.File{Name: &ast.Ident{Name: "test"}}, single).FieldNames()
	assert.NoError(t, err)
	assert.Equal(t, []string{"renamed"}, names)
}
