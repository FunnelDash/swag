package swag

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"
)

// nodeFromValue converts a decoded scalar/composite (from defineType,
// json.Unmarshal, yaml.Unmarshal, ...) into the *yaml.Node that libopenapi uses
// for example/default/enum/extension values.
func nodeFromValue(v interface{}) *yaml.Node {
	if v == nil {
		return nil
	}
	n := &yaml.Node{}
	if err := n.Encode(v); err != nil {
		return nil
	}
	return n
}

// extensionsFromMap converts the map produced by setExtensionParam into the
// ordered *yaml.Node map libopenapi renders. Keys are sorted so regeneration is
// deterministic.
func extensionsFromMap(m map[string]interface{}) *orderedmap.Map[string, *yaml.Node] {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	om := orderedmap.New[string, *yaml.Node]()
	for _, k := range keys {
		om.Set(k, nodeFromValue(m[k]))
	}
	return om
}

// Operation describes a single API operation on a path.
// For more information: https://github.com/swaggo/swag#api-operation
type Operation struct {
	parser              *Parser
	codeExampleFilesDir string
	v3.Operation
	RouterProperties  []RouteProperties
	responseMimeTypes []string
}

// NewOperation returns a new instance of Operation.
func NewOperation(parser *Parser, options ...func(*Operation)) *Operation {
	op := v3.Operation{
		Responses: &v3.Responses{
			Codes:      orderedmap.New[string, *v3.Response](),
			Extensions: orderedmap.New[string, *yaml.Node](),
		},
	}

	operation := &Operation{
		parser:    parser,
		Operation: op,
	}

	for _, option := range options {
		option(operation)
	}

	return operation
}

// SetCodeExampleFilesDirectory sets the directory to search for codeExamples.
func SetCodeExampleFilesDirectory(directoryPath string) func(*Operation) {
	return func(o *Operation) {
		o.codeExampleFilesDir = directoryPath
	}
}

// ParseComment parses comment for given comment string and returns error if error occurs.
func (o *Operation) ParseComment(comment string, astFile *ast.File) error {
	commentLine := strings.TrimSpace(strings.TrimLeft(comment, "/"))
	if len(commentLine) == 0 {
		return nil
	}

	fields := FieldsByAnySpace(commentLine, 2)
	attribute := fields[0]
	lowerAttribute := strings.ToLower(attribute)
	var lineRemainder string
	if len(fields) > 1 {
		lineRemainder = fields[1]
	}
	switch lowerAttribute {
	case descriptionAttr:
		o.ParseDescriptionComment(lineRemainder)
	case descriptionMarkdownAttr:
		commentInfo, err := getMarkdownForTag(lineRemainder, o.parser.markdownFileDir)
		if err != nil {
			return err
		}

		o.ParseDescriptionComment(string(commentInfo))
	case summaryAttr:
		o.Summary = lineRemainder
	case idAttr:
		o.OperationId = lineRemainder
	case tagsAttr:
		o.ParseTagsComment(lineRemainder)
	case acceptAttr:
		return o.ParseAcceptComment(lineRemainder)
	case produceAttr:
		return o.ParseProduceComment(lineRemainder)
	case paramAttr:
		return o.ParseParamComment(lineRemainder, astFile)
	case successAttr, failureAttr, responseAttr:
		return o.ParseResponseComment(lineRemainder, astFile)
	case headerAttr:
		return o.ParseResponseHeaderComment(lineRemainder, astFile)
	case routerAttr:
		return o.ParseRouterComment(lineRemainder)
	case securityAttr:
		return o.ParseSecurityComment(lineRemainder)
	case deprecatedAttr:
		deprecated := true
		o.Deprecated = &deprecated
	case xCodeSamplesAttr, xCodeSamplesAttrOriginal:
		return o.ParseCodeSample(attribute, commentLine, lineRemainder)
	case "@servers.url":
		return o.ParseServerURLComment(lineRemainder)
	case "@servers.description":
		return o.ParseServerDescriptionComment(lineRemainder)
	default:
		return o.ParseMetadata(attribute, lowerAttribute, lineRemainder)
	}

	return nil
}

// ParseDescriptionComment parses the description comment and sets it to the operation.
func (o *Operation) ParseDescriptionComment(lineRemainder string) {
	if o.Description == "" {
		o.Description = lineRemainder

		return
	}

	o.Description += "\n" + lineRemainder
}

// ParseMetadata godoc.
func (o *Operation) ParseMetadata(attribute, lowerAttribute, lineRemainder string) error {
	// parsing specific meta data extensions
	if strings.HasPrefix(lowerAttribute, "@x-") {
		if len(lineRemainder) == 0 {
			return fmt.Errorf("annotation %s need a value", attribute)
		}

		var valueJSON any

		err := json.Unmarshal([]byte(lineRemainder), &valueJSON)
		if err != nil {
			return fmt.Errorf("annotation %s need a valid json value. error: %s", attribute, err.Error())
		}

		if o.Responses.Extensions == nil {
			o.Responses.Extensions = orderedmap.New[string, *yaml.Node]()
		}
		o.Responses.Extensions.Set(attribute[1:], nodeFromValue(valueJSON))
		return nil
	}

	return nil
}

// ParseTagsComment parses comment for given `tag` comment string.
func (o *Operation) ParseTagsComment(commentLine string) {
	for _, tag := range strings.Split(commentLine, ",") {
		o.Tags = append(o.Tags, strings.TrimSpace(tag))
	}
}

// ParseAcceptComment parses comment for given `accept` comment string.
func (o *Operation) ParseAcceptComment(commentLine string) error {
	const errMessage = "could not parse accept comment"

	validTypes, err := parseMimeTypeList(commentLine, "%v accept type can't be accepted")
	if err != nil {
		return fmt.Errorf("%s: %w", errMessage, err)
	}

	if o.RequestBody == nil {
		o.RequestBody = &v3.RequestBody{}
	}

	if o.RequestBody.Content == nil {
		o.RequestBody.Content = orderedmap.New[string, *v3.MediaType]()
	}

	for _, value := range validTypes {
		// skip correctly setup types like application/json
		if o.RequestBody.Content.GetOrZero(value) != nil {
			continue
		}

		schema := &base.Schema{}

		switch value {
		case "application/json", "multipart/form-data", "text/xml":
			schema.Type = []string{OBJECT}
		case "image/png",
			"image/jpeg",
			"image/gif",
			"application/octet-stream",
			"application/pdf",
			"application/msexcel",
			"application/zip",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation":
			schema.Type = []string{STRING}
			schema.Format = "binary"
		default:
			schema.Type = []string{STRING}
		}

		o.RequestBody.Content.Set(value, &v3.MediaType{Schema: base.CreateSchemaProxy(schema)})
	}

	return nil
}

