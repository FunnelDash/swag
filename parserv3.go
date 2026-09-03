package swag

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"
)

// toYAMLNode encodes an arbitrary value into a *yaml.Node for use as an
// extension value (Extensions maps are keyed to *yaml.Node in libopenapi).
func toYAMLNode(v any) *yaml.Node {
	n := &yaml.Node{}
	_ = n.Encode(v)
	return n
}

// toSecurityRequirement converts a parsed {scheme: scopes} map into a
// libopenapi SecurityRequirement.
func toSecurityRequirement(m map[string][]string) *base.SecurityRequirement {
	req := orderedmap.New[string, []string]()
	for k, v := range m {
		req.Set(k, v)
	}
	return &base.SecurityRequirement{Requirements: req}
}

// FieldParserFactory create FieldParser.
type FieldParserFactory func(ps *Parser, file *ast.File, field *ast.Field) FieldParser

// FieldParser parse struct field.
type FieldParser interface {
	ShouldSkip() bool
	FieldNames() ([]string, error)
	FormName() string
	CustomSchema() (*base.SchemaProxy, error)
	ComplementSchema(schema *base.SchemaProxy) (*base.SchemaProxy, error)
	IsRequired() (bool, error)
}

// GetOpenAPI returns *v3.Document which is the root document object for the API specification.
func (p *Parser) GetOpenAPI() *v3.Document {
	return p.openAPI
}

var (
	serversURLPattern       = regexp.MustCompile(`\{([^}]+)\}`)
	serversVariablesPattern = regexp.MustCompile(`^(\w+)\s+(.+)$`)
)

func (p *Parser) parseGeneralAPIInfo(comments []string) error {
	previousAttribute := ""
	var tag *base.Tag

	// parsing classic meta data model
	for line := 0; line < len(comments); line++ {
		commentLine := comments[line]
		commentLine = strings.TrimSpace(commentLine)
		if len(commentLine) == 0 {
			continue
		}
		fields := FieldsByAnySpace(commentLine, 2)

		attribute := fields[0]
		var value string
		if len(fields) > 1 {
			value = fields[1]
		}

		switch attr := strings.ToLower(attribute); attr {
		case versionAttr, titleAttr, tosAttr, licNameAttr, licURLAttr, conNameAttr, conURLAttr, conEmailAttr:
			setspecInfo(p.openAPI, attr, value)
		case descriptionAttr:
			if previousAttribute == attribute {
				p.openAPI.Info.Description += "\n" + value

				continue
			}

			setspecInfo(p.openAPI, attr, value)
		case descriptionMarkdownAttr:
			commentInfo, err := getMarkdownForTag("api", p.markdownFileDir)
			if err != nil {
				return err
			}

			setspecInfo(p.openAPI, descriptionAttr, string(commentInfo))
		case "@host":
			if len(p.openAPI.Servers) == 0 {
				server := &v3.Server{URL: value}
				p.openAPI.Servers = append(p.openAPI.Servers, server)
			}

			println("@host is deprecated use servers instead")
		case "@basepath":
			if len(p.openAPI.Servers) == 0 {
				p.openAPI.Servers = append(p.openAPI.Servers, &v3.Server{})
			}
			p.openAPI.Servers[0].URL += value

			println("@basepath is deprecated use servers instead")

		case acceptAttr:
			println("acceptAttribute is deprecated, as there is no such field on top level in spec V3.1")
		case produceAttr:
			println("produce is deprecated, as there is no such field on top level in spec V3.1")
		case "@schemes":
			println("@schemes is deprecated use servers instead")
		case "@tag.name":
			if p.matchTag(value) {
				tag = &base.Tag{
					Name: value,
				}

				p.openAPI.Tags = append(p.openAPI.Tags, tag)
			} else {
				tag = nil
			}
		case "@tag.description":
			if tag != nil {
				tag.Description = value
			}
		case "@tag.description.markdown":
			if tag != nil {
				commentInfo, err := getMarkdownForTag(tag.Name, p.markdownFileDir)
				if err != nil {
					return err
				}

				tag.Description = string(commentInfo)
			}
		case "@tag.docs.url":
			if tag != nil {
				tag.ExternalDocs = &base.ExternalDoc{URL: value}
			}
		case "@tag.docs.description":
			if tag != nil {
				if tag.ExternalDocs == nil {
					return fmt.Errorf("%s needs to come after a @tags.docs.url", attribute)
				}

				tag.ExternalDocs.Description = value
			}
		case secBasicAttr, secAPIKeyAttr, secApplicationAttr, secImplicitAttr, secPasswordAttr, secAccessCodeAttr, secBearerAuthAttr:
			key, scheme, err := parseSecAttributes(attribute, comments, &line)
			if err != nil {
				return err
			}

			if p.openAPI.Components.SecuritySchemes == nil {
				p.openAPI.Components.SecuritySchemes = orderedmap.New[string, *v3.SecurityScheme]()
			}

			p.openAPI.Components.SecuritySchemes.Set(key, scheme)

		case securityAttr:
			p.openAPI.Security = append(p.openAPI.Security, toSecurityRequirement(parseSecurity(value)))

		case "@query.collection.format":
			p.collectionFormatInQuery = TransToValidCollectionFormat(value)

		case extDocsDescAttr, extDocsURLAttr:
			if p.openAPI.ExternalDocs == nil {
				p.openAPI.ExternalDocs = &base.ExternalDoc{}
			}

			switch attr {
			case extDocsDescAttr:
				p.openAPI.ExternalDocs.Description = value
			case extDocsURLAttr:
				p.openAPI.ExternalDocs.URL = value
			}

		case "@x-taggroups":
			originalAttribute := strings.Split(commentLine, " ")[0]
			if len(value) == 0 {
				return fmt.Errorf("annotation %s need a value", attribute)
			}

			var valueJSON interface{}
			if err := json.Unmarshal([]byte(value), &valueJSON); err != nil {
				return fmt.Errorf("annotation %s need a valid json value. error: %s", originalAttribute, err.Error())
			}

			if p.openAPI.Info.Extensions == nil {
				p.openAPI.Info.Extensions = orderedmap.New[string, *yaml.Node]()
			}
			p.openAPI.Info.Extensions.Set(originalAttribute[1:], toYAMLNode(valueJSON))
		case "@servers.url":
			server := &v3.Server{URL: value}
			matches := serversURLPattern.FindAllStringSubmatch(value, -1)
			server.Variables = orderedmap.New[string, *v3.ServerVariable]()
			for _, match := range matches {
				server.Variables.Set(match[1], &v3.ServerVariable{})
			}

			p.openAPI.Servers = append(p.openAPI.Servers, server)
		case "@servers.description":
			server := p.openAPI.Servers[len(p.openAPI.Servers)-1]
			server.Description = value
		case "@servers.variables.enum":
			server := p.openAPI.Servers[len(p.openAPI.Servers)-1]
			matches := serversVariablesPattern.FindStringSubmatch(value)
			if len(matches) > 0 {
				variable, ok := server.Variables.Get(matches[1])
				if !ok {
					p.debug.Printf("Variables are not detected.")
					continue
				}
				variable.Enum = append(variable.Enum, matches[2])
			}
		case "@servers.variables.default":
			server := p.openAPI.Servers[len(p.openAPI.Servers)-1]
			matches := serversVariablesPattern.FindStringSubmatch(value)
			if len(matches) > 0 {
				variable, ok := server.Variables.Get(matches[1])
				if !ok {
					p.debug.Printf("Variables are not detected.")
					continue
				}
				variable.Default = matches[2]
			}
		case "@servers.variables.description":
			server := p.openAPI.Servers[len(p.openAPI.Servers)-1]
			matches := serversVariablesPattern.FindStringSubmatch(value)
			if len(matches) > 0 {
				variable, ok := server.Variables.Get(matches[1])
				if !ok {
					p.debug.Printf("Variables are not detected.")
					continue
				}
				variable.Default = matches[2]
			}
		case "@servers.variables.description.markdown":
			server := p.openAPI.Servers[len(p.openAPI.Servers)-1]
			matches := serversVariablesPattern.FindStringSubmatch(value)
			if len(matches) > 0 {
				variable, ok := server.Variables.Get(matches[1])
				if !ok {
					p.debug.Printf("Variables are not detected.")
					continue
				}
				commentInfo, err := getMarkdownForTag(matches[1], p.markdownFileDir)
				if err != nil {
					return err
				}
				variable.Description = string(commentInfo)
			}
		default:
			if strings.HasPrefix(attribute, "@x-") {
				err := p.parseExtensions(value, attribute)
				if err != nil {
					return fmt.Errorf("could not parse extension comment: %w", err)
				}
			} else if strings.HasPrefix(attribute, "@tag.x-") && tag != nil {
				if len(value) == 0 {
					return fmt.Errorf("annotation %s need a value", attribute)
				}

				if tag.Extensions == nil {
					tag.Extensions = orderedmap.New[string, *yaml.Node]()
				}

				// The name keeps the case it was written in and the value stays a
				// raw string: ReDoc's x-displayName and Mintlify's x-group are both
				// case-sensitive, and neither takes JSON.
				tag.Extensions.Set(attribute[len("@tag."):], toYAMLNode(value))
			}
		}

		previousAttribute = attribute
	}

	return nil
}

