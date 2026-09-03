package swag

import (
	"encoding/json"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v4"
)

// decodeNode decodes a libopenapi extension/enum *yaml.Node back into the plain
// Go value (string, map[string]interface{}, []interface{}, ...) so it can be
// compared against a hand-built expected value.
func decodeNode(t *testing.T, n *yaml.Node) interface{} {
	t.Helper()
	require.NotNil(t, n)
	var v interface{}
	require.NoError(t, n.Decode(&v))
	return v
}

// extVal reads an extension key and decodes its node to a plain Go value.
func extVal(t *testing.T, ext *orderedmap.Map[string, *yaml.Node], key string) interface{} {
	t.Helper()
	require.NotNil(t, ext, "extensions map should not be nil for key %s", key)
	n, ok := ext.Get(key)
	require.True(t, ok, "extension %s should be present", key)
	return decodeNode(t, n)
}

// enumValues decodes each enum node into its plain Go value.
func enumValues(t *testing.T, enum []*yaml.Node) []interface{} {
	t.Helper()
	out := make([]interface{}, 0, len(enum))
	for _, n := range enum {
		out = append(out, decodeNode(t, n))
	}
	return out
}

// renderSchemaProxyJSON renders a single schema proxy to JSON by wrapping it in
// a throwaway document's components and rendering the whole document — a
// standalone *base.Schema/*base.SchemaProxy built by hand (no low-level backing)
// cannot be marshalled directly, but the document node-builder renders nested
// schemas without touching the low model.
func renderSchemaProxyJSON(t *testing.T, proxy *base.SchemaProxy) string {
	t.Helper()
	schemas := orderedmap.New[string, *base.SchemaProxy]()
	schemas.Set("__subject", proxy)
	doc := &v3.Document{
		Version:    "3.1.0",
		Info:       &base.Info{Title: "t", Version: "1"},
		Paths:      &v3.Paths{PathItems: orderedmap.New[string, *v3.PathItem]()},
		Components: &v3.Components{Schemas: schemas},
	}
	jb, err := doc.RenderJSON("")
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(jb, &m))
	sub := m["components"].(map[string]interface{})["schemas"].(map[string]interface{})["__subject"]
	b, err := json.Marshal(sub)
	require.NoError(t, err)
	return string(b)
}

func TestOverridesGetTypeSchema(t *testing.T) {
	t.Parallel()

	overrides := map[string]string{
		"sql.NullString": "string",
	}

	p := New(SetOverrides(overrides))

	t.Run("Override sql.NullString by string", func(t *testing.T) {
		t.Parallel()

		s, err := p.getTypeSchema("sql.NullString", nil, false)
		if assert.NoError(t, err) {
			assert.Truef(t, s.Schema().Type[0] == "string", "type sql.NullString should be overridden by string")
		}
	})

	t.Run("Missing Override for sql.NullInt64", func(t *testing.T) {
		t.Parallel()

		_, err := p.getTypeSchema("sql.NullInt64", nil, false)
		if assert.Error(t, err) {
			assert.Equal(t, "cannot find type definition: sql.NullInt64", err.Error())
		}
	})
}

func TestParserParseDefinition(t *testing.T) {
	p := New()

	// Parsing existing type
	definition := &TypeSpecDef{
		PkgPath: "github.com/swagger/swag",
		File: &ast.File{
			Name: &ast.Ident{
				Name: "swag",
			},
		},
		TypeSpec: &ast.TypeSpec{
			Name: &ast.Ident{
				Name: "Test",
			},
		},
	}

	expected := &Schema{}
	p.parsedSchemas[definition] = expected

	schema, err := p.ParseDefinition(definition)
	assert.NoError(t, err)
	assert.Equal(t, expected, schema)

	// Parsing *ast.FuncType
	definition = &TypeSpecDef{
		PkgPath: "github.com/swagger/swag/model",
		File: &ast.File{
			Name: &ast.Ident{
				Name: "model",
			},
		},
		TypeSpec: &ast.TypeSpec{
			Name: &ast.Ident{
				Name: "Test",
			},
			Type: &ast.FuncType{},
		},
	}
	_, err = p.ParseDefinition(definition)
	assert.Error(t, err)

	// Parsing *ast.FuncType with parent spec
	definition = &TypeSpecDef{
		PkgPath: "github.com/swagger/swag/model",
		File: &ast.File{
			Name: &ast.Ident{
				Name: "model",
			},
		},
		TypeSpec: &ast.TypeSpec{
			Name: &ast.Ident{
				Name: "Test",
			},
			Type: &ast.FuncType{},
		},
		ParentSpec: &ast.FuncDecl{
			Name: ast.NewIdent("TestFuncDecl"),
		},
	}
	_, err = p.ParseDefinition(definition)
	assert.Error(t, err)
	assert.Equal(t, "model.TestFuncDecl.Test", definition.TypeName())
}