// ParseProduceComment parses comment for given `produce` comment string.
func (o *Operation) ParseProduceComment(commentLine string) error {
	const errMessage = "could not parse produce comment"

	validTypes, err := parseMimeTypeList(commentLine, "%v produce type can't be accepted")
	if err != nil {
		return fmt.Errorf("%s: %w", errMessage, err)
	}

	o.responseMimeTypes = validTypes

	return nil
}

// ProcessProduceComment processes the previously parsed produce comment.
func (o *Operation) ProcessProduceComment() error {
	const errMessage = "could not process produce comment"

	if o.Responses == nil {
		return nil
	}

	for _, value := range o.responseMimeTypes {
		if o.Responses.Codes == nil {
			o.Responses.Codes = orderedmap.New[string, *v3.Response]()
		}

		for pair := o.Responses.Codes.First(); pair != nil; pair = pair.Next() {
			key := pair.Key()
			response := pair.Value()

			code, err := strconv.Atoi(key)
			if err != nil {
				return fmt.Errorf("%s: %w", errMessage, err)
			}

			// Status 204 is no content. So we do not need to add content.
			if code == 204 {
				continue
			}

			// As this is a workaround, we need to check if the code is in range.
			// The Produce comment is being deprecated soon.
			if code < 200 || code > 299 {
				continue
			}

			// skip correctly setup types like application/json
			if response.Content != nil && response.Content.GetOrZero(value) != nil {
				continue
			}

			schema := &base.Schema{}

			switch value {
			case "application/json", "multipart/form-data", "text/xml":
				schema.Type = []string{OBJECT}
			case "image/png",
				"image/jpeg",
				"image/gif",
				"application/octet-stream",
				"application/pdf",
				"application/msexcel",
				"application/zip",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation":
				schema.Type = []string{STRING}
				schema.Format = "binary"
			default:
				schema.Type = []string{STRING}
			}

			if response.Content == nil {
				response.Content = orderedmap.New[string, *v3.MediaType]()
			}

			response.Content.Set(value, &v3.MediaType{Schema: base.CreateSchemaProxy(schema)})
		}
	}

	return nil
}

// parseMimeTypeList parses a list of MIME Types for a comment like
// `produce` (`Content-Type:` response header) or
// `accept` (`Accept:` request header).
func parseMimeTypeList(mimeTypeList string, format string) ([]string, error) {
	var result []string
	for _, typeName := range strings.Split(mimeTypeList, ",") {
		typeName = strings.TrimSpace(typeName)

		if mimeTypePattern.MatchString(typeName) {
			result = append(result, typeName)

			continue
		}

		aliasMimeType, ok := mimeTypeAliases[typeName]
		if !ok {
			return nil, fmt.Errorf(format, typeName)
		}

		result = append(result, aliasMimeType)
	}

	return result, nil
}