func (p *Parser) parseExtensions(value, attribute string) error {
	extensionName := attribute[1:]

	if len(value) == 0 {
		return fmt.Errorf("annotation %s need a value", attribute)
	}

	if p.openAPI.Info.Extensions == nil {
		p.openAPI.Info.Extensions = orderedmap.New[string, *yaml.Node]()
	}

	var valueJSON interface{}
	err := json.Unmarshal([]byte(value), &valueJSON)
	if err != nil {
		return fmt.Errorf("annotation %s need a valid json value. error: %s", attribute, err.Error())
	}

	if strings.Contains(extensionName, "logo") {
		p.openAPI.Info.Extensions.Set(extensionName, toYAMLNode(valueJSON))
		return nil
	}

	p.openAPI.Info.Extensions.Set(attribute[1:], toYAMLNode(valueJSON))

	return nil
}

func setspecInfo(openAPI *v3.Document, attribute, value string) {
	switch attribute {
	case versionAttr:
		openAPI.Info.Version = value
	case titleAttr:
		openAPI.Info.Title = value
	case tosAttr:
		openAPI.Info.TermsOfService = value
	case descriptionAttr:
		openAPI.Info.Description = value
	case conNameAttr:
		if openAPI.Info.Contact == nil {
			openAPI.Info.Contact = &base.Contact{}
		}

		openAPI.Info.Contact.Name = value
	case conEmailAttr:
		if openAPI.Info.Contact == nil {
			openAPI.Info.Contact = &base.Contact{}
		}

		openAPI.Info.Contact.Email = value
	case conURLAttr:
		if openAPI.Info.Contact == nil {
			openAPI.Info.Contact = &base.Contact{}
		}

		openAPI.Info.Contact.URL = value
	case licNameAttr:
		if openAPI.Info.License == nil {
			openAPI.Info.License = &base.License{}
		}
		openAPI.Info.License.Name = value
	case licURLAttr:
		if openAPI.Info.License == nil {
			openAPI.Info.License = &base.License{}
		}
		openAPI.Info.License.URL = value
	}
}