func TestParserParseDefinitionV3OmitsGinBindingFormTagFromSchema(t *testing.T) {
	t.Parallel()

	src := `
package api

type Example struct {
	Status string ` + "`json:\"status\" binding:\"omitempty,oneof=a b c\" form:\"status\"`" + `
}
`

	p := New(GenerateOpenAPI3Doc(true))
	err := p.packages.ParseFile("api", "api/api.go", src, ParseAll)
	assert.NoError(t, err)
	err = p.packages.ParseTypes()
	assert.NoError(t, err)

	definition := p.packages.uniqueDefinitions["api.Example"]
	require.NotNil(t, definition)

	schema, err := p.ParseDefinition(definition)
	assert.NoError(t, err)
	status, ok := schema.Properties.Get("status")
	require.True(t, ok)
	require.NotNil(t, status)

	assert.JSONEq(t, `{"type":"string","enum":["a","b","c"]}`, renderSchemaProxyJSON(t, status))
}

func TestParseGeneralAPIInfoSkipsOperationDescription(t *testing.T) {
	t.Parallel()

	p := New(GenerateOpenAPI3Doc(true))
	err := p.ParseGeneralAPIInfo("testdata/v3/general_info_health/main.go")
	assert.NoError(t, err)

	assert.Equal(t, "Manages accounts and their lifecycle.", p.openAPI.Info.Description,
		"the health handler's @Description must not clobber the API description")
	assert.Equal(t, "Account API", p.openAPI.Info.Title)
}

func TestParserParseGeneralApiInfo(t *testing.T) {
	t.Parallel()

	gopath := os.Getenv("GOPATH")
	assert.NotNil(t, gopath)

	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseGeneralAPIInfo("testdata/v3/main.go")
	assert.NoError(t, err)

	assert.Equal(t, "This is a sample server Petstore server.\nIt has a lot of beautiful features.", p.openAPI.Info.Description)
	assert.Equal(t, "Swagger Example API", p.openAPI.Info.Title)
	assert.Equal(t, "http://swagger.io/terms/", p.openAPI.Info.TermsOfService)
	assert.Equal(t, "API Support", p.openAPI.Info.Contact.Name)
	assert.Equal(t, "http://www.swagger.io/support", p.openAPI.Info.Contact.URL)
	assert.Equal(t, "support@swagger.io", p.openAPI.Info.Contact.Email)
	assert.Equal(t, "Apache 2.0", p.openAPI.Info.License.Name)
	assert.Equal(t, "http://www.apache.org/licenses/LICENSE-2.0.html", p.openAPI.Info.License.URL)
	assert.Equal(t, "1.0", p.openAPI.Info.Version)

	xLogo := map[string]interface{}(map[string]interface{}{"altText": "Petstore logo", "backgroundColor": "#FFFFFF", "url": "https://redocly.github.io/redoc/petstore-logo.png"})
	assert.Equal(t, xLogo, extVal(t, p.openAPI.Info.Extensions, "x-logo"))
	assert.Equal(t, "marks values", extVal(t, p.openAPI.Info.Extensions, "x-google-marks"))

	endpoints := interface{}([]interface{}{map[string]interface{}{"allowCors": true, "name": "name.endpoints.environment.cloud.goog"}})
	assert.Equal(t, endpoints, extVal(t, p.openAPI.Info.Extensions, "x-google-endpoints"))

	assert.Equal(t, "OpenAPI", p.openAPI.ExternalDocs.Description)
	assert.Equal(t, "https://swagger.io/resources/open-api", p.openAPI.ExternalDocs.URL)

	assert.Equal(t, 8, p.openAPI.Components.SecuritySchemes.Len())

	security := p.openAPI.Components.SecuritySchemes
	if v, ok := security.Get("basic"); ok && v != nil {
		assert.Equal(t, "basic", v.Scheme)
		assert.Equal(t, "http", v.Type)
	}
	if v, ok := security.Get("ApiKeyAuth"); ok && v != nil {
		assert.Equal(t, "apiKey", v.Type)
		assert.Equal(t, "Authorization", v.Name)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "some description", v.Description)
	}
	if v, ok := security.Get("OAuth2Application"); ok && v != nil {
		assert.Equal(t, "oauth2", v.Type)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "https://example.com/oauth/token", v.Flows.ClientCredentials.TokenUrl)
		assert.Equal(t, 2, v.Flows.ClientCredentials.Scopes.Len())
	}
	if v, ok := security.Get("OAuth2Implicit"); ok && v != nil {
		assert.Equal(t, "oauth2", v.Type)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "https://example.com/oauth/authorize", v.Flows.Implicit.AuthorizationUrl)
		assert.Equal(t, "some_audience.google.com", extVal(t, v.Flows.Extensions, "x-google-audiences"))
	}
	if v, ok := security.Get("OAuth2Password"); ok && v != nil {
		assert.Equal(t, "oauth2", v.Type)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "https://example.com/oauth/token", v.Flows.Password.TokenUrl)
	}
	if v, ok := security.Get("OAuth2AccessCode"); ok && v != nil {
		assert.Equal(t, "oauth2", v.Type)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "https://example.com/oauth/token", v.Flows.AuthorizationCode.TokenUrl)
	}
	if v, ok := security.Get("BearerAuth1"); ok && v != nil {
		assert.Equal(t, "bearer", v.Scheme)
		assert.Equal(t, "http", v.Type)
		assert.Equal(t, "JWT", v.BearerFormat)
		assert.Equal(t, "First bearer token", v.Description)
	}
	if v, ok := security.Get("BearerAuth2"); ok && v != nil {
		assert.Equal(t, "bearer", v.Scheme)
		assert.Equal(t, "http", v.Type)
		assert.Equal(t, "CustomToken", v.BearerFormat)
		assert.Equal(t, "Second bearer token", v.Description)
	}
}