// ParseParamComment parses params return []string of param properties
// E.g. @Param	queryText		formData	      string	  true		        "The email for login"
//
//	[param name]    [paramType] [data type]  [is mandatory?]   [Comment]
//
// E.g. @Param   some_id     path    int     true        "Some ID".
func (o *Operation) ParseParamComment(commentLine string, astFile *ast.File) error {
	matches := paramPattern.FindStringSubmatch(commentLine)
	if len(matches) != 6 {
		return fmt.Errorf("missing required param comment parameters \"%s\"", commentLine)
	}

	name := matches[1]
	paramType := matches[2]
	refType := TransToValidSchemeType(matches[3])

	// Detect refType
	objectType := OBJECT

	if strings.HasPrefix(refType, "[]") {
		objectType = ARRAY
		refType = strings.TrimPrefix(refType, "[]")
		refType = TransToValidSchemeType(refType)
	} else if IsPrimitiveType(refType) ||
		paramType == "formData" && refType == "file" {
		objectType = PRIMITIVE
	}

	var enums []*yaml.Node
	if !IsPrimitiveType(refType) {
		schema, _ := o.parser.getTypeSchema(refType, astFile, false)
		if schema != nil {
			if s := schema.Schema(); s != nil && s.Enum != nil {
				// schema.Type != ARRAY
				if objectType == OBJECT {
					objectType = PRIMITIVE
				}
				refType = TransToValidSchemeType(s.Type[0])
				enums = s.Enum
			}
		}
	}

	requiredText := strings.ToLower(matches[4])
	required := requiredText == "true" || requiredText == requiredLabel
	description := matches[5]

	param := createParameter(paramType, description, name, objectType, refType, required, enums, o.parser.collectionFormatInQuery)

	switch paramType {
	case "path", "header":
		switch objectType {
		case ARRAY:
			if !IsPrimitiveType(refType) {
				return fmt.Errorf("%s is not supported array type for %s", refType, paramType)
			}
		case OBJECT:
			return fmt.Errorf("%s is not supported type for %s", refType, paramType)
		}
	case "query":
		switch objectType {
		case ARRAY:
			if !IsPrimitiveType(refType) && !(refType == "file" && paramType == "formData") {
				return fmt.Errorf("%s is not supported array type for %s", refType, paramType)
			}
		case PRIMITIVE:
			break
		case OBJECT:
			schema, err := o.parser.getTypeSchema(refType, astFile, false)
			if err != nil {
				return err
			}

			s := schema.Schema()

			if s == nil || s.Properties == nil || s.Properties.Len() == 0 {
				// A generic/aliased query type can resolve (via a .swaggo
				// override) to an array or primitive rather than a struct — emit
				// it as a single named parameter instead of silently dropping it.
				if s != nil && len(s.Type) > 0 && s.Type[0] != OBJECT {
					itemParam := createParameter(paramType, description, name, s.Type[0], "", required, enums, o.parser.collectionFormatInQuery)
					itemParam.Schema = schema
					o.Parameters = append(o.Parameters, &itemParam)
					return nil
				}
				o.parser.debug.Printf("skip query param %s: %s resolved to an empty object", name, refType)
				return nil
			}

			// Iterate properties in a stable order: ranging the map directly
			// emits parameters in Go's randomized map order, so every
			// regeneration reshuffles the parameter list and churns the spec.
			names := make([]string, 0, s.Properties.Len())
			for pair := s.Properties.First(); pair != nil; pair = pair.Next() {
				names = append(names, pair.Key())
			}
			sort.Strings(names)

			// Query params are optional by default — a filter you may omit — so
			// --requiredByDefault (a request-body notion, and inescapable for
			// form-only fields that can't carry json:omitempty) must not leak in.
			// Only an explicit binding/validate "required" marks a query param.
			reqFields := explicitlyRequiredQueryFields(o.parser, refType, astFile)

			for _, name := range names {
				item := s.Properties.GetOrZero(name)
				prop := flattenQueryPropSchema(o.parser, item)
				if prop == nil || len(prop.Type) == 0 {
					o.parser.debug.Printf("skip field [%s] in %s: type does not resolve to a primitive for %s (add a .swaggo override or swaggertype tag)", name, refType, paramType)
					continue
				}

				// A typed-enum field resolves to a $ref whose schema holds the enum
				// values (and default); flattenQueryPropSchema inlined them above.
				// When no example was given, the first enum value serves as one so
				// the parameter still documents a concrete value.
				if len(prop.Enum) > 0 && prop.Example == nil {
					prop.Example = prop.Enum[0]
				}

				itemParam := param // Avoid shadowed variable which could cause side effects to o.Operation.Parameters

				switch {
				case prop.Type[0] == ARRAY && prop.Items != nil && prop.Items.A != nil:
					// Items may be a primitive (has a schema type) or a $ref to a
					// component (an enum element, whose inline schema is nil).
					// Either way the parameter schema is replaced by prop below, so
					// an empty item type here is fine — it just must not panic on
					// the ref.
					itemType := ""
					if isc := prop.Items.A.Schema(); isc != nil && len(isc.Type) > 0 {
						itemType = isc.Type[0]
					}
					itemParam = createParameter(paramType, prop.Description, name, ARRAY, itemType, reqFields[name], enums, o.parser.collectionFormatInQuery)

				case IsSimplePrimitiveType(prop.Type[0]):
					itemParam = createParameter(paramType, prop.Description, name, PRIMITIVE, prop.Type[0], reqFields[name], enums, o.parser.collectionFormatInQuery)
				default:
					o.parser.debug.Printf("skip field [%s] in %s is not supported type for %s", name, refType, paramType)

					continue
				}

				applyParamSerialization(&itemParam, prop)
				itemParam.Schema = base.CreateSchemaProxy(prop)

				o.Parameters = append(o.Parameters, &itemParam)
			}

			return nil
		}
	case "body", "formData":
		if paramType == "formData" && objectType == OBJECT {
			return o.expandFormDataStruct(refType, description, astFile)
		}
		if objectType == PRIMITIVE {
			schema := PrimitiveSchema(refType)

			err := o.parseParamAttributeForBody(commentLine, objectType, refType, schema.Schema())
			if err != nil {
				return err
			}

			o.fillRequestBody(name, schema, required, description, true, paramType == "formData")

			return nil

		}

		schema, err := o.parseAPIObjectSchema(commentLine, objectType, refType, astFile)
		if err != nil {
			return err
		}

		err = o.parseParamAttributeForBody(commentLine, objectType, refType, schema.Schema())
		if err != nil {
			return err
		}
		o.fillRequestBody(name, schema, required, description, false, paramType == "formData")

		return nil

	default:
		return fmt.Errorf("%s is not supported paramType", paramType)
	}

	err := o.parseParamAttribute(commentLine, objectType, refType, &param)
	if err != nil {
		return err
	}

	o.Parameters = append(o.Parameters, &param)

	return nil
}

// expandFormDataStruct expands the struct named refType into the operation's
// form request body: each form-tagged field becomes a property of a single
// object schema, a multipart.FileHeader field renders as {string, binary}, and
// required comes from the field's binding/validate tags. The content type is
// multipart/form-data when the operation @Accepts it or carries a file part
// (both need multipart), otherwise application/x-www-form-urlencoded. This
// replaces the per-field @Param formData lines with one struct reference whose
// shape is driven by the DTO's own tags and types.
func (o *Operation) expandFormDataStruct(refType, description string, astFile *ast.File) error {
	def := o.parser.packages.FindTypeSpec(refType, astFile)
	if def == nil || def.TypeSpec == nil {
		return fmt.Errorf("formData struct %s not found", refType)
	}
	st, ok := def.TypeSpec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return fmt.Errorf("formData param %s is not a struct", refType)
	}

	obj := &base.Schema{
		Type:       []string{OBJECT},
		Properties: orderedmap.New[string, *base.SchemaProxy](),
	}
	var required []string
	hasFile := false

	for _, field := range st.Fields.List {
		if field.Tag == nil || len(field.Names) == 0 {
			continue
		}
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		propName := strings.TrimSpace(strings.Split(tag.Get("form"), ",")[0])
		if propName == "" || propName == "-" {
			continue
		}
		if isMultipartFileType(field.Type) {
			hasFile = true
		}
		propSchema := o.formDataFieldSchema(field.Type, astFile)
		if ps := propSchema.Schema(); ps != nil {
			if field.Doc != nil {
				ps.Description = strings.TrimSpace(field.Doc.Text())
			} else if field.Comment != nil {
				ps.Description = strings.TrimSpace(field.Comment.Text())
			}
		}
		obj.Properties.Set(propName, propSchema)
		if tagHasRequired(tag.Get("binding")) || tagHasRequired(tag.Get("validate")) {
			required = append(required, propName)
		}
	}
	if len(required) == 0 {
		required = nil // omit `required: []` rather than emit an empty array
	}
	obj.Required = required

	contentType := "application/x-www-form-urlencoded"
	if hasFile || (o.RequestBody != nil && o.RequestBody.Content != nil && o.RequestBody.Content.GetOrZero("multipart/form-data") != nil) {
		contentType = "multipart/form-data"
	}

	if o.RequestBody == nil {
		o.RequestBody = &v3.RequestBody{}
	}
	if o.RequestBody.Content == nil {
		o.RequestBody.Content = orderedmap.New[string, *v3.MediaType]()
	}
	mediaType := o.RequestBody.Content.GetOrZero(contentType)
	if mediaType == nil {
		mediaType = &v3.MediaType{}
		o.RequestBody.Content.Set(contentType, mediaType)
	}
	mediaType.Schema = base.CreateSchemaProxy(obj)
	if description != "" {
		o.RequestBody.Description = description
	}
	return nil
}