func parseSecAttributes(context string, lines []string, index *int) (string, *v3.SecurityScheme, error) {
	const (
		in               = "@in"
		name             = "@name"
		descriptionAttr  = "@description"
		tokenURL         = "@tokenurl"
		authorizationURL = "@authorizationurl"
	)

	var search []string

	attribute := strings.ToLower(FieldsByAnySpace(lines[*index], 2)[0])
	key := getSecurityDefinitionKey(lines[*index])
	switch attribute {
	case secBasicAttr:
		scheme := v3.SecurityScheme{
			Type:   "http",
			Scheme: "basic",
		}
		return key, &scheme, nil
	case secAPIKeyAttr:
		search = []string{in, name}
	case secApplicationAttr, secPasswordAttr:
		search = []string{tokenURL, in, name}
	case secImplicitAttr:
		search = []string{authorizationURL, in}
	case secAccessCodeAttr:
		search = []string{tokenURL, authorizationURL, in}
	case secBearerAuthAttr:
		// Support Bearer scheme with parameters
		scheme := v3.SecurityScheme{
			Type:   "http",
			Scheme: "bearer",
		}
		// Parse parameters
		*index++
		description := ""
		for ; *index < len(lines); *index++ {
			v := strings.TrimSpace(lines[*index])
			if len(v) == 0 {
				continue
			}
			fields := FieldsByAnySpace(v, 2)
			securityAttr := strings.ToLower(fields[0])
			var value string
			if len(fields) > 1 {
				value = fields[1]
			}
			if securityAttr == "@description" {
				description = value
			}
			if securityAttr == "@bearerformat" {
				scheme.BearerFormat = value
			}
			if strings.HasPrefix(securityAttr, "@securitydefinitions.") {
				*index--
				break
			}
		}
		scheme.Description = description
		return key, &scheme, nil
	}

	// For the first line we get the attributes in the context parameter, so we skip to the next one
	*index++

	attrMap, scopes := make(map[string]string), make(map[string]string)
	extensions, description := make(map[string]interface{}), ""

	for ; *index < len(lines); *index++ {
		v := strings.TrimSpace(lines[*index])
		if len(v) == 0 {
			continue
		}

		fields := FieldsByAnySpace(v, 2)
		securityAttr := strings.ToLower(fields[0])
		var value string
		if len(fields) > 1 {
			value = fields[1]
		}

		for _, findTerm := range search {
			if securityAttr == findTerm {
				attrMap[securityAttr] = value

				break
			}
		}

		isExists, err := isExistsScope(securityAttr)
		if err != nil {
			return "", nil, err
		}

		if isExists {
			scopes[securityAttr[len(scopeAttrPrefix):]] = v[len(securityAttr):]
		}

		if strings.HasPrefix(securityAttr, "@x-") {
			// Add the custom attribute without the @
			extensions[securityAttr[1:]] = value
		}

		// Not mandatory field
		if securityAttr == descriptionAttr {
			description = value
		}

		// next securityDefinitions
		if strings.Index(securityAttr, "@securitydefinitions.") == 0 {
			// Go back to the previous line and break
			*index--

			break
		}
	}

	if len(attrMap) != len(search) {
		return "", nil, fmt.Errorf("%s is %v required", context, search)
	}

	scheme := &v3.SecurityScheme{}

	switch attribute {
	case secAPIKeyAttr:
		scheme.Type = "apiKey"
		scheme.In = attrMap[in]
		scheme.Name = attrMap[name]
	case secApplicationAttr:
		scheme.Type = "oauth2"
		scheme.In = attrMap[in]
		scheme.Flows = &v3.OAuthFlows{}
		scheme.Flows.ClientCredentials = &v3.OAuthFlow{}
		scheme.Flows.ClientCredentials.TokenUrl = attrMap[tokenURL]

		scheme.Flows.ClientCredentials.Scopes = orderedmap.New[string, string]()
		for k, v := range scopes {
			scheme.Flows.ClientCredentials.Scopes.Set(k, v)
		}
	case secImplicitAttr:
		scheme.Type = "oauth2"
		scheme.In = attrMap[in]
		scheme.Flows = &v3.OAuthFlows{}
		scheme.Flows.Implicit = &v3.OAuthFlow{}
		scheme.Flows.Implicit.AuthorizationUrl = attrMap[authorizationURL]
		scheme.Flows.Implicit.Scopes = orderedmap.New[string, string]()
		for k, v := range scopes {
			scheme.Flows.Implicit.Scopes.Set(k, v)
		}
	case secPasswordAttr:
		scheme.Type = "oauth2"
		scheme.In = attrMap[in]
		scheme.Flows = &v3.OAuthFlows{}
		scheme.Flows.Password = &v3.OAuthFlow{}
		scheme.Flows.Password.TokenUrl = attrMap[tokenURL]

		scheme.Flows.Password.Scopes = orderedmap.New[string, string]()
		for k, v := range scopes {
			scheme.Flows.Password.Scopes.Set(k, v)
		}

	case secAccessCodeAttr:
		scheme.Type = "oauth2"
		scheme.In = attrMap[in]
		scheme.Flows = &v3.OAuthFlows{}
		scheme.Flows.AuthorizationCode = &v3.OAuthFlow{}
		scheme.Flows.AuthorizationCode.AuthorizationUrl = attrMap[authorizationURL]
		scheme.Flows.AuthorizationCode.TokenUrl = attrMap[tokenURL]
	}

	scheme.Description = description

	if scheme.Flows != nil && scheme.Flows.Extensions == nil && len(extensions) > 0 {
		scheme.Flows.Extensions = orderedmap.New[string, *yaml.Node]()
	}

	for k, v := range extensions {
		scheme.Flows.Extensions.Set(k, toYAMLNode(v))
	}

	return key, scheme, nil
}

func getSecurityDefinitionKey(line string) string {
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(line)), "@securitydefinitions") {
		splittedLine := strings.Fields(line)
		return splittedLine[len(splittedLine)-1]
	}

	return ""
}