func TestParserParseGeneralApiInfoV3GroupedSecurityDefinitions(t *testing.T) {
	t.Parallel()

	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseGeneralAPIInfo("testdata/v3/security_grouped.go")
	require.NoError(t, err)

	security := p.openAPI.Components.SecuritySchemes
	require.Equal(t, 3, security.Len())

	if v, ok := security.Get("BearerAuth"); ok && v != nil {
		assert.Equal(t, "http", v.Type)
		assert.Equal(t, "bearer", v.Scheme)
		assert.Equal(t, "JWT", v.BearerFormat)
		assert.Equal(t, "Bearer access token.", v.Description)
	}
	if v, ok := security.Get("APIKeyAuth"); ok && v != nil {
		assert.Equal(t, "apiKey", v.Type)
		assert.Equal(t, "header", v.In)
		assert.Equal(t, "X-API-Key", v.Name)
		assert.Equal(t, "API key auth.", v.Description)
	}
	if v, ok := security.Get("SessionCookieAuth"); ok && v != nil {
		assert.Equal(t, "apiKey", v.Type)
		assert.Equal(t, "cookie", v.In)
		assert.Equal(t, "session_cookie", v.Name)
		assert.Equal(t, "Session cookie auth.", v.Description)
	}
}

func TestParser_ParseGeneralApiInfoExtensions(t *testing.T) {
	// should return an error because extension value is not a valid json
	t.Run("Test invalid extension value", func(t *testing.T) {
		t.Parallel()

		expected := "could not parse extension comment: annotation @x-google-endpoints need a valid json value. error: invalid character ':' after array element"
		gopath := os.Getenv("GOPATH")
		assert.NotNil(t, gopath)

		p := New(GenerateOpenAPI3Doc(true))

		err := p.ParseGeneralAPIInfo("testdata/v3/extensionsFail1.go")
		if assert.Error(t, err) {
			assert.Equal(t, expected, err.Error())
		}
	})

	// should return an error because extension don't have a value
	t.Run("Test missing extension value", func(t *testing.T) {
		t.Parallel()

		expected := "could not parse extension comment: annotation @x-google-endpoints need a value"
		gopath := os.Getenv("GOPATH")
		assert.NotNil(t, gopath)

		p := New(GenerateOpenAPI3Doc(true))

		err := p.ParseGeneralAPIInfo("testdata/v3/extensionsFail2.go")
		if assert.Error(t, err) {
			assert.Equal(t, expected, err.Error())
		}
	})
}

func TestParserParseGeneralApiInfoWithOpsInSameFile(t *testing.T) {
	t.Parallel()

	gopath := os.Getenv("GOPATH")
	assert.NotNil(t, gopath)

	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseGeneralAPIInfo("testdata/single_file_api/main.go")
	assert.NoError(t, err)

	assert.Equal(t, "This is a sample server Petstore server.\nIt has a lot of beautiful features.", p.openAPI.Info.Description)
	assert.Equal(t, "Swagger Example API", p.openAPI.Info.Title)
	assert.Equal(t, "http://swagger.io/terms/", p.openAPI.Info.TermsOfService)
}