// isMultipartFileType reports whether expr is a (possibly pointer/slice)
// mime/multipart.FileHeader — the type gin binds an uploaded file to.
func isMultipartFileType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return isMultipartFileType(t.X)
	case *ast.ArrayType:
		return isMultipartFileType(t.Elt)
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		return ok && pkg.Name == "multipart" && t.Sel.Name == "FileHeader"
	}
	return false
}

// formDataFieldSchema maps a struct field's Go type to its form-property
// schema: a multipart.FileHeader is a binary string, a slice is an array of the
// element schema, a Go primitive maps to its OpenAPI scalar, and a named type
// resolves through getTypeSchema (so .swaggo overrides apply). Unknown types
// fall back to string — a form value is a string on the wire.
func (o *Operation) formDataFieldSchema(expr ast.Expr, astFile *ast.File) *base.SchemaProxy {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return o.formDataFieldSchema(t.X, astFile)
	case *ast.ArrayType:
		return base.CreateSchemaProxy(&base.Schema{
			Type:  []string{ARRAY},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: o.formDataFieldSchema(t.Elt, astFile)},
		})
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		if ok && pkg.Name == "multipart" && t.Sel.Name == "FileHeader" {
			return base.CreateSchemaProxy(&base.Schema{Type: []string{STRING}, Format: "binary"})
		}
		if ok {
			if s, err := o.parser.getTypeSchema(pkg.Name+"."+t.Sel.Name, astFile, false); err == nil && s != nil {
				return s
			}
		}
	case *ast.Ident:
		if IsGolangPrimitiveType(t.Name) {
			return PrimitiveSchema(TransToValidSchemeType(t.Name))
		}
		if s, err := o.parser.getTypeSchema(t.Name, astFile, false); err == nil && s != nil {
			return s
		}
	}
	return PrimitiveSchema(STRING)
}

func (o *Operation) fillRequestBody(name string, schema *base.SchemaProxy, required bool, description string, primitive, formData bool) {
	if o.RequestBody == nil {
		o.RequestBody = &v3.RequestBody{}
		o.RequestBody.Content = orderedmap.New[string, *v3.MediaType]()

		if primitive && !formData {
			o.RequestBody.Content.Set("text/plain", &v3.MediaType{})
		} else if formData {
			o.RequestBody.Content.Set("application/x-www-form-urlencoded", &v3.MediaType{})
		} else {
			o.RequestBody.Content.Set("application/json", &v3.MediaType{})
		}
	}

	// Required renders even when false, so leave it nil for an optional body
	// (matching the previous model's omit-when-false output).
	if required {
		t := true
		o.RequestBody.Required = &t
	} else {
		o.RequestBody.Required = nil
	}

	// Append description to existing description if this is not the first body
	if o.RequestBody.Description != "" && description != "" {
		o.RequestBody.Description += " | " + description
	} else if description != "" {
		o.RequestBody.Description = description
	}

	// Handle oneOf merging for request body schemas
	contentType := "application/json"
	if primitive && !formData {
		contentType = "text/plain"
	} else if formData {
		contentType = "application/x-www-form-urlencoded"
	}

	mediaType := o.RequestBody.Content.GetOrZero(contentType)
	if mediaType == nil {
		mediaType = &v3.MediaType{}
		o.RequestBody.Content.Set(contentType, mediaType)
	}
	if schema.IsReference() {
		// A schema $ref carries the body's description as a sibling of $ref.
		// (The previous model also stamped a Summary onto the ref; libopenapi
		// schema refs have no summary sibling, so only the description is kept.)
		if description != "" {
			schema = base.CreateSchemaProxyRefWithSchema(schema.GetReference(), &base.Schema{Description: description})
		}
	} else if s := schema.Schema(); s != nil {
		s.Title = name
	}
	if mediaType.Schema == nil || isAcceptPlaceholderSchema(mediaType.Schema) {
		// No schema yet, or only the empty {type:object}/{type:string} placeholder
		// that @Accept seeds — the real body schema replaces it rather than being
		// oneOf-merged into a spurious oneOf:[{type:object}, $ref] (swaggo/swag#2086).
		mediaType.Schema = schema
	} else if existing := mediaType.Schema; existing.IsReference() || existing.Schema() == nil || existing.Schema().OneOf == nil {
		// If there's an existing schema that doesn't have oneOf, create a oneOf schema
		mediaType.Schema = base.CreateSchemaProxy(&base.Schema{
			OneOf: []*base.SchemaProxy{existing, schema},
		})
	} else {
		// If there's already a oneOf schema, append to it
		existing.Schema().OneOf = append(existing.Schema().OneOf, schema)
	}
}

// isAcceptPlaceholderSchema reports whether s is the bare, type-only schema that
// ProcessAcceptComment seeds for a media type (a {type:object}/{type:string}
// with no properties, items, refs, or composition). Such a placeholder should be
// overwritten by a real body schema, not merged with it.
func isAcceptPlaceholderSchema(s *base.SchemaProxy) bool {
	if s == nil || s.IsReference() {
		return false
	}
	sp := s.Schema()
	if sp == nil || len(sp.Type) == 0 {
		return false
	}
	return (sp.Properties == nil || sp.Properties.Len() == 0) &&
		sp.Items == nil &&
		len(sp.OneOf) == 0 &&
		sp.AdditionalProperties == nil
}