// ParseRouterAPIInfo parses router api info for given astFile.
func (p *Parser) ParseRouterAPIInfo(fileInfo *AstFileInfo) error {
	for _, astDescription := range fileInfo.File.Decls {
		if (fileInfo.ParseFlag & ParseOperations) == ParseNone {
			continue
		}

		astDeclaration, ok := astDescription.(*ast.FuncDecl)
		if !ok || astDeclaration.Doc == nil || astDeclaration.Doc.List == nil {
			continue
		}

		if p.matchTags(astDeclaration.Doc.List) &&
			matchExtension(p.parseExtension, astDeclaration.Doc.List) {
			// for per 'function' comment, create a new 'Operation' object
			operation := NewOperation(p, SetCodeExampleFilesDirectory(p.codeExampleFilesDir))

			for _, comment := range astDeclaration.Doc.List {
				err := operation.ParseComment(comment.Text, fileInfo.File)
				if err != nil {
					return fmt.Errorf("ParseComment error in file %s :%+v", fileInfo.Path, err)
				}
			}

			// Default the operationId to the handler's function name when no @ID
			// annotation set one, so every operation carries a stable id for
			// client codegen without a hand-written tag per handler. This is not
			// deduped here: checkOperationIDUniqueness only walks the v2
			// parser.swagger, so it's a no-op on the v3 path. Two handlers sharing
			// a func name yield the same id; the consumer merges/suffixes them.
			if operation.OperationId == "" {
				operation.OperationId = astDeclaration.Name.Name
			}

			// workaround until we replace the produce comment with a new @Success syntax
			// We first need to setup all responses before we can set the mimetypes
			err := operation.ProcessProduceComment()
			if err != nil {
				return err
			}

			err = processRouterOperation(p, operation)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func processRouterOperation(p *Parser, o *Operation) error {
	for _, routeProperties := range o.RouterProperties {
		var (
			pathItem *v3.PathItem
			ok       bool
		)

		pathItem, ok = p.openAPI.Paths.PathItems.Get(routeProperties.Path)
		if !ok {
			pathItem = &v3.PathItem{}
		}

		op := refRouteMethodOp(pathItem, routeProperties.HTTPMethod)

		// check if we already have an operation for this path and method
		if *op != nil {
			err := fmt.Errorf("route %s %s is declared multiple times", routeProperties.HTTPMethod, routeProperties.Path)
			if p.Strict {
				return err
			}

			p.debug.Printf("warning: %s\n", err)
		}

		*op = &o.Operation

		p.openAPI.Paths.PathItems.Set(routeProperties.Path, pathItem)
	}

	return nil
}

// orderPathsByTags reorders the document's paths to follow the order the tags
// were declared in. A path whose first tag was never declared keeps the order
// it was parsed in, after every path that carries a declared tag.
func (p *Parser) orderPathsByTags() {
	if len(p.openAPI.Tags) == 0 || p.openAPI.Paths == nil {
		return
	}

	rank := make(map[string]int, len(p.openAPI.Tags))
	for i, tag := range p.openAPI.Tags {
		rank[tag.Name] = i
	}

	type rankedPath struct {
		path string
		item *v3.PathItem
		rank int
	}

	paths := make([]rankedPath, 0, orderedmap.Len(p.openAPI.Paths.PathItems))
	for path, item := range p.openAPI.Paths.PathItems.FromOldest() {
		r, ok := rank[firstOperationTag(item)]
		if !ok {
			r = len(p.openAPI.Tags)
		}

		paths = append(paths, rankedPath{path: path, item: item, rank: r})
	}

	sort.SliceStable(paths, func(i, j int) bool { return paths[i].rank < paths[j].rank })

	ordered := orderedmap.New[string, *v3.PathItem]()
	for _, p := range paths {
		ordered.Set(p.path, p.item)
	}

	p.openAPI.Paths.PathItems = ordered
}

// firstOperationTag returns the first tag of the path's first tagged operation,
// methods taken in the order the OpenAPI path item object declares them.
func firstOperationTag(item *v3.PathItem) string {
	for _, op := range []*v3.Operation{
		item.Get, item.Put, item.Post, item.Delete,
		item.Options, item.Head, item.Patch, item.Trace,
	} {
		if op != nil && len(op.Tags) > 0 {
			return op.Tags[0]
		}
	}

	return ""
}

func refRouteMethodOp(item *v3.PathItem, method string) **v3.Operation {
	switch method {
	case http.MethodGet:
		if item.Get == nil {
			item.Get = &v3.Operation{}
		}
		return &item.Get
	case http.MethodPost:
		if item.Post == nil {
			item.Post = &v3.Operation{}
		}
		return &item.Post
	case http.MethodDelete:
		if item.Delete == nil {
			item.Delete = &v3.Operation{}
		}
		return &item.Delete
	case http.MethodPut:
		if item.Put == nil {
			item.Put = &v3.Operation{}
		}
		return &item.Put
	case http.MethodPatch:
		if item.Patch == nil {
			item.Patch = &v3.Operation{}
		}
		return &item.Patch
	case http.MethodHead:
		if item.Head == nil {
			item.Head = &v3.Operation{}
		}
		return &item.Head
	case http.MethodOptions:
		if item.Options == nil {
			item.Options = &v3.Operation{}
		}
		return &item.Options
	default:
		return nil
	}
}

// splitOverride separates a .swaggo override value into its type spec and the
// optional format=/example= metadata tokens, e.g.
// `string,format=date,example=2025-01-01` -> ("string", "date", "2025-01-01").
// Stripping the metadata first keeps a dotted example value from being mistaken
// for a pkg.Type substitution.
// Transient private extension keys carrying a .swaggo override's parameter
// serialization (style/explode) from type resolution to the query-parameter
// builder. They are stripped before the schema is rendered, so they never
// appear in the output spec — style/explode are Parameter properties, not
// schema keywords.
const (
	paramStyleMarker   = "x-swag-param-style"
	paramExplodeMarker = "x-swag-param-explode"
)

// overrideMeta is the parsed form of a .swaggo `replace <type> <override>`
// value: the core type spec plus optional format/example decoration and, for
// query-array types, a style/explode serialization hint.
type overrideMeta struct {
	core    string
	format  string
	example string
	style   string
	explode *bool
}

func splitOverride(override string) overrideMeta {
	m := overrideMeta{}
	parts := make([]string, 0)
	for _, tok := range strings.Split(override, ",") {
		switch {
		case strings.HasPrefix(tok, "format="):
			m.format = strings.TrimPrefix(tok, "format=")
		case strings.HasPrefix(tok, "example="):
			m.example = strings.TrimPrefix(tok, "example=")
		case strings.HasPrefix(tok, "style="):
			m.style = strings.TrimPrefix(tok, "style=")
		case strings.HasPrefix(tok, "explode="):
			v := strings.TrimPrefix(tok, "explode=") == "true"
			m.explode = &v
		default:
			parts = append(parts, tok)
		}
	}
	m.core = strings.Join(parts, ",")
	return m
}

// applyOverrideMeta stamps format/example from a .swaggo override onto a schema.
// A SchemaProxy is build-once, so this unwraps the inline schema, mutates the
// concrete *base.Schema, and re-wraps it. A $ref proxy (no inline schema) is
// returned unchanged, matching the old behaviour of skipping refs.
func applyOverrideMeta(schema *base.SchemaProxy, format, example string) *base.SchemaProxy {
	if schema == nil || (format == "" && example == "") {
		return schema
	}
	s := schema.Schema()
	if s == nil {
		return schema
	}
	if format != "" {
		s.Format = format
	}
	if example != "" {
		s.Example = toYAMLNode(example)
	}
	return base.CreateSchemaProxy(s)
}

// copyPropSchema returns a shallow copy of an embedded struct property's
// schema safe to decorate without mutating a shared/cached source, and strips
// any inherited x-order: a promoted field must be re-ordered within the
// enclosing struct's sequence, not keep the order it had in the embedded type's
// own schema (which would collide with the enclosing struct's own fields). The
// schema struct and Extensions map are copied; deeper nodes (Properties, Items,
// Enum) are shared, which is fine for the top-level decoration callers do.
func copyPropSchema(v *base.SchemaProxy) *base.SchemaProxy {
	if v == nil {
		return nil
	}
	if v.IsReference() {
		return base.CreateSchemaProxyRef(v.GetReference()) // a $ref; not mutated by callers
	}
	s := v.Schema()
	if s == nil {
		return v
	}
	cp := *s
	if s.Extensions != nil {
		ext := orderedmap.New[string, *yaml.Node]()
		for pair := s.Extensions.First(); pair != nil; pair = pair.Next() {
			if pair.Key() == "x-order" {
				continue // re-assigned by the enclosing struct
			}
			ext.Set(pair.Key(), pair.Value())
		}
		cp.Extensions = ext
	}
	return base.CreateSchemaProxy(&cp)
}

// resolveOverride is the single entry point for .swaggo type overrides. It
// looks a type up by, in precedence order, its full path then its generic-base
// path, and returns one of:
//
//   - (schema, _, false, nil): the override produced a schema outright — a
//     swaggertype spec, or a generic array whose items come from the element T.
//   - (nil, sub, true, nil): the override substitutes another type (`pkg.Type`
//     form); resolve `sub` (possibly nil if not found) instead.
//   - (nil, nil, false, nil): no override for this type; resolve it normally.
//   - (nil, _, _, ErrSkippedField): an empty override means "ignore this type".
//
// A `pkg.Type` substitution is followed here (cycle-guarded) so an override that
// points at another overridden type resolves in one place rather than leaking
// the re-check back into getTypeSchema.
func (p *Parser) resolveOverride(typeSpecDef *TypeSpecDef, file *ast.File) (*base.SchemaProxy, *TypeSpecDef, bool, error) {
	seen := map[string]bool{}
	didSubstitute := false

	for typeSpecDef != nil {
		overrideKey := typeSpecDef.FullPath()
		override, ok := p.Overrides[overrideKey]
		viaGeneric := false
		if !ok {
			if base := typeSpecDef.GenericBaseFullPath(); base != "" {
				override, ok = p.Overrides[base]
				overrideKey = base
				viaGeneric = ok
			}
		}
		if !ok {
			return nil, typeSpecDef, didSubstitute, nil
		}
		if override == "" {
			p.debug.Printf("Override detected for %s: ignoring", overrideKey)
			return nil, nil, didSubstitute, ErrSkippedField
		}
		p.debug.Printf("Override detected for %s: using %s instead", overrideKey, override)

		// Strip format=/example= before the substitution check so a dotted
		// example value isn't mistaken for a pkg.Type.
		m := splitOverride(override)

		// A generic array wrapper (e.g. CommaArray[T]) applied via an `array`
		// base-type override renders its items from the real type argument T,
		// resolved through getTypeSchema so it matches how T renders anywhere
		// else: a $ref for a named element (an enum keeps its values; a
		// .swaggo-overridden type like types.UUID keeps its string/format), a
		// primitive schema for a Go-primitive element. The element is always T;
		// there's no hardcoded fallback, so the override is just `array` and
		// CommaArray[int] is an int array, not a string one.
		if viaGeneric && (m.core == ARRAY || strings.HasPrefix(m.core, ARRAY+",")) &&
			len(typeSpecDef.TypeArgNames) == 1 {
			items, err := p.getTypeSchema(typeSpecDef.TypeArgNames[0], file, true)
			if err != nil {
				return nil, nil, didSubstitute, fmt.Errorf("resolve generic array element %s: %w", typeSpecDef.TypeArgNames[0], err)
			}
			arr := &base.Schema{
				Type:  []string{ARRAY},
				Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: items},
			}
			// A style=/explode= hint on the override rides along as a transient
			// marker for the query-parameter builder (stripped before render).
			if m.style != "" || m.explode != nil {
				arr.Extensions = orderedmap.New[string, *yaml.Node]()
				if m.style != "" {
					arr.Extensions.Set(paramStyleMarker, toYAMLNode(m.style))
				}
				if m.explode != nil {
					arr.Extensions.Set(paramExplodeMarker, toYAMLNode(strconv.FormatBool(*m.explode)))
				}
			}
			result := applyOverrideMeta(base.CreateSchemaProxy(arr), m.format, m.example)
			return result, nil, didSubstitute, nil
		}

		if !strings.Contains(m.core, ".") {
			// swaggertype spec (+ optional format/example)
			schema, err := BuildCustomSchema(strings.Split(m.core, ","))
			if err != nil {
				return nil, nil, didSubstitute, err
			}
			schema = applyOverrideMeta(schema, m.format, m.example)
			return schema, nil, didSubstitute, nil
		}

		// pkg.Type substitution: resolve the named type, then re-check whether
		// it too carries an override (bounded by the cycle guard).
		if seen[overrideKey] {
			return nil, nil, didSubstitute, fmt.Errorf("override substitution cycle at %s", overrideKey)
		}
		seen[overrideKey] = true
		separator := strings.LastIndex(m.core, ".")
		typeSpecDef = p.packages.findTypeSpec(m.core[0:separator], m.core[separator+1:])
		didSubstitute = true
	}

	return nil, nil, didSubstitute, nil
}

func (p *Parser) getTypeSchema(typeName string, file *ast.File, ref bool) (*base.SchemaProxy, error) {
	if override, ok := p.Overrides[typeName]; ok {
		p.debug.Printf("Override detected for %s: using %s instead", typeName, override)
		m := splitOverride(override)
		var (
			schema *base.SchemaProxy
			err    error
		)
		if strings.Contains(m.core, ".") {
			schema, err = parseObjectSchema(p, m.core, file) // pkg.Type substitution
		} else {
			schema, err = BuildCustomSchema(strings.Split(m.core, ","))
		}
		if err != nil {
			return nil, err
		}
		schema = applyOverrideMeta(schema, m.format, m.example)
		return schema, nil
	}

	if IsInterfaceLike(typeName) {
		return base.CreateSchemaProxy(&base.Schema{}), nil
	}

	if IsGolangPrimitiveType(typeName) {
		return PrimitiveSchema(TransToValidSchemeType(typeName)), nil
	}

	typeSpecDef := p.packages.FindTypeSpec(typeName, file)

	// A .swaggo override wins over the built-in specific-type mapping below, so
	// a repo type can be described entirely in .swaggo (e.g.
	// `replace .../types.UUID string,format=uuid`) instead of relying on swag's
	// built-in UUID/Time/... short-circuit, which can't carry a format.
	if typeSpecDef != nil {
		schema, substitute, didSubstitute, err := p.resolveOverride(typeSpecDef, file)
		if err != nil {
			return nil, err
		}
		if schema != nil {
			return schema, nil
		}
		if didSubstitute {
			typeSpecDef = substitute // a pkg.Type override; resolve that type below
		}
	}

	// Built-in specific->primitive mapping, for types without a .swaggo override.
	if schemaType, err := convertFromSpecificToPrimitive(typeName); err == nil {
		return PrimitiveSchema(schemaType), nil
	}

	if typeSpecDef == nil {
		return nil, fmt.Errorf("cannot find type definition: %s", typeName)
	}

	schema, ok := p.parsedSchemas[typeSpecDef]
	if !ok {
		var err error

		schema, err = p.ParseDefinition(typeSpecDef)
		if err != nil {
			if err == ErrRecursiveParseStruct && ref {
				return p.getRefTypeSchema(typeSpecDef, schema), nil
			}
			return nil, err
		}
	}

	if ref {
		if IsComplexSchema(schema) {
			return p.getRefTypeSchema(typeSpecDef, schema), nil
		}

		// if it is a simple schema, just return a copy
		newSchema := *schema.Schema
		return base.CreateSchemaProxy(&newSchema), nil
	}

	return base.CreateSchemaProxy(schema.Schema), nil
}

// ParseDefinition parses given type spec that corresponds to the type under
// given name and package, and populates swagger schema definitions registry
// with a schema for the given type
func (p *Parser) ParseDefinition(typeSpecDef *TypeSpecDef) (*Schema, error) {
	typeName := typeSpecDef.TypeName()
	schema, found := p.parsedSchemas[typeSpecDef]
	if found {
		p.debug.Printf("Skipping '%s', already parsed.", typeName)

		return schema, nil
	}

	if p.isInStructStack(typeSpecDef) {
		p.debug.Printf("Skipping '%s', recursion detected.", typeName)

		schemaName := typeName
		if typeSpecDef.SchemaName != "" {
			schemaName = typeSpecDef.SchemaName
		}

		schema := &Schema{
			Name:    schemaName,
			PkgPath: typeSpecDef.PkgPath,
			Schema:  PrimitiveSchema(OBJECT).Schema(),
		}

		p.parsedSchemas[typeSpecDef] = schema

		if p.openAPI.Components.Schemas == nil {
			p.openAPI.Components.Schemas = orderedmap.New[string, *base.SchemaProxy]()
		}
		p.openAPI.Components.Schemas.Set(schema.Name, base.CreateSchemaProxy(schema.Schema))

		return schema, ErrRecursiveParseStruct
	}

	p.structStack = append(p.structStack, typeSpecDef)

	p.debug.Printf("Generating %s", typeName)

	definition, err := p.parseTypeExpr(typeSpecDef.File, typeSpecDef.TypeSpec.Type, false)
	if err != nil {
		p.debug.Printf("Error parsing type definition '%s': %s", typeName, err)
		return nil, err
	}

	// SchemaProxy is build-once: unwrap the parsed schema, decorate the concrete
	// *base.Schema fully (description, enum, extensions), then re-wrap once.
	def := definition.Schema()
	if def == nil {
		def = &base.Schema{}
	}

	if def.Description == "" {
		fillDefinitionDescription(p, def, typeSpecDef.File, typeSpecDef)
	}

	if len(typeSpecDef.Enums) > 0 {
		// Set the enum from the type's own consts rather than appending to
		// whatever the parsed underlying arrived with: for a type alias
		// (`type T = other.Enum`) the underlying already carries the target's
		// enum, so appending the alias's consts on top doubles every value.
		var varNames []string
		var enumComments = make(map[string]string)
		enum := make([]*yaml.Node, 0, len(typeSpecDef.Enums))
		for _, value := range typeSpecDef.Enums {
			enum = append(enum, toYAMLNode(value.Value))
			varNames = append(varNames, value.key)
			if len(value.Comment) > 0 {
				enumComments[value.key] = value.Comment
			}
		}
		def.Enum = enum

		if def.Extensions == nil {
			def.Extensions = orderedmap.New[string, *yaml.Node]()
		}

		def.Extensions.Set(enumVarNamesExtension, toYAMLNode(varNames))
		if len(enumComments) > 0 {
			def.Extensions.Set(enumCommentsExtension, toYAMLNode(enumComments))
		}
	}
	schemaName := typeName
	if typeSpecDef.SchemaName != "" {
		schemaName = typeSpecDef.SchemaName
	}

	sch := Schema{
		Name:    schemaName,
		PkgPath: typeSpecDef.PkgPath,
		Schema:  def,
	}
	p.parsedSchemas[typeSpecDef] = &sch

	// update an empty schema as a result of recursion
	s2, found := p.outputSchemas[typeSpecDef]
	if found {
		p.openAPI.Components.Schemas.Set(s2.Name, base.CreateSchemaProxy(def))
	}

	return &sch, nil
}

// fillDefinitionDescription additionally fills fields in definition (base.Schema)
// TODO: If .go file contains many types, it may work for a long time
func fillDefinitionDescription(parser *Parser, definition *base.Schema, file *ast.File, typeSpecDef *TypeSpecDef) {
	for _, astDeclaration := range file.Decls {
		generalDeclaration, ok := astDeclaration.(*ast.GenDecl)
		if !ok || generalDeclaration.Tok != token.TYPE {
			continue
		}

		for _, astSpec := range generalDeclaration.Specs {
			typeSpec, ok := astSpec.(*ast.TypeSpec)
			if !ok || typeSpec != typeSpecDef.TypeSpec {
				continue
			}

			var typeName string
			if typeSpec.Name != nil {
				typeName = typeSpec.Name.Name
			}

			text, err := parser.extractDeclarationDescription(typeName, typeSpec.Comment, generalDeclaration.Doc)
			if err != nil {
				parser.debug.Printf("Error extracting declaration description: %s", err)
				continue
			}

			definition.Description = text
		}
	}
}

// parseTypeExpr parses given type expression that corresponds to the type under
// given name and package, and returns swagger schema for it.
func (p *Parser) parseTypeExpr(file *ast.File, typeExpr ast.Expr, ref bool) (*base.SchemaProxy, error) {
	const errMessage = "parse type expression v3"

	switch expr := typeExpr.(type) {
	// type Foo interface{}
	case *ast.InterfaceType:
		return base.CreateSchemaProxy(&base.Schema{}), nil

	// type Foo struct {...}
	case *ast.StructType:
		return p.parseStruct(file, expr.Fields)

	// type Foo Baz
	case *ast.Ident:
		result, err := p.getTypeSchema(expr.Name, file, ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", errMessage, err)
		}

		return result, nil
	// type Foo *Baz
	case *ast.StarExpr:
		return p.parseTypeExpr(file, expr.X, ref)

	// type Foo pkg.Bar
	case *ast.SelectorExpr:
		if xIdent, ok := expr.X.(*ast.Ident); ok {
			result, err := p.getTypeSchema(fullTypeName(xIdent.Name, expr.Sel.Name), file, ref)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", errMessage, err)
			}

			return result, nil
		}
	// type Foo []Baz
	case *ast.ArrayType:
		itemSchema, err := p.parseTypeExpr(file, expr.Elt, true)
		if err != nil {
			return nil, err
		}

		if itemSchema == nil {
			schema := &base.Schema{
				Type:  []string{ARRAY},
				Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{})},
			}
			p.debug.Printf("Creating array with empty item schema %v", expr.Elt)

			return base.CreateSchemaProxy(schema), nil
		}

		result := &base.Schema{
			Type:  []string{ARRAY},
			Items: &base.DynamicValue[*base.SchemaProxy, bool]{A: itemSchema},
		}

		return base.CreateSchemaProxy(result), nil
	// type Foo map[string]Bar
	case *ast.MapType:
		if _, ok := expr.Value.(*ast.InterfaceType); ok {
			result := &base.Schema{
				Type:                 []string{OBJECT},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: base.CreateSchemaProxy(&base.Schema{})},
			}

			return base.CreateSchemaProxy(result), nil
		}

		schema, err := p.parseTypeExpr(file, expr.Value, true)
		if err != nil {
			return nil, err
		}

		result := &base.Schema{
			Type:                 []string{OBJECT},
			AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{A: schema},
		}

		return base.CreateSchemaProxy(result), nil
	case *ast.FuncType:
		return nil, ErrFuncTypeField
		// ...
	}

	return p.parseGenericTypeExpr(file, typeExpr)
}

