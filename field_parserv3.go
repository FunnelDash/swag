package swag

import (
	"fmt"
	"go/ast"
	"reflect"
	"strconv"
	"strings"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"
)

// yamlNodeFrom encodes an arbitrary Go value into a *yaml.Node, the shape
// libopenapi uses for schema Default/Example/Enum entries and extension
// values (was plain interface{} under sv-tools).
func yamlNodeFrom(v interface{}) *yaml.Node {
	n := &yaml.Node{}
	_ = n.Encode(v)
	return n
}

// intPtrToFloat64Ptr / intPtrToInt64Ptr bridge the structField accumulator
// (which keeps numeric bounds as *int) to libopenapi's *float64 / *int64
// schema fields. nil in, nil out — preserving the original overwrite-with-nil
// semantics.
func intPtrToFloat64Ptr(p *int) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

func intPtrToInt64Ptr(p *int) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

// schemaExtensions converts setExtensionParam's map[string]interface{} output
// into libopenapi's ordered extension map.
func schemaExtensions(m map[string]interface{}) *orderedmap.Map[string, *yaml.Node] {
	om := orderedmap.New[string, *yaml.Node]()
	for k, v := range m {
		om.Set(k, yamlNodeFrom(v))
	}
	return om
}

type structField struct {
	schemaType   string
	arrayType    string
	formatType   string
	maximum      *int
	minimum      *int
	multipleOf   *int
	maxLength    *int
	minLength    *int
	maxItems     *int
	minItems     *int
	exampleValue interface{}
	enums        []interface{}
	enumVarNames []interface{}
	unique       bool
	pattern      string
}

func (sf *structField) setOneOf(valValue string) {
	if len(sf.enums) != 0 {
		return
	}

	enumType := sf.schemaType
	if sf.schemaType == ARRAY {
		enumType = sf.arrayType
	}

	valValues := parseOneOfParam2(valValue)
	for i := range valValues {
		value, err := defineType(enumType, valValues[i])
		if err != nil {
			continue
		}

		sf.enums = append(sf.enums, value)
	}
}

func (sf *structField) setMin(valValue string) {
	value, err := strconv.Atoi(valValue)
	if err != nil {
		return
	}

	switch sf.schemaType {
	case INTEGER, NUMBER:
		sf.minimum = &value
	case STRING:
		sf.minLength = &value
	case ARRAY:
		sf.minItems = &value
	}
}

func (sf *structField) setMax(valValue string) {
	value, err := strconv.Atoi(valValue)
	if err != nil {
		return
	}

	switch sf.schemaType {
	case INTEGER, NUMBER:
		sf.maximum = &value
	case STRING:
		sf.maxLength = &value
	case ARRAY:
		sf.maxItems = &value
	}
}

type tagBaseFieldParser struct {
	p     *Parser
	file  *ast.File
	field *ast.Field
	tag   reflect.StructTag
}

func newTagBaseFieldParser(p *Parser, file *ast.File, field *ast.Field) FieldParser {
	fieldParser := tagBaseFieldParser{
		p:     p,
		file:  file,
		field: field,
		tag:   "",
	}
	if fieldParser.field.Tag != nil {
		fieldParser.tag = reflect.StructTag(strings.ReplaceAll(field.Tag.Value, "`", ""))
	}

	return &fieldParser
}

func (ps *tagBaseFieldParser) CustomSchema() (*base.SchemaProxy, error) {
	if ps.field.Tag == nil {
		return nil, nil
	}

	typeTag := ps.tag.Get(swaggerTypeTag)
	if typeTag != "" {
		return BuildCustomSchema(strings.Split(typeTag, ","))
	}

	return nil, nil
}

// ComplementSchema complement schema with field properties. It returns the
// (possibly rebuilt) schema proxy: libopenapi schema proxies are build-once,
// so a schema that gains field attributes is assembled fully and wrapped
// fresh rather than mutated in place behind an already-published proxy.
func (ps *tagBaseFieldParser) ComplementSchema(schema *base.SchemaProxy) (*base.SchemaProxy, error) {
	// GetSchemaTypePath follows a $ref to the referenced component, so the
	// type path is available without resolving (and mutating) that component.
	types := ps.p.GetSchemaTypePath(schema, 2)
	if len(types) == 0 {
		return nil, fmt.Errorf("invalid type for field: %s", ps.field.Names[0])
	}

	if schema.IsReference() { // a $ref field
		// Apply the field's own tags to a fresh schema wrapped beside the
		// reference (allOf) — never into the shared component the ref points at,
		// which is used by every other field of the same type. Writing a
		// per-field default/example/description/enum into the component leaks it
		// globally (last writer wins). A field with no tags of its own stays a
		// pure $ref. A query-param expansion later flattens this allOf back to
		// the referenced scalar plus these sibling attributes.
		var newSchema base.Schema
		if err := ps.complementSchema(&newSchema, types); err != nil {
			return nil, err
		}
		if !reflect.ValueOf(newSchema).IsZero() {
			newSchema.AllOf = []*base.SchemaProxy{base.CreateSchemaProxyRef(schema.GetReference())}
			return base.CreateSchemaProxy(&newSchema), nil
		}
		return schema, nil
	}

	s := schema.Schema()
	if err := ps.complementSchema(s, types); err != nil {
		return nil, err
	}
	return base.CreateSchemaProxy(s), nil
}