func (o *Operation) parseParamAttribute(comment, objectType, schemaType string, param *v3.Parameter) error {
	if param == nil {
		return fmt.Errorf("cannot parse empty parameter for comment: %s", comment)
	}

	schemaType = TransToValidSchemeType(schemaType)

	sc := param.Schema.Schema()

	for attrKey, re := range regexAttributes {
		attr, err := findAttr(re, comment)
		if err != nil {
			continue
		}

		switch attrKey {
		case enumsTag:
			err = setEnumParam(sc, attr, objectType, schemaType)
		case minimumTag, maximumTag:
			err = setNumberParam(sc, attrKey, schemaType, attr, comment)
		case defaultTag:
			err = setDefault(sc, schemaType, attr)
		case minLengthTag, maxLengthTag:
			err = setStringParam(sc, attrKey, schemaType, attr, comment)
		case formatTag:
			sc.Format = attr
		case exampleTag:
			val, err := defineType(schemaType, attr)
			if err != nil {
				continue // Don't set a example value if it's not valid
			}

			param.Example = nodeFromValue(val)
		case schemaExampleTag:
			err = setSchemaExample(sc, schemaType, attr)
		case extensionsTag:
			sc.Extensions = extensionsFromMap(setExtensionParam(attr))
		case collectionFormatTag:
			err = setCollectionFormatParam(param, attrKey, objectType, attr, comment)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (o *Operation) parseParamAttributeForBody(comment, objectType, schemaType string, param *base.Schema) error {
	schemaType = TransToValidSchemeType(schemaType)

	for attrKey, re := range regexAttributes {
		attr, err := findAttr(re, comment)
		if err != nil {
			continue
		}

		switch attrKey {
		case enumsTag:
			err = setEnumParam(param, attr, objectType, schemaType)
		case minimumTag, maximumTag:
			err = setNumberParam(param, attrKey, schemaType, attr, comment)
		case defaultTag:
			err = setDefault(param, schemaType, attr)
		case minLengthTag, maxLengthTag:
			err = setStringParam(param, attrKey, schemaType, attr, comment)
		case formatTag:
			param.Format = attr
		case exampleTag:
			err = setSchemaExample(param, schemaType, attr)
		case schemaExampleTag:
			err = setSchemaExample(param, schemaType, attr)
		case extensionsTag:
			param.Extensions = extensionsFromMap(setExtensionParam(attr))
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func setCollectionFormatParam(param *v3.Parameter, name, schemaType, attr, commentLine string) error {
	if schemaType == ARRAY {
		param.Style = TransToValidParamStyle(attr, param.In)
		// Every collection format except `multi` (repeated params) is a single
		// delimited value, i.e. explode:false. Without this, `style: form` alone
		// defaults to explode:true and misdescribes a csv/ssv/pipes param.
		if param.Style != "" && attr != "multi" {
			f := false
			param.Explode = &f
		}
		return nil
	}

	return fmt.Errorf("%s is attribute to set to an array. comment=%s got=%s", name, commentLine, schemaType)
}

func setSchemaExample(param *base.Schema, schemaType string, value string) error {
	val, err := defineType(schemaType, value)
	if err != nil {
		return nil // Don't set a example value if it's not valid
	}

	// skip schema
	if param == nil {
		return nil
	}

	switch v := val.(type) {
	case string:
		//  replaces \r \n \t in example string values.
		param.Example = nodeFromValue(strings.NewReplacer(`\r`, "\r", `\n`, "\n", `\t`, "\t").Replace(v))
	default:
		param.Example = nodeFromValue(val)
	}

	return nil
}

func setExampleParameter(param *v3.Parameter, schemaType string, value string) error {
	val, err := defineType(schemaType, value)
	if err != nil {
		return nil // Don't set a example value if it's not valid
	}

	param.Example = nodeFromValue(val)

	return nil
}

func setStringParam(param *base.Schema, name, schemaType, attr, commentLine string) error {
	if schemaType != STRING {
		return fmt.Errorf("%s is attribute to set to a number. comment=%s got=%s", name, commentLine, schemaType)
	}

	n, err := strconv.Atoi(attr)
	if err != nil {
		return fmt.Errorf("%s is allow only a number got=%s", name, attr)
	}

	v := int64(n)
	switch name {
	case minLengthTag:
		param.MinLength = &v
	case maxLengthTag:
		param.MaxLength = &v
	}

	return nil
}

func setDefault(param *base.Schema, schemaType string, value string) error {
	val, err := defineType(schemaType, value)
	if err != nil {
		return nil // Don't set a default value if it's not valid
	}

	param.Default = nodeFromValue(val)

	return nil
}

func setEnumParam(param *base.Schema, attr, objectType, schemaType string) error {
	for _, e := range strings.Split(attr, ",") {
		e = strings.TrimSpace(e)

		value, err := defineType(schemaType, e)
		if err != nil {
			return err
		}

		switch objectType {
		case ARRAY:
			items := param.Items.A.Schema()
			items.Enum = append(items.Enum, nodeFromValue(value))
		default:
			param.Enum = append(param.Enum, nodeFromValue(value))
		}
	}

	return nil
}

func setNumberParam(param *base.Schema, name, schemaType, attr, commentLine string) error {
	switch schemaType {
	case INTEGER, NUMBER:
		n, err := strconv.Atoi(attr)
		if err != nil {
			return fmt.Errorf("maximum is allow only a number. comment=%s got=%s", commentLine, attr)
		}

		v := float64(n)
		switch name {
		case minimumTag:
			param.Minimum = &v
		case maximumTag:
			param.Maximum = &v
		}

		return nil
	default:
		return fmt.Errorf("%s is attribute to set to a number. comment=%s got=%s", name, commentLine, schemaType)
	}
}

func (o *Operation) parseAPIObjectSchema(commentLine, schemaType, refType string, astFile *ast.File) (*base.SchemaProxy, error) {
	if strings.HasSuffix(refType, ",") && strings.Contains(refType, "[") {
		// regexp may have broken generic syntax. find closing bracket and add it back
		allMatchesLenOffset := strings.Index(commentLine, refType) + len(refType)
		lostPartEndIdx := strings.Index(commentLine[allMatchesLenOffset:], "]")
		if lostPartEndIdx >= 0 {
			refType += commentLine[allMatchesLenOffset : allMatchesLenOffset+lostPartEndIdx+1]
		}
	}

	switch schemaType {
	case OBJECT:
		if !strings.HasPrefix(refType, "[]") {
			return parseObjectSchema(o.parser, refType, astFile)
		}

		refType = refType[2:]

		fallthrough
	case ARRAY:
		schema, err := parseObjectSchema(o.parser, refType, astFile)
		if err != nil {
			return nil, err
		}

		return base.CreateSchemaProxy(&base.Schema{
			Type:  []string{ARRAY},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}), nil

	default:
		return PrimitiveSchema(schemaType), nil
	}
}

// ParseRouterComment parses comment for given `router` comment string.
func (o *Operation) ParseRouterComment(commentLine string) error {
	matches := routerPattern.FindStringSubmatch(commentLine)
	if len(matches) != 3 {
		return fmt.Errorf("can not parse router comment \"%s\"", commentLine)
	}

	signature := RouteProperties{
		Path:       matches[1],
		HTTPMethod: strings.ToUpper(matches[2]),
	}

	if _, ok := allMethod[signature.HTTPMethod]; !ok {
		return fmt.Errorf("invalid method: %s", signature.HTTPMethod)
	}

	o.RouterProperties = append(o.RouterProperties, signature)

	return nil
}

func (o *Operation) ParseServerURLComment(commentLine string) error {
	o.Servers = append(o.Servers, &v3.Server{URL: commentLine})
	return nil
}

func (o *Operation) ParseServerDescriptionComment(commentLine string) error {
	o.Servers[len(o.Servers)-1].Description = commentLine
	return nil
}

// createParameter returns swagger v3.Parameter for given  paramType, description, paramName, schemaType, required.
func createParameter(in, description, paramName, objectType, schemaType string, required bool, enums []*yaml.Node, collectionFormat string) v3.Parameter {
	// //five possible parameter types. 	query, path, body, header, form
	// Required is a *bool that renders even when false, so leave it nil for
	// optional params (matching the previous model's omit-when-false output);
	// only a true value emits `required: true`.
	var req *bool
	if required {
		t := true
		req = &t
	}
	sc := &base.Schema{}

	if in != "body" {
		switch objectType {
		case ARRAY:
			sc.Type = []string{objectType}
			sc.Items = &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{Type: []string{schemaType}})}
			sc.Enum = enums
		case PRIMITIVE, OBJECT:
			sc.Type = []string{schemaType}
			sc.Enum = enums
		}
	}

	return v3.Parameter{
		Description: description,
		Required:    req,
		Name:        paramName,
		In:          in,
		Schema:      base.CreateSchemaProxy(sc),
	}
}

func parseObjectSchema(parser *Parser, refType string, astFile *ast.File) (*base.SchemaProxy, error) {
	switch {
	case refType == NIL:
		return nil, nil
	case refType == INTERFACE:
		return PrimitiveSchema(OBJECT), nil
	case refType == ANY:
		return PrimitiveSchema(OBJECT), nil
	case IsGolangPrimitiveType(refType):
		refType = TransToValidSchemeType(refType)

		return PrimitiveSchema(refType), nil
	case IsPrimitiveType(refType):
		return PrimitiveSchema(refType), nil
	case strings.HasPrefix(refType, "[]"):
		schema, err := parseObjectSchema(parser, refType[2:], astFile)
		if err != nil {
			return nil, err
		}

		return base.CreateSchemaProxy(&base.Schema{
			Type:  []string{ARRAY},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}), nil
	case strings.HasPrefix(refType, "map["):
		// ignore key type
		idx := strings.Index(refType, "]")
		if idx < 0 {
			return nil, fmt.Errorf("invalid type: %s", refType)
		}

		refType = refType[idx+1:]
		if refType == INTERFACE || refType == ANY {
			return base.CreateSchemaProxy(&base.Schema{
				Type:                 []string{OBJECT},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{})},
			}), nil
		}

		schema, err := parseObjectSchema(parser, refType, astFile)
		if err != nil {
			return nil, err
		}

		return base.CreateSchemaProxy(&base.Schema{
			Type:                 []string{OBJECT},
			AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}), nil
	case strings.Contains(refType, "{"):
		return parseCombinedObjectSchema(parser, refType, astFile)
	default:
		if parser != nil { // checking refType has existing in 'TypeDefinitions'
			schema, err := parser.getTypeSchema(refType, astFile, true)
			if err != nil {
				return nil, err
			}

			return schema, nil
		}

		return RefSchema(refType), nil
	}
}

// ParseResponseHeaderComment parses comment for given `response header` comment string.
func (o *Operation) ParseResponseHeaderComment(commentLine string, _ *ast.File) error {
	matches := responsePattern.FindStringSubmatch(commentLine)
	if len(matches) != 5 {
		return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
	}

	header := newHeaderSpec(strings.Trim(matches[2], "{}"), strings.Trim(matches[4], "\""))

	headerKey := strings.TrimSpace(matches[3])

	if strings.EqualFold(matches[1], "all") {
		if o.Responses.Default != nil {
			setResponseHeader(o.Responses.Default, headerKey, header)
		}

		if o.Responses.Codes != nil {
			for pair := o.Responses.Codes.First(); pair != nil; pair = pair.Next() {
				setResponseHeader(pair.Value(), headerKey, header)
			}
		}

		return nil
	}

	for _, codeStr := range strings.Split(matches[1], ",") {
		if strings.EqualFold(codeStr, defaultTag) {
			if o.Responses.Default != nil {
				setResponseHeader(o.Responses.Default, headerKey, header)
			}

			continue
		}

		_, err := strconv.Atoi(codeStr)
		if err != nil {
			return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
		}

		if o.Responses != nil && o.Responses.Codes != nil {
			response := o.Responses.Codes.GetOrZero(codeStr)
			if response != nil {
				setResponseHeader(response, headerKey, header)
				o.Responses.Codes.Set(codeStr, response)
			}
		}
	}

	return nil
}

// setResponseHeader stores header under key on response, lazily creating the
// headers map.
func setResponseHeader(response *v3.Response, key string, header *v3.Header) {
	if response.Headers == nil {
		response.Headers = orderedmap.New[string, *v3.Header]()
	}
	response.Headers.Set(key, header)
}

func newHeaderSpec(schemaType, description string) *v3.Header {
	return &v3.Header{
		Description: description,
		Schema:      base.CreateSchemaProxy(&base.Schema{Type: []string{schemaType}}),
	}
}

// ParseResponseComment parses comment for given `response` comment string.
func (o *Operation) ParseResponseComment(commentLine string, astFile *ast.File) error {
	matches := responsePattern.FindStringSubmatch(commentLine)
	if len(matches) != 5 {
		err := o.ParseEmptyResponseComment(commentLine)
		if err != nil {
			return o.ParseEmptyResponseOnly(commentLine)
		}

		return err
	}

	description := strings.Trim(matches[4], "\"")

	schema, err := o.parseAPIObjectSchema(commentLine, strings.Trim(matches[2], "{}"), strings.TrimSpace(matches[3]), astFile)
	if err != nil {
		return err
	}

	for _, codeStr := range strings.Split(matches[1], ",") {
		if strings.EqualFold(codeStr, defaultTag) {
			codeStr = ""
		} else {
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
			}
			if description == "" {
				description = http.StatusText(code)
			}
		}

		response := &v3.Response{Description: description}

		// Add the schema to all specified response MIME types
		if len(o.responseMimeTypes) > 0 {
			for _, mimeType := range o.responseMimeTypes {
				setResponseSchema(response, mimeType, schema)
			}
		} else {
			// Default to application/json if no MIME types were specified
			setResponseSchema(response, "application/json", schema)
		}

		o.AddResponse(codeStr, response)
	}

	return nil
}

// setResponseSchema sets response schema for given response.
func setResponseSchema(response *v3.Response, mimeType string, schema *base.SchemaProxy) {
	if response.Content == nil {
		response.Content = orderedmap.New[string, *v3.MediaType]()
	}

	response.Content.Set(mimeType, &v3.MediaType{Schema: schema})
}

// ParseEmptyResponseComment parse only comment out status code and description,eg: @Success 200 "it's ok".
func (o *Operation) ParseEmptyResponseComment(commentLine string) error {
	matches := emptyResponsePattern.FindStringSubmatch(commentLine)
	if len(matches) != 3 {
		return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
	}

	description := strings.Trim(matches[2], "\"")

	for _, codeStr := range strings.Split(matches[1], ",") {
		if strings.EqualFold(codeStr, defaultTag) {
			codeStr = ""
		} else {
			_, err := strconv.Atoi(codeStr)
			if err != nil {
				return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
			}
		}

		o.AddResponse(codeStr, newResponseWithDescription(description))
	}

	return nil
}

// AddResponse add a response for a code.
// If the code is already exist, it will merge with the old one:
// 1. The description will be replaced by the new one if the new one is not empty.
// 2. The content schema will be merged using `oneOf` if the new one is not empty.
func (o *Operation) AddResponse(code string, response *v3.Response) {
	if response.Headers == nil {
		response.Headers = orderedmap.New[string, *v3.Header]()
	}

	if o.Responses.Codes == nil {
		o.Responses.Codes = orderedmap.New[string, *v3.Response]()
	}

	res := response
	var prev *v3.Response
	if code != "" {
		prev = o.Responses.Codes.GetOrZero(code)
	} else {
		prev = o.Responses.Default
	}
	if prev != nil { // merge into prev
		res = prev
		if response.Description != "" {
			prev.Description = response.Description
		}
		if response.Content != nil && response.Content.Len() > 0 {
			// responses should only have one content type
			singleKey := ""
			for pair := response.Content.First(); pair != nil; pair = pair.Next() {
				singleKey = pair.Key()
				break
			}

			var prevMediaType *v3.MediaType
			if prev.Content != nil {
				prevMediaType = prev.Content.GetOrZero(singleKey)
			}

			if prevMediaType == nil {
				prev.Content = response.Content
			} else {
				newMediaType := response.Content.GetOrZero(singleKey)
				if newMediaType.Extensions != nil && newMediaType.Extensions.Len() > 0 {
					if prevMediaType.Extensions == nil {
						prevMediaType.Extensions = orderedmap.New[string, *yaml.Node]()
					}
					for pair := newMediaType.Extensions.First(); pair != nil; pair = pair.Next() {
						prevMediaType.Extensions.Set(pair.Key(), pair.Value())
					}
				}
				if newMediaType.Examples != nil && newMediaType.Examples.Len() > 0 {
					if prevMediaType.Examples == nil {
						prevMediaType.Examples = orderedmap.New[string, *base.Example]()
					}
					for pair := newMediaType.Examples.First(); pair != nil; pair = pair.Next() {
						prevMediaType.Examples.Set(pair.Key(), pair.Value())
					}
				}
				if prevSchema := prevMediaType.Schema; prevSchema.IsReference() || prevSchema.Schema() == nil || prevSchema.Schema().OneOf == nil {
					prevMediaType.Schema = base.CreateSchemaProxy(&base.Schema{
						OneOf: []*base.SchemaProxy{prevSchema, newMediaType.Schema},
					})
				} else {
					prevSchema.Schema().OneOf = append(prevSchema.Schema().OneOf, newMediaType.Schema)
				}
			}
		}
	}

	if code != "" {
		o.Responses.Codes.Set(code, res)
	} else {
		o.Responses.Default = res
	}
}

// ParseEmptyResponseOnly parse only comment out status code ,eg: @Success 200.
func (o *Operation) ParseEmptyResponseOnly(commentLine string) error {
	for _, codeStr := range strings.Split(commentLine, ",") {
		var description string
		if strings.EqualFold(codeStr, defaultTag) {
			codeStr = ""
		} else {
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return fmt.Errorf("can not parse response comment \"%s\"", commentLine)
			}
			description = http.StatusText(code)
		}

		o.AddResponse(codeStr, newResponseWithDescription(description))
	}

	return nil
}