func TestParserParseGeneralAPIInfoMarkdown(t *testing.T) {
	t.Parallel()

	p := New(SetMarkdownFileDirectory("testdata"), GenerateOpenAPI3Doc(true))
	mainAPIFile := "testdata/markdown.go"
	err := p.ParseGeneralAPIInfo(mainAPIFile)
	assert.NoError(t, err)

	assert.Equal(t, "Swagger Example API Markdown Description", p.openAPI.Info.Description)
	assert.Equal(t, "users", p.openAPI.Tags[0].Name)
	assert.Equal(t, "Users Tag Markdown Description", p.openAPI.Tags[0].Description)

	p = New(GenerateOpenAPI3Doc(true))

	err = p.ParseGeneralAPIInfo(mainAPIFile)
	assert.Error(t, err)
}

func TestParserParseGeneralAPIInfoMarkdownV3FilteredTags(t *testing.T) {
	t.Parallel()

	p := New(SetMarkdownFileDirectory("testdata"), SetTags("users"), GenerateOpenAPI3Doc(true))
	mainAPIFile := "testdata/markdown.go"

	err := p.ParseGeneralAPIInfo(mainAPIFile)
	assert.NoError(t, err)

	if assert.Len(t, p.openAPI.Tags, 1) {
		assert.Equal(t, "users", p.openAPI.Tags[0].Name)
		assert.Equal(t, "Users Tag Markdown Description", p.openAPI.Tags[0].Description)
	}
}

func TestParserParseGeneralApiInfoFailed(t *testing.T) {
	t.Parallel()

	gopath := os.Getenv("GOPATH")
	assert.NotNil(t, gopath)
	p := New(GenerateOpenAPI3Doc(true))
	assert.Error(t, p.ParseGeneralAPIInfo("testdata/noexist.go"))
}

func TestParserParseGeneralAPIInfoCollectionFormat(t *testing.T) {
	t.Parallel()

	parser := New(GenerateOpenAPI3Doc(true))
	assert.NoError(t, parser.parseGeneralAPIInfo([]string{
		"@query.collection.format csv",
	}))
	assert.Equal(t, parser.collectionFormatInQuery, "csv")

	assert.NoError(t, parser.parseGeneralAPIInfo([]string{
		"@query.collection.format tsv",
	}))
	assert.Equal(t, parser.collectionFormatInQuery, "tsv")
}

func TestParserParseGeneralAPITagGroups(t *testing.T) {
	t.Parallel()

	parser := New(GenerateOpenAPI3Doc(true))
	assert.NoError(t, parser.parseGeneralAPIInfo([]string{
		"@x-tagGroups [{\"name\":\"General\",\"tags\":[\"lanes\",\"video-recommendations\"]}]",
	}))

	expected := []interface{}{map[string]interface{}{"name": "General", "tags": []interface{}{"lanes", "video-recommendations"}}}
	assert.Equal(t, expected, extVal(t, parser.openAPI.Info.Extensions, "x-tagGroups"))
}

func TestParserParseGeneralAPITagDocs(t *testing.T) {
	t.Parallel()

	parser := New(GenerateOpenAPI3Doc(true))
	assert.Error(t, parser.parseGeneralAPIInfo([]string{
		"@tag.name Test",
		"@tag.docs.description Best example documentation"}))

	parser = New(GenerateOpenAPI3Doc(true))
	err := parser.parseGeneralAPIInfo([]string{
		"@tag.name test",
		"@tag.description A test Tag",
		"@tag.docs.url https://example.com",
		"@tag.docs.description Best example documentation"})
	assert.NoError(t, err)

	assert.Equal(t, "test", parser.openAPI.Tags[0].Name)
	assert.Equal(t, "A test Tag", parser.openAPI.Tags[0].Description)
	assert.Equal(t, "https://example.com", parser.openAPI.Tags[0].ExternalDocs.URL)
	assert.Equal(t, "Best example documentation", parser.openAPI.Tags[0].ExternalDocs.Description)
}

func TestParserParseGeneralAPITagExtensions(t *testing.T) {
	t.Parallel()

	parser := New(GenerateOpenAPI3Doc(true))
	err := parser.parseGeneralAPIInfo([]string{
		"@tag.name test",
		"@tag.x-displayName Test group",
		"@tag.x-group Cards"})
	assert.NoError(t, err)

	assert.Equal(t, "Test group", extVal(t, parser.openAPI.Tags[0].Extensions, "x-displayName"))
	assert.Equal(t, "Cards", extVal(t, parser.openAPI.Tags[0].Extensions, "x-group"))

	parser = New(GenerateOpenAPI3Doc(true))
	assert.Error(t, parser.parseGeneralAPIInfo([]string{
		"@tag.name test",
		"@tag.x-displayName"}))
}