// complementSchema complement schema with field properties
func (ps *tagBaseFieldParser) complementSchema(schema *base.Schema, types []string) error {
	if ps.field.Tag == nil {
		if ps.field.Doc != nil {
			schema.Description = strings.TrimSpace(ps.field.Doc.Text())
		}

		if schema.Description == "" && ps.field.Comment != nil {
			schema.Description = strings.TrimSpace(ps.field.Comment.Text())
		}

		return nil
	}

	field := &structField{
		schemaType: types[0],
		formatType: ps.tag.Get(formatTag),
	}

	if len(types) > 1 && (types[0] == ARRAY || types[0] == OBJECT) {
		field.arrayType = types[1]
	}

	jsonTagValue := ps.tag.Get(jsonTag)

	bindingTagValue := ps.tag.Get(bindingTag)
	if bindingTagValue != "" {
		field.parseValidTags(bindingTagValue)
	}

	validateTagValue := ps.tag.Get(validateTag)
	if validateTagValue != "" {
		field.parseValidTags(validateTagValue)
	}

	enumsTagValue := ps.tag.Get(enumsTag)
	if enumsTagValue != "" {
		err := field.parseEnumTags(enumsTagValue)
		if err != nil {
			return err
		}
	}

	if IsNumericType(field.schemaType) || IsNumericType(field.arrayType) {
		maximum, err := getIntTag(ps.tag, maximumTag)
		if err != nil {
			return err
		}

		if maximum != nil {
			field.maximum = maximum
		}

		minimum, err := getIntTag(ps.tag, minimumTag)
		if err != nil {
			return err
		}

		if minimum != nil {
			field.minimum = minimum
		}

		multipleOf, err := getIntTag(ps.tag, multipleOfTag)
		if err != nil {
			return err
		}

		if multipleOf != nil {
			field.multipleOf = multipleOf
		}
	}

	if field.schemaType == STRING || field.arrayType == STRING {
		maxLength, err := getIntTag(ps.tag, maxLengthTag)
		if err != nil {
			return err
		}

		if maxLength != nil {
			field.maxLength = maxLength
		}

		minLength, err := getIntTag(ps.tag, minLengthTag)
		if err != nil {
			return err
		}

		if minLength != nil {
			field.minLength = minLength
		}

		pattern, ok := ps.tag.Lookup(patternTag)
		if ok {
			field.pattern = pattern
		}
	}

	// json:"name,string" or json:",string"
	exampleTagValue, ok := ps.tag.Lookup(exampleTag)
	if ok {
		field.exampleValue = exampleTagValue

		if !strings.Contains(jsonTagValue, ",string") {
			example, err := defineTypeOfExample(field.schemaType, field.arrayType, exampleTagValue)
			if err != nil {
				return err
			}

			field.exampleValue = example
		}
	}

	// perform this after setting everything else (min, max, etc...)
	if strings.Contains(jsonTagValue, ",string") {
		// @encoding/json: "It applies only to fields of string, floating point, integer, or boolean types."
		defaultValues := map[string]string{
			// Zero Values as string
			STRING:  "",
			INTEGER: "0",
			BOOLEAN: "false",
			NUMBER:  "0",
		}

		defaultValue, ok := defaultValues[field.schemaType]
		if ok {
			field.schemaType = STRING
			*schema = *PrimitiveSchema(field.schemaType).Schema()

			if field.exampleValue == nil {
				// if exampleValue is not defined by the user,
				// we will force an example with a correct value
				// (eg: int->"0", bool:"false")
				field.exampleValue = defaultValue
			}
		}
	}

	if ps.field.Doc != nil {
		schema.Description = strings.TrimSpace(ps.field.Doc.Text())
	}

	if schema.Description == "" && ps.field.Comment != nil {
		schema.Description = strings.TrimSpace(ps.field.Comment.Text())
	}

	if ps.tag.Get(readOnlyTag) == "true" {
		readOnly := true
		schema.ReadOnly = &readOnly
	}

	defaultTagValue := ps.tag.Get(defaultTag)
	if defaultTagValue != "" {
		value, err := defineType(field.schemaType, defaultTagValue)
		if err != nil {
			return err
		}

		schema.Default = yamlNodeFrom(value)
	}

	if field.exampleValue != nil {
		schema.Example = yamlNodeFrom(field.exampleValue)
	}

	if field.schemaType != ARRAY && field.formatType != "" {
		schema.Format = field.formatType
	}

	extensionsTagValue := ps.tag.Get(extensionsTag)
	if extensionsTagValue != "" {
		schema.Extensions = schemaExtensions(setExtensionParam(extensionsTagValue))
	}

	varNamesTag := ps.tag.Get("x-enum-varnames")
	if varNamesTag != "" {
		varNames := strings.Split(varNamesTag, ",")
		if len(varNames) != len(field.enums) {
			return fmt.Errorf("invalid count of x-enum-varnames. expected %d, got %d", len(field.enums), len(varNames))
		}

		field.enumVarNames = nil

		for _, v := range varNames {
			field.enumVarNames = append(field.enumVarNames, v)
		}

		if field.schemaType == ARRAY {
			// Add the var names in the items schema
			itemSchema := schema.Items.A.Schema()
			if itemSchema.Extensions == nil {
				itemSchema.Extensions = orderedmap.New[string, *yaml.Node]()
			}
			itemSchema.Extensions.Set(enumVarNamesExtension, yamlNodeFrom(field.enumVarNames))
		} else {
			// Add to top level schema
			if schema.Extensions == nil {
				schema.Extensions = orderedmap.New[string, *yaml.Node]()
			}
			schema.Extensions.Set(enumVarNamesExtension, yamlNodeFrom(field.enumVarNames))
		}
	}

	var oneOfSchemas []*base.SchemaProxy
	oneOfTagValue := ps.tag.Get(oneOfTag)
	if oneOfTagValue != "" {
		oneOfTypes := strings.Split((oneOfTagValue), ",")
		for _, oneOfType := range oneOfTypes {
			oneOfSchema, err := ps.p.getTypeSchema(oneOfType, ps.file, true)
			if err != nil {
				return fmt.Errorf("can't find oneOf type %q: %v", oneOfType, err)
			}
			oneOfSchemas = append(oneOfSchemas, oneOfSchema)
		}
	}

	elemSchema := schema

	if field.schemaType == ARRAY {
		// For Array only
		schema.MaxItems = intPtrToInt64Ptr(field.maxItems)
		schema.MinItems = intPtrToInt64Ptr(field.minItems)
		schema.UniqueItems = &field.unique

		// A $ref element (e.g. []dto.Customer) must stay a $ref: item-level
		// scalar validations don't apply to a referenced component, and
		// resolving+mutating it would both inline the ref and corrupt the
		// shared definition.
		if schema.Items != nil && schema.Items.A != nil && schema.Items.A.IsReference() {
			return nil
		}

		elemSchema = schema.Items.A.Schema()
		if elemSchema == nil {
			elemSchema = ps.p.getSchemaByRef(schema.Items.A.GetReference())
		}

		if field.formatType != "" {
			elemSchema.Format = field.formatType
		}
	}

	elemSchema.Maximum = intPtrToFloat64Ptr(field.maximum)
	elemSchema.Minimum = intPtrToFloat64Ptr(field.minimum)
	elemSchema.MultipleOf = intPtrToFloat64Ptr(field.multipleOf)
	elemSchema.MaxLength = intPtrToInt64Ptr(field.maxLength)
	elemSchema.MinLength = intPtrToInt64Ptr(field.minLength)
	for _, e := range field.enums {
		elemSchema.Enum = append(elemSchema.Enum, yamlNodeFrom(e))
	}
	elemSchema.Pattern = field.pattern
	elemSchema.OneOf = oneOfSchemas

	if field.schemaType == ARRAY {
		// Re-wrap the mutated item schema so the array's Items proxy is built
		// fresh from the fully-assembled element schema (build-once rule).
		schema.Items = &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(elemSchema)}
	}

	return nil
}