func newResponseWithDescription(description string) *v3.Response {
	return &v3.Response{Description: description}
}

func parseCombinedObjectSchema(parser *Parser, refType string, astFile *ast.File) (*base.SchemaProxy, error) {
	matches := combinedPattern.FindStringSubmatch(refType)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid type: %s", refType)
	}

	schema, err := parseObjectSchema(parser, matches[1], astFile)
	if err != nil {
		return nil, err
	}

	type propEntry struct {
		name   string
		schema *base.SchemaProxy
	}

	fields := parseFields(matches[2])
	var props []propEntry

	for _, field := range fields {
		keyVal := strings.SplitN(field, "=", 2)
		if len(keyVal) != 2 {
			continue
		}

		propSchema, err := parseObjectSchema(parser, keyVal[1], astFile)
		if err != nil {
			return nil, err
		}

		props = append(props, propEntry{name: keyVal[0], schema: propSchema})
	}

	if len(props) == 0 {
		return schema, nil
	}

	if !schema.IsReference() {
		if s := schema.Schema(); s != nil &&
			len(s.Type) > 0 &&
			s.Type[0] == OBJECT &&
			(s.Properties == nil || s.Properties.Len() == 0) &&
			s.AdditionalProperties == nil {
			properties := orderedmap.New[string, *base.SchemaProxy]()
			for _, p := range props {
				properties.Set(p.name, p.schema)
			}
			s.Properties = properties
			return schema, nil
		}
	}

	schemaRefPath := strings.Replace(schema.GetReference(), "#/components/schemas/", "", 1)
	schemaSpec := parser.openAPI.Components.Schemas.GetOrZero(schemaRefPath)

	allOf := make([]*base.SchemaProxy, len(props))
	for i, p := range props {
		wrapperProps := orderedmap.New[string, *base.SchemaProxy]()
		wrapperProps.Set(p.name, p.schema)

		wrapper := base.CreateSchemaProxy(&base.Schema{
			Type:       []string{OBJECT},
			Properties: wrapperProps,
		})

		parser.openAPI.Components.Schemas.Set(p.name, wrapper)

		allOf[i] = base.CreateSchemaProxyRef("#/components/schemas/" + p.name)
	}

	schemaSpec.Schema().AllOf = allOf

	return schemaSpec, nil
}