func TestParserParseGeneralAPITagExtensionsOnFilteredTag(t *testing.T) {
	t.Parallel()

	parser := New(GenerateOpenAPI3Doc(true), SetTags("keep"))
	err := parser.parseGeneralAPIInfo([]string{
		"@tag.name keep",
		"@tag.x-group Kept",
		"@tag.name drop",
		"@tag.x-group Dropped"})
	assert.NoError(t, err)

	require.Len(t, parser.openAPI.Tags, 1)
	assert.Equal(t, "keep", parser.openAPI.Tags[0].Name)
	assert.Equal(t, "Kept", extVal(t, parser.openAPI.Tags[0].Extensions, "x-group"))
}

func newTaggedPathParser(t *testing.T, tags []string, paths [][2]string) *Parser {
	t.Helper()

	parser := New(GenerateOpenAPI3Doc(true))
	declarations := make([]string, 0, len(tags))
	for _, tag := range tags {
		declarations = append(declarations, "@tag.name "+tag)
	}
	require.NoError(t, parser.parseGeneralAPIInfo(declarations))

	for _, p := range paths {
		item := &v3.PathItem{Get: &v3.Operation{}}
		if p[1] != "" {
			item.Get.Tags = []string{p[1]}
		}
		parser.openAPI.Paths.PathItems.Set(p[0], item)
	}

	return parser
}

func pathOrder(p *Parser) []string {
	out := make([]string, 0)
	for path := range p.openAPI.Paths.PathItems.KeysFromOldest() {
		out = append(out, path)
	}

	return out
}

func TestParserOrderPathsByTags(t *testing.T) {
	t.Parallel()

	parser := newTaggedPathParser(t,
		[]string{"Card", "Account", "Financial"},
		[][2]string{
			{"/accounts", "Account"},
			{"/balance", "Financial"},
			{"/cards", "Card"},
			{"/cards/{id}", "Card"},
			{"/payments", "Financial"},
		})

	parser.orderPathsByTags()

	assert.Equal(t,
		[]string{"/cards", "/cards/{id}", "/accounts", "/balance", "/payments"},
		pathOrder(parser))
}

func TestParserOrderPathsByTagsKeepsUndeclaredLast(t *testing.T) {
	t.Parallel()

	parser := newTaggedPathParser(t,
		[]string{"Card"},
		[][2]string{
			{"/health", "health"},
			{"/echo", ""},
			{"/cards", "Card"},
		})

	parser.orderPathsByTags()

	assert.Equal(t, []string{"/cards", "/health", "/echo"}, pathOrder(parser))
}

func TestParserOrderPathsByTagsWithoutDeclaredTagsIsNoop(t *testing.T) {
	t.Parallel()

	parser := newTaggedPathParser(t, nil, [][2]string{
		{"/accounts", "Account"},
		{"/cards", "Card"},
	})

	parser.orderPathsByTags()

	assert.Equal(t, []string{"/accounts", "/cards"}, pathOrder(parser))
}

func TestGetAllGoFileInfo(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/pet"

	p := New(GenerateOpenAPI3Doc(true))
	err := p.getAllGoFileInfo("testdata", searchDir)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(p.packages.files))
}

func TestParser_ParseType(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/simple/"

	p := New(GenerateOpenAPI3Doc(true))
	err := p.getAllGoFileInfo("testdata", searchDir)
	assert.NoError(t, err)

	err = p.packages.ParseTypes()

	assert.NoError(t, err)
	assert.NotNil(t, p.packages.uniqueDefinitions["api.Pet3"])
	assert.NotNil(t, p.packages.uniqueDefinitions["web.Pet"])
	assert.NotNil(t, p.packages.uniqueDefinitions["web.Pet2"])
}

func TestParsePet(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/pet"

	p := New(GenerateOpenAPI3Doc(true))
	p.PropNamingStrategy = PascalCase

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	assert.NoError(t, err)

	schemas := p.openAPI.Components.Schemas
	assert.NotNil(t, schemas)

	tagSchema := schemas.GetOrZero("web.Tag").Schema()
	assert.Equal(t, 2, tagSchema.Properties.Len())
	assert.Equal(t, []string{INTEGER}, tagSchema.Properties.GetOrZero("id").Schema().Type)
	assert.Equal(t, []string{STRING}, tagSchema.Properties.GetOrZero("name").Schema().Type)

	petSchema := schemas.GetOrZero("web.Pet").Schema()
	assert.NotNil(t, petSchema)
	assert.Equal(t, 8, petSchema.Properties.Len())
	assert.Equal(t, []string{INTEGER}, petSchema.Properties.GetOrZero("id").Schema().Type)
	assert.Equal(t, []string{STRING}, petSchema.Properties.GetOrZero("name").Schema().Type)

}