func getIntTag(structTag reflect.StructTag, tagName string) (*int, error) {
	strValue := structTag.Get(tagName)
	if strValue == "" {
		return nil, nil
	}

	value, err := strconv.Atoi(strValue)
	if err != nil {
		return nil, fmt.Errorf("can't parse numeric value of %q tag: %v", tagName, err)
	}

	return &value, nil
}

func (sf *structField) parseValidTags(validTag string) {

	// `validate:"required,max=10,min=1"`
	// ps. required checked by IsRequired().
	for _, val := range strings.Split(validTag, ",") {
		var (
			valValue string
			keyVal   = strings.Split(val, "=")
		)

		switch len(keyVal) {
		case 1:
		case 2:
			valValue = strings.ReplaceAll(strings.ReplaceAll(keyVal[1], utf8HexComma, ","), utf8Pipe, "|")
		default:
			continue
		}

		switch keyVal[0] {
		case "max", "lte":
			sf.setMax(valValue)
		case "min", "gte":
			sf.setMin(valValue)
		case "oneof":
			if strings.Contains(validTag, "swaggerIgnore") {
				continue
			}

			sf.setOneOf(valValue)
		case "unique":
			if sf.schemaType == ARRAY {
				sf.unique = true
			}
		case "dive":
			// ignore dive
			return
		default:
			continue
		}
	}
}