func (p *Parser) parseStruct(file *ast.File, fields *ast.FieldList) (*base.SchemaProxy, error) {
	required := make([]string, 0)
	properties := orderedmap.New[string, *base.SchemaProxy]()
	order := 0

	for _, field := range fields.List {
		fieldProps, requiredFromAnon, err := p.parseStructField(file, field)
		if err != nil {
			if err == ErrFuncTypeField || err == ErrSkippedField {
				continue
			}

			return nil, err
		}

		if len(fieldProps) == 0 {
			continue
		}

		required = append(required, requiredFromAnon...)

		// Stamp x-order in declaration order so renderers show fields in source
		// order. fieldProps is a map; iterate its keys sorted for a stable
		// assignment (a single named field yields one entry, so order tracks
		// declaration; an embedded field's props are ordered among themselves
		// but keep their block position relative to siblings).
		fieldNames := make([]string, 0, len(fieldProps))
		for k := range fieldProps {
			fieldNames = append(fieldNames, k)
		}
		sort.Strings(fieldNames)

		for _, k := range fieldNames {
			v := fieldProps[k]
			if p.AutoOrderProperties {
				// A bare $ref property can't carry an extension, so wrap it in
				// allOf — the same shape the manual x-order tag produces for a
				// $ref field (OpenAPI 3.1 allows keywords beside a $ref).
				// SchemaProxy is build-once, so assemble the concrete schema
				// with x-order stamped, then wrap once.
				if v.IsReference() {
					order++
					s := &base.Schema{
						AllOf:      []*base.SchemaProxy{base.CreateSchemaProxyRef(v.GetReference())},
						Extensions: orderedmap.New[string, *yaml.Node](),
					}
					s.Extensions.Set("x-order", toYAMLNode(fmt.Sprintf("%04d", order)))
					v = base.CreateSchemaProxy(s)
				} else if s := v.Schema(); s != nil {
					if _, ok := extGet(s.Extensions, "x-order"); !ok {
						order++
						cp := *s
						cp.Extensions = extClone(s.Extensions)
						cp.Extensions.Set("x-order", toYAMLNode(fmt.Sprintf("%04d", order)))
						v = base.CreateSchemaProxy(&cp)
					}
				}
			}
			properties.Set(k, v)
		}
	}

	sort.Strings(required)
	if len(required) == 0 {
		required = nil // omit `required: []` rather than emit an empty array
	}

	result := &base.Schema{
		Type:       []string{OBJECT},
		Properties: properties,
		Required:   required,
	}

	return base.CreateSchemaProxy(result), nil
}

