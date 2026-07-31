package swag

import (
	"errors"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
)

// PrimitiveSchema build a primitive schema.
func PrimitiveSchema(refType string) *base.SchemaProxy {
	return base.CreateSchemaProxy(&base.Schema{Type: []string{refType}})
}

// IsComplexSchema whether a schema is complex and should be a ref schema
func IsComplexSchema(schema *Schema) bool {
	// a enum type should be complex
	if len(schema.Enum) > 0 {
		return true
	}

	// a schema without type (i.e. `any`) cannot be complex
	if len(schema.Type) == 0 {
		return false
	}

	// a deep array type is complex, how to determine deep? here more than 2 ,for example: [][]object,[][][]int
	if len(schema.Type) > 2 {
		return true
	}

	//Object included, such as Object or []Object
	for _, st := range schema.Type {
		if st == OBJECT {
			return true
		}
	}
	return false
}

// RefSchema build a reference schema.
func RefSchema(refType string) *base.SchemaProxy {
	return base.CreateSchemaProxyRef("#/components/schemas/" + refType)
}

// BuildCustomSchema build custom schema specified by tag swaggertype.
func BuildCustomSchema(types []string) (*base.SchemaProxy, error) {
	if len(types) == 0 {
		return nil, nil
	}

	switch types[0] {
	case PRIMITIVE:
		if len(types) == 1 {
			return nil, errors.New("need primitive type after primitive")
		}

		return BuildCustomSchema(types[1:])
	case ARRAY:
		if len(types) == 1 {
			return nil, errors.New("need array item type after array")
		}

		schema, err := BuildCustomSchema(types[1:])
		if err != nil {
			return nil, err
		}

		return base.CreateSchemaProxy(&base.Schema{
			Type:  []string{ARRAY},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}), nil
	case OBJECT:
		if len(types) == 1 {
			return PrimitiveSchema(types[0]), nil
		}

		schema, err := BuildCustomSchema(types[1:])
		if err != nil {
			return nil, err
		}

		return base.CreateSchemaProxy(&base.Schema{
			Type:                 []string{OBJECT},
			AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}), nil
	default:
		err := CheckSchemaType(types[0])
		if err != nil {
			return nil, err
		}

		return PrimitiveSchema(types[0]), nil
	}
}

// TransToValidParamStyle determine valid collection format.
func TransToValidParamStyle(format, in string) string {
	switch in {
	case "query":
		switch format {
		case "form", "spaceDelimited", "pipeDelimited", "deepObject":
			return format
		case "ssv":
			return "spaceDelimited"
		case "pipes":
			return "pipe"
		case "multi":
			return "form"
		case "csv":
			return "form"
		default:
			return ""
		}
	case "path":
		switch format {
		case "matrix", "label", "simple":
			return format
		case "csv":
			return "simple"
		default:
			return ""
		}
	case "header":
		switch format {
		case "form", "simple":
			return format
		case "csv":
			return "simple"
		default:
			return ""
		}
	case "cookie":
		switch format {
		case "form":
			return format
		}
	}

	return ""
}