// ParseSecurityComment parses comment for given `security` comment string.
func (o *Operation) ParseSecurityComment(commentLine string) error {
	securitySource := commentLine[strings.Index(commentLine, "@Security")+1:]

	requirements := orderedmap.New[string, []string]()

	for _, securityOption := range strings.Split(securitySource, "||") {
		securityOption = strings.TrimSpace(securityOption)

		left, right := strings.Index(securityOption, "["), strings.Index(securityOption, "]")

		if !(left == -1 && right == -1) {
			scopes := securityOption[left+1 : right]

			var options []string

			for _, scope := range strings.Split(scopes, ",") {
				options = append(options, strings.TrimSpace(scope))
			}

			securityKey := securityOption[0:left]
			requirements.Set(securityKey, append(requirements.GetOrZero(securityKey), options...))
		} else {
			securityKey := strings.TrimSpace(securityOption)
			requirements.Set(securityKey, []string{})
		}
	}

	o.Security = append(o.Security, &base.SecurityRequirement{Requirements: requirements})

	return nil
}

// ParseCodeSample godoc.
func (o *Operation) ParseCodeSample(attribute, _, lineRemainder string) error {
	log.Println("line remainder:", lineRemainder)

	if lineRemainder == "file" {
		log.Println("line remainder is file")

		data, isJSON, err := getCodeExampleForSummary(o.Summary, o.codeExampleFilesDir)
		if err != nil {
			return err
		}

		// using custom type, as json marshaller has problems with []map[interface{}]map[interface{}]interface{}
		var valueJSON CodeSamples

		if isJSON {
			err = json.Unmarshal(data, &valueJSON)
			if err != nil {
				return fmt.Errorf("annotation %s need a valid json value. error: %s", attribute, err.Error())
			}
		} else {
			err = yaml.Unmarshal(data, &valueJSON)
			if err != nil {
				return fmt.Errorf("annotation %s need a valid yaml value. error: %s", attribute, err.Error())
			}
		}

		if o.Responses.Extensions == nil {
			o.Responses.Extensions = orderedmap.New[string, *yaml.Node]()
		}
		o.Responses.Extensions.Set(attribute[1:], nodeFromValue(valueJSON))

		return nil
	}

	// Fallback into existing logic
	return o.ParseMetadata(attribute, strings.ToLower(attribute), lineRemainder)
}