func TestParseSimpleApi(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/simple"
	p := New(GenerateOpenAPI3Doc(true))
	p.PropNamingStrategy = PascalCase

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	assert.NoError(t, err)

	paths := p.openAPI.Paths.PathItems

	op := paths.GetOrZero("/testapi/get-string-by-int/{some_id}").Get
	assert.Equal(t, "get string by ID", op.Description)
	assert.Equal(t, "Add a new pet to the store", op.Summary)
	assert.Equal(t, "get-string-by-int", op.OperationId)

	response := op.Responses.Codes.GetOrZero("200")
	assert.Equal(t, "ok", response.Description)

	formOp := paths.GetOrZero("/FormData").Post
	assert.NotNil(t, formOp)
	assert.NotNil(t, formOp.RequestBody)
	//TODO add asserts

	t.Run("Test parse struct oneOf", func(t *testing.T) {
		t.Parallel()

		// Render the whole document once and extract each component schema's JSON;
		// a hand-built *base.Schema can't be marshalled standalone.
		docJSON, err := p.openAPI.RenderJSON("")
		require.NoError(t, err)
		var docMap map[string]interface{}
		require.NoError(t, json.Unmarshal(docJSON, &docMap))
		renderedSchemas := docMap["components"].(map[string]interface{})["schemas"].(map[string]interface{})
		schemaJSON := func(name string) string {
			b, err := json.Marshal(renderedSchemas[name])
			require.NoError(t, err)
			return string(b)
		}

		_, ok := p.openAPI.Components.Schemas.Get("web.OneOfTest")
		assert.True(t, ok)
		expected := `{
    "properties": {
        "big_int": {
            "oneOf": [
                {
                    "type": "string"
                },
                {
                    "type": "integer"
                }
            ]
        },
        "pet_detail": {
            "oneOf": [
                {
                    "$ref": "#/components/schemas/web.Cat"
                },
                {
                    "$ref": "#/components/schemas/web.Dog"
                }
            ]
        }
    },
    "type": "object"
}`
		assert.JSONEq(t, expected, schemaJSON("web.OneOfTest"))

		_, ok = p.openAPI.Components.Schemas.Get("web.Cat")
		assert.True(t, ok)
		expected = `{
    "properties": {
        "age": {
            "type": "integer"
        },
        "hunts": {
            "type": "boolean"
        }
    },
    "type": "object"
}`
		assert.JSONEq(t, expected, schemaJSON("web.Cat"))

		_, ok = p.openAPI.Components.Schemas.Get("web.Dog")
		assert.True(t, ok)
		expected = `{
    "properties": {
        "bark": {
            "type": "boolean"
        },
        "breed": {
            "enum": [
                "Dingo",
                "Husky",
                "Retriever",
                "Shepherd"
            ],
            "type": "string"
        }
    },
    "type": "object"
}`
		assert.JSONEq(t, expected, schemaJSON("web.Dog"))
	})

	t.Run("Test parse response oneOf", func(t *testing.T) {
		t.Parallel()

		_, ok := paths.Get("/pets/{id}")
		assert.True(t, ok)
		path := paths.GetOrZero("/pets/{id}")
		_, ok = path.Get.Responses.Codes.Get("200")
		assert.True(t, ok)
		response = path.Get.Responses.Codes.GetOrZero("200")
		assert.Equal(t, "Return Cat or Dog", response.Description)
		mediaType := response.Content.GetOrZero("application/json")
		rootSchema := mediaType.Schema.Schema()
		require.Len(t, rootSchema.OneOf, 2)
		assert.True(t, rootSchema.OneOf[0].IsReference())
		assert.Equal(t, "#/components/schemas/web.Cat", rootSchema.OneOf[0].GetReference())
		assert.True(t, rootSchema.OneOf[1].IsReference())
		assert.Equal(t, "#/components/schemas/web.Dog", rootSchema.OneOf[1].GetReference())

	})
}

func TestParserParseServers(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/servers"
	p := New(GenerateOpenAPI3Doc(true))
	p.PropNamingStrategy = PascalCase

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	assert.NoError(t, err)

	servers := p.openAPI.Servers
	require.NotNil(t, servers)

	assert.Equal(t, 2, len(servers))
	assert.Equal(t, "{scheme}://{host}:{port}", servers[0].URL)
	assert.Equal(t, "Test Petstore server.", servers[0].Description)

	assert.Equal(t, "https", servers[0].Variables.GetOrZero("scheme").Default)
	assert.Equal(t, []string{"http", "https"}, servers[0].Variables.GetOrZero("scheme").Enum)
	assert.Equal(t, "test.petstore.com", servers[0].Variables.GetOrZero("host").Default)
	assert.Equal(t, "443", servers[0].Variables.GetOrZero("port").Default)

	assert.Equal(t, "https://petstore.com/v3", servers[1].URL)
	assert.Equal(t, "Production Petstore server.", servers[1].Description)

}