// extGet reads a key from an extensions map, tolerating a nil map.
func extGet(ext *orderedmap.Map[string, *yaml.Node], key string) (*yaml.Node, bool) {
	if ext == nil {
		return nil, false
	}
	return ext.Get(key)
}

// extClone returns a fresh copy of an extensions map (nil-safe), so mutating
// the copy doesn't touch a schema already wrapped in a proxy.
func extClone(ext *orderedmap.Map[string, *yaml.Node]) *orderedmap.Map[string, *yaml.Node] {
	out := orderedmap.New[string, *yaml.Node]()
	if ext == nil {
		return out
	}
	for pair := ext.First(); pair != nil; pair = pair.Next() {
		out.Set(pair.Key(), pair.Value())
	}
	return out
}

func (p *Parser) parseStructField(file *ast.File, field *ast.Field) (map[string]*base.SchemaProxy, []string, error) {
	if field.Tag != nil {
		skip, ok := reflect.StructTag(strings.ReplaceAll(field.Tag.Value, "`", "")).Lookup("swaggerignore")
		if ok && strings.EqualFold(skip, "true") {
			return nil, nil, nil
		}
	}

	ps := p.fieldParserFactory(p, file, field)

	if ps.ShouldSkip() {
		return nil, nil, nil
	}

	fieldNames, err := ps.FieldNames()
	if err != nil {
		return nil, nil, err
	}

	if len(fieldNames) == 0 {
		typeName, err := getFieldType(file, field.Type, nil)
		if err != nil {
			return nil, nil, err
		}

		schema, err := p.getTypeSchema(typeName, file, false)
		if err != nil {
			return nil, nil, err
		}

		if s := schema.Schema(); s != nil && len(s.Type) > 0 && s.Type[0] == OBJECT {
			if orderedmap.Len(s.Properties) == 0 {
				return nil, nil, nil
			}

			// Copy each promoted property before re-parenting it: the embedded
			// type's parsed schema is shared (cached in parsedSchemas), and the
			// enclosing struct stamps x-order onto these entries — which would
			// otherwise mutate the embedded type's own schema and every other
			// struct that embeds it.
			properties := make(map[string]*base.SchemaProxy)
			for pair := s.Properties.First(); pair != nil; pair = pair.Next() {
				properties[pair.Key()] = copyPropSchema(pair.Value())
			}

			return properties, s.Required, nil
		}
		// for alias type of non-struct types ,such as array,map, etc. ignore field tag.
		return map[string]*base.SchemaProxy{
			typeName: schema,
		}, nil, nil

	}

	schema, err := ps.CustomSchema()
	if err != nil {
		return nil, nil, err
	}

	if schema == nil {
		typeName, err := getFieldType(file, field.Type, nil)
		if err == nil {
			// named type
			schema, err = p.getTypeSchema(typeName, file, true)
			if err != nil {
				return nil, nil, err
			}

		} else {
			// unnamed type
			parsedSchema, err := p.parseTypeExpr(file, field.Type, false)
			if err != nil {
				return nil, nil, err
			}

			schema = parsedSchema
		}
	}

	schema, err = ps.ComplementSchema(schema)
	if err != nil {
		return nil, nil, err
	}

	var tagRequired []string

	required, err := ps.IsRequired()
	if err != nil {
		return nil, nil, err
	}

	if required {
		tagRequired = append(tagRequired, fieldNames...)
	}

	fieldProps := make(map[string]*base.SchemaProxy, len(fieldNames))
	for _, name := range fieldNames {
		fieldProps[name] = schema
	}

	return fieldProps, tagRequired, nil
}