// flattenQueryPropSchema resolves a struct property into the scalar schema a
// query/header/path parameter needs. A typed-enum field resolves to a $ref (or
// an allOf wrap that also carries the field's default/example/description), and
// a parameter can't reference a component the way a body property can — so this
// inlines the referenced scalar and merges the field's own sibling attributes,
// keeping the enum values, the default, and the example on the parameter.
// Returns a copy, never the shared component schema, so callers may set an
// inferred example without mutating the parsed definition.
func flattenQueryPropSchema(p *Parser, item *base.SchemaProxy) *base.Schema {
	if item == nil {
		return nil
	}

	if item.IsReference() {
		resolved := *p.getSchemaByRef(item.GetReference())
		return &resolved
	}

	s := item.Schema()
	if s == nil {
		return nil
	}

	// ComplementSchema wraps a $ref field carrying its own tags as
	// {default/example/…, allOf:[{$ref}]}. Flatten to the referenced scalar and
	// keep the sibling attributes the wrap added.
	if len(s.Type) == 0 && len(s.AllOf) == 1 {
		var baseSchema *base.Schema
		if inner := s.AllOf[0]; !inner.IsReference() {
			baseSchema = inner.Schema()
		} else {
			baseSchema = p.getSchemaByRef(inner.GetReference())
		}
		if baseSchema != nil {
			merged := *baseSchema
			if s.Default != nil {
				merged.Default = s.Default
			}
			if s.Example != nil {
				merged.Example = s.Example
			}
			if s.Description != "" {
				merged.Description = s.Description
			}
			return &merged
		}
	}

	cp := *s
	return &cp
}

// applyParamSerialization moves the transient style/explode markers that a
// .swaggo override stamped on a query-array schema onto the parameter, and
// strips them from the schema so they never render (style/explode are
// Parameter fields, not schema keywords). A bare explode override defaults
// style to "form" — the query default that makes a comma-delimited array
// well-defined. No-op for non-query params or schemas without the markers.
func applyParamSerialization(param *v3.Parameter, s *base.Schema) {
	if param.In != "query" || s == nil || s.Extensions == nil {
		return
	}
	style := ""
	if n, ok := s.Extensions.Get(paramStyleMarker); ok && n != nil {
		style = n.Value
		s.Extensions.Delete(paramStyleMarker)
	}
	if n, ok := s.Extensions.Get(paramExplodeMarker); ok && n != nil {
		explode := n.Value == "true"
		param.Explode = &explode
		s.Extensions.Delete(paramExplodeMarker)
		if style == "" {
			style = "form"
		}
	}
	if style != "" {
		param.Style = style
	}
}

// explicitlyRequiredQueryFields returns the query-property names of struct
// typeName whose field carries an explicit binding/validate "required" marker.
// Embedded and untagged fields are skipped — they carry no explicit query
// requirement, so their params correctly default to optional.
func explicitlyRequiredQueryFields(parser *Parser, typeName string, file *ast.File) map[string]bool {
	out := map[string]bool{}
	def := parser.packages.FindTypeSpec(typeName, file)
	if def == nil || def.TypeSpec == nil {
		return out
	}
	st, ok := def.TypeSpec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return out
	}
	for _, field := range st.Fields.List {
		if field.Tag == nil || len(field.Names) == 0 {
			continue
		}
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		if !tagHasRequired(tag.Get("binding")) && !tagHasRequired(tag.Get("validate")) {
			continue
		}
		name := strings.TrimSpace(strings.Split(tag.Get("json"), ",")[0])
		if name == "" || name == "-" {
			name = strings.TrimSpace(strings.Split(tag.Get("form"), ",")[0])
		}
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

func tagHasRequired(tagValue string) bool {
	for _, v := range strings.Split(tagValue, ",") {
		if strings.TrimSpace(v) == "required" {
			return true
		}
	}
	return false
}