func TestParserParseGeneralAPIInfoGlobalSecurity(t *testing.T) {
	t.Parallel()

	// Test simple global security
	parser := New(GenerateOpenAPI3Doc(true))
	err := parser.parseGeneralAPIInfo([]string{
		"@security ApiKeyAuth",
	})
	assert.NoError(t, err)
	assert.Len(t, parser.openAPI.Security, 1)
	_, ok := parser.openAPI.Security[0].Requirements.Get("ApiKeyAuth")
	assert.True(t, ok)
	assert.Equal(t, []string{}, parser.openAPI.Security[0].Requirements.GetOrZero("ApiKeyAuth"))

	// Test OAuth2 with scopes
	parser2 := New(GenerateOpenAPI3Doc(true))
	err2 := parser2.parseGeneralAPIInfo([]string{
		"@security OAuth2Implicit[read,write]",
	})
	assert.NoError(t, err2)
	assert.Len(t, parser2.openAPI.Security, 1)
	_, ok = parser2.openAPI.Security[0].Requirements.Get("OAuth2Implicit")
	assert.True(t, ok)
	assert.Equal(t, []string{"read", "write"}, parser2.openAPI.Security[0].Requirements.GetOrZero("OAuth2Implicit"))

	// Test OR logic
	parser3 := New(GenerateOpenAPI3Doc(true))
	err3 := parser3.parseGeneralAPIInfo([]string{
		"@security ApiKeyAuth || BasicAuth",
	})
	assert.NoError(t, err3)
	assert.Len(t, parser3.openAPI.Security, 1)
	_, ok = parser3.openAPI.Security[0].Requirements.Get("ApiKeyAuth")
	assert.True(t, ok)
	_, ok = parser3.openAPI.Security[0].Requirements.Get("BasicAuth")
	assert.True(t, ok)
	assert.Equal(t, []string{}, parser3.openAPI.Security[0].Requirements.GetOrZero("ApiKeyAuth"))
	assert.Equal(t, []string{}, parser3.openAPI.Security[0].Requirements.GetOrZero("BasicAuth"))

	// Test AND logic (multiple @security lines)
	parser4 := New(GenerateOpenAPI3Doc(true))
	err4 := parser4.parseGeneralAPIInfo([]string{
		"@security ApiKeyAuth",
		"@security BasicAuth",
	})
	assert.NoError(t, err4)
	assert.Len(t, parser4.openAPI.Security, 2)
	_, ok = parser4.openAPI.Security[0].Requirements.Get("ApiKeyAuth")
	assert.True(t, ok)
	_, ok = parser4.openAPI.Security[1].Requirements.Get("BasicAuth")
	assert.True(t, ok)
}

func TestParseTypeAlias(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/type_alias_definition"

	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	require.NoError(t, err)

	expected, err := os.ReadFile(filepath.Join(searchDir, "expected.json"))
	require.NoError(t, err)

	result, err := p.openAPI.RenderJSON("")
	require.NoError(t, err)

	assert.JSONEq(t, string(expected), string(result))
}

func TestParseInterface(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/v3/interface"

	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	require.NoError(t, err)

	expected, err := os.ReadFile(filepath.Join(searchDir, "expected.json"))
	require.NoError(t, err)

	result, err := p.openAPI.RenderJSON("")
	require.NoError(t, err)

	assert.JSONEq(t, string(expected), string(result))
}

func TestParseRecursionWithSchemaName(t *testing.T) {
	t.Parallel()

	searchDir := "testdata/recursion_schema_name"
	p := New(GenerateOpenAPI3Doc(true))

	err := p.ParseAPI(searchDir, mainAPIFile, defaultParseDepth)
	require.NoError(t, err)

	userSchema, exists := p.openAPI.Components.Schemas.Get("User")
	require.True(t, exists, "User schema should exist")
	require.NotNil(t, userSchema, "User schema should not be nil")
	userSpec := userSchema.Schema()
	require.NotNil(t, userSpec, "User schema spec should not be nil")

	assert.Equal(t, "object", userSpec.Type[0])

	childrenProp, exists := userSpec.Properties.Get("children")
	require.True(t, exists, "children property should exist")
	childrenSpec := childrenProp.Schema()
	require.NotNil(t, childrenSpec, "children property spec should not be nil")

	assert.Equal(t, "array", childrenSpec.Type[0])

	require.NotNil(t, childrenSpec.Items, "children items should not be nil")
	require.NotNil(t, childrenSpec.Items.A, "children items schema should not be nil")

	expectedRef := "#/components/schemas/User"
	assert.Equal(t, expectedRef, childrenSpec.Items.A.GetReference())
}