func (p *Parser) getRefTypeSchema(typeSpecDef *TypeSpecDef, schema *Schema) *base.SchemaProxy {
	_, ok := p.outputSchemas[typeSpecDef]
	if !ok {
		if p.openAPI.Components.Schemas == nil {
			p.openAPI.Components.Schemas = orderedmap.New[string, *base.SchemaProxy]()
		}

		p.openAPI.Components.Schemas.Set(schema.Name, base.CreateSchemaProxy(&base.Schema{}))

		if schema.Schema != nil {
			p.openAPI.Components.Schemas.Set(schema.Name, base.CreateSchemaProxy(schema.Schema))
		}

		p.outputSchemas[typeSpecDef] = schema
	}

	refSchema := RefSchema(schema.Name)

	return refSchema
}

// GetSchemaTypePath get path of schema type.
func (p *Parser) GetSchemaTypePath(schema *base.SchemaProxy, depth int) []string {
	if schema == nil || depth == 0 {
		return nil
	}

	name := ""
	if schema.IsReference() {
		name = schema.GetReference()
	}

	if name != "" {
		if pos := strings.LastIndexByte(name, '/'); pos >= 0 {
			name = name[pos+1:]
			if s, ok := p.openAPI.Components.Schemas.Get(name); ok {
				return p.GetSchemaTypePath(s, depth)
			}
		}

		return nil
	}

	s := schema.Schema()
	if s != nil && len(s.Type) > 0 {
		switch s.Type[0] {
		case ARRAY:
			if s.Items != nil && s.Items.A != nil {
				depth--

				return append([]string{s.Type[0]}, p.GetSchemaTypePath(s.Items.A, depth)...)
			}
		case OBJECT:
			if s.AdditionalProperties != nil && s.AdditionalProperties.A != nil {
				// for map
				depth--

				return append([]string{s.Type[0]}, p.GetSchemaTypePath(s.AdditionalProperties.A, depth)...)
			}
		}

		return []string{s.Type[0]}
	}

	println("found schema with no Type, returning any")
	return []string{ANY}
}

func (p *Parser) getSchemaByRef(ref string) *base.Schema {
	searchString := strings.ReplaceAll(ref, "#/components/schemas/", "")
	schemaRef, exists := p.openAPI.Components.Schemas.Get(searchString)
	if !exists || schemaRef == nil {
		println(fmt.Sprintf("Schema not found for ref: %s, returning any", ref))
		return &base.Schema{} // return empty schema if not found
	}

	return schemaRef.Schema()
}