func (sf *structField) parseEnumTags(enumTag string) error {
	enumType := sf.schemaType
	if sf.schemaType == ARRAY {
		enumType = sf.arrayType
	}

	sf.enums = nil

	for _, e := range strings.Split(enumTag, ",") {
		value, err := defineType(enumType, e)
		if err != nil {
			return err
		}

		sf.enums = append(sf.enums, value)
	}

	return nil
}

func (ps *tagBaseFieldParser) ShouldSkip() bool {
	// Skip non-exported fields.
	if ps.field.Names != nil && !ast.IsExported(ps.field.Names[0].Name) {
		return true
	}

	if ps.field.Tag == nil {
		return false
	}

	ignoreTag := ps.tag.Get(swaggerIgnoreTag)
	if strings.EqualFold(ignoreTag, "true") {
		return true
	}

	// `json:"-"` explicitly excludes the field. Absence of a json tag falls
	// through so FieldName() can pick up form/header tags or use the
	// configured naming strategy on the Go field name.
	return ps.isJsonIgnored()
}

func (ps *tagBaseFieldParser) isJsonIgnored() bool {
	if ps.field.Tag == nil {
		return false
	}
	name := strings.TrimSpace(strings.Split(ps.tag.Get(jsonTag), ",")[0])
	return name == "-"
}

// FieldNames returns the property names for the field. A declaration with
// multiple names (e.g. `Space, Local string`, as in encoding/xml.Name) yields
// one property per name, matching v1's FieldNames behavior.
func (ps *tagBaseFieldParser) FieldNames() ([]string, error) {
	if len(ps.field.Names) <= 1 {
		// json:"tag,hoge"
		if name := ps.JsonName(); name != "" {
			return []string{name}, nil
		}

		// use "form" tag over json tag
		if name := ps.FormName(); name != "" {
			return []string{name}, nil
		}

		if len(ps.field.Names) == 0 {
			return nil, nil
		}
	}

	names := make([]string, 0, len(ps.field.Names))
	for _, name := range ps.field.Names {
		names = append(names, ps.applyPropNamingStrategy(name.Name))
	}

	return names, nil
}

func (ps *tagBaseFieldParser) applyPropNamingStrategy(name string) string {

	switch ps.p.PropNamingStrategy {
	case SnakeCase:
		return toSnakeCase(name)
	case PascalCase:
		return name
	default:
		return toLowerCamelCase(name)
	}
}

func (ps *tagBaseFieldParser) FormName() string {
	if ps.field.Tag != nil {
		name := strings.TrimSpace(strings.Split(ps.tag.Get(formTag), ",")[0])
		if name != "-" {
			return name
		}
	}
	return ""
}

func (ps *tagBaseFieldParser) JsonName() string {
	if ps.field.Tag != nil {
		name := strings.TrimSpace(strings.Split(ps.tag.Get(jsonTag), ",")[0])
		if name != "-" {
			return name
		}
	}
	return ""
}

func (ps *tagBaseFieldParser) IsRequired() (bool, error) {
	if ps.field.Tag == nil {
		return false, nil
	}

	bindingTag := ps.tag.Get(bindingTag)
	if bindingTag != "" {
		for _, val := range strings.Split(bindingTag, ",") {
			switch val {
			case requiredLabel:
				return true, nil
			case optionalLabel:
				return false, nil
			}
		}
	}

	validateTag := ps.tag.Get(validateTag)
	if validateTag != "" {
		for _, val := range strings.Split(validateTag, ",") {
			switch val {
			case requiredLabel:
				return true, nil
			case optionalLabel:
				return false, nil
			}
		}
	}

	jsonTag := ps.tag.Get(jsonTag)
	if jsonTag != "" {
		for _, val := range strings.Split(jsonTag, ",") {
			if val == omitEmptyLabel || val == omitZeroLabel {
				return false, nil
			}
		}
	}

	// Pointer types are inherently optional.
	if _, ok := ps.field.Type.(*ast.StarExpr); ok {
		return false, nil
	}

	return ps.p.RequiredByDefault, nil
}