func TestGetSchemaByRef(t *testing.T) {
	t.Parallel()

	p := New(GenerateOpenAPI3Doc(true))
	p.openAPI.Components.Schemas = orderedmap.New[string, *base.SchemaProxy]()

	t.Run("Existing schema", func(t *testing.T) {
		testSchema := &base.Schema{Type: []string{"string"}}
		p.openAPI.Components.Schemas.Set("TestSchema", base.CreateSchemaProxy(testSchema))

		result := p.getSchemaByRef("#/components/schemas/TestSchema")

		require.NotNil(t, result)
		assert.Equal(t, testSchema, result)
	})

	t.Run("Non-existing schema returns empty schema", func(t *testing.T) {
		result := p.getSchemaByRef("#/components/schemas/NonExistentSchema")

		require.NotNil(t, result)
		assert.Equal(t, &base.Schema{}, result)
	})
}

func TestEmptyExternalDocsOmitted(t *testing.T) {
	p := New()
	assert.Nil(t, p.openAPI.ExternalDocs)

	b, err := p.openAPI.RenderJSON("")
	require.NoError(t, err)
	assert.NotContains(t, string(b), `"externalDocs"`)

	// After setting a real URL, externalDocs should appear.
	p.openAPI.ExternalDocs = &base.ExternalDoc{URL: "https://example.com/docs"}
	b, err = p.openAPI.RenderJSON("")
	require.NoError(t, err)
	assert.Contains(t, string(b), `"externalDocs"`)
	assert.Contains(t, string(b), "https://example.com/docs")
}

func TestAutoXOrderEmbeddedNoDup(t *testing.T) {
	p := New(GenerateOpenAPI3Doc(true))
	p.AutoOrderProperties = true
	err := p.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = p.packages.ParseTypes()
	require.NoError(t, err)

	// resolve both the embedded base and the embedder
	td := p.packages.uniqueDefinitions["param_structs.Embedder"]
	if td == nil {
		td = p.packages.uniqueDefinitions["structs.Embedder"]
	}
	require.NotNil(t, td, "Embedder type not found")
	schema, err := p.ParseDefinition(td)
	require.NoError(t, err)

	seen := map[string]string{}
	for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
		name := pair.Key()
		prop := pair.Value()
		require.False(t, prop.IsReference(), "prop %s should be inline", name)
		ps := prop.Schema()
		require.NotNil(t, ps, "prop %s should be inline", name)
		var xo string
		if node, ok := extGet(ps.Extensions, "x-order"); ok {
			require.NoError(t, node.Decode(&xo))
		}
		require.NotEmpty(t, xo, "prop %s missing x-order", name)
		if other, dup := seen[xo]; dup {
			t.Fatalf("duplicate x-order %s on %s and %s", xo, other, name)
		}
		seen[xo] = name
	}
	require.Len(t, seen, 3, "alpha, beta, gamma each get a unique x-order")
}

func TestEnumAliasNoDoubleEnum(t *testing.T) {
	p := New(GenerateOpenAPI3Doc(true))
	require.NoError(t, p.parseFile("github.com/FunnelDash/swag/v3/testdata/enum_alias/target", "testdata/enum_alias/target/target.go", nil, ParseAll))
	require.NoError(t, p.parseFile("github.com/FunnelDash/swag/v3/testdata/enum_alias", "testdata/enum_alias/alias.go", nil, ParseAll))
	err := p.packages.ParseTypes()
	require.NoError(t, err)

	td := p.packages.uniqueDefinitions["enum_alias.Holder"]
	require.NotNil(t, td, "Holder not found")
	_, err = p.ParseDefinition(td)
	require.NoError(t, err)

	for pair := p.openAPI.Components.Schemas.First(); pair != nil; pair = pair.Next() {
		name := pair.Key()
		s := pair.Value().Schema()
		if strings.HasSuffix(name, "Status") && s != nil && len(s.Enum) > 0 {
			vals := enumValues(t, s.Enum)
			assert.ElementsMatch(t, []interface{}{"open", "closed"}, vals,
				"enum on %s must not be doubled", name)
			assert.Len(t, s.Enum, 2, "enum on %s must not be doubled", name)
		}
	}
}

func TestOperationIDDefaultsToFuncName(t *testing.T) {
	p := New(GenerateOpenAPI3Doc(true))
	require.NoError(t, p.ParseAPI("testdata/v3/operationid", mainAPIFile, defaultParseDepth))

	paths := p.openAPI.Paths.PathItems
	// no @ID -> operationId defaults to the handler func name
	require.NotNil(t, paths.GetOrZero("/bare"))
	assert.Equal(t, "ListWidgets", paths.GetOrZero("/bare").Get.OperationId)
	// explicit @ID wins over the default
	require.NotNil(t, paths.GetOrZero("/explicit"))
	assert.Equal(t, "custom-explicit-id", paths.GetOrZero("/explicit").Get.OperationId)
}
