package swag

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	goparser "go/parser"
	"go/token"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KyleBanks/depth"
	openapi "github.com/sv-tools/openapi/spec"
)

const (
	// CamelCase indicates using CamelCase strategy for struct field.
	CamelCase = "camelcase"

	// PascalCase indicates using PascalCase strategy for struct field.
	PascalCase = "pascalcase"

	// SnakeCase indicates using SnakeCase strategy for struct field.
	SnakeCase = "snakecase"

	idAttr               = "@id"
	acceptAttr           = "@accept"
	produceAttr          = "@produce"
	paramAttr            = "@param"
	successAttr          = "@success"
	failureAttr          = "@failure"
	responseAttr         = "@response"
	headerAttr           = "@header"
	tagsAttr             = "@tags"
	routerAttr           = "@router"
	deprecatedRouterAttr = "@deprecatedrouter"

	summaryAttr              = "@summary"
	deprecatedAttr           = "@deprecated"
	securityAttr             = "@security"
	titleAttr                = "@title"
	conNameAttr              = "@contact.name"
	conURLAttr               = "@contact.url"
	conEmailAttr             = "@contact.email"
	licNameAttr              = "@license.name"
	licURLAttr               = "@license.url"
	versionAttr              = "@version"
	descriptionAttr          = "@description"
	descriptionMarkdownAttr  = "@description.markdown"
	secBasicAttr             = "@securitydefinitions.basic"
	secAPIKeyAttr            = "@securitydefinitions.apikey"
	secBearerAuthAttr        = "@securitydefinitions.bearerauth"
	secApplicationAttr       = "@securitydefinitions.oauth2.application"
	secImplicitAttr          = "@securitydefinitions.oauth2.implicit"
	secPasswordAttr          = "@securitydefinitions.oauth2.password"
	secAccessCodeAttr        = "@securitydefinitions.oauth2.accesscode"
	tosAttr                  = "@termsofservice"
	extDocsDescAttr          = "@externaldocs.description"
	extDocsURLAttr           = "@externaldocs.url"
	xCodeSamplesAttr         = "@x-codesamples"
	xCodeSamplesAttrOriginal = "@x-codeSamples"
	scopeAttrPrefix          = "@scope."
	stateAttr                = "@state"
)

// ParseFlag determine what to parse
type ParseFlag int

const (
	// ParseNone parse nothing
	ParseNone ParseFlag = 0x00
	// ParseModels parse models
	ParseModels = 0x01
	// ParseOperations parse operations
	ParseOperations = 0x02
	// ParseAll parse operations and models
	ParseAll = ParseOperations | ParseModels
)

var (
	// ErrRecursiveParseStruct recursively parsing struct.
	ErrRecursiveParseStruct = errors.New("recursively parsing struct")

	// ErrFuncTypeField field type is func.
	ErrFuncTypeField = errors.New("field type is func")

	// ErrFailedConvertPrimitiveType Failed to convert for swag to interpretable type.
	ErrFailedConvertPrimitiveType = errors.New("swag property: failed convert primitive type")

	// ErrSkippedField .swaggo specifies field should be skipped.
	ErrSkippedField = errors.New("field is skipped by global overrides")
)

var allMethod = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPut:     {},
	http.MethodPost:    {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
	http.MethodHead:    {},
	http.MethodPatch:   {},
}

// Parser implements a parser for Go source files.
type Parser struct {
	// openAPI represents the v3.1 root document object for the API specification
	openAPI *openapi.OpenAPI

	// packages store entities of APIs, definitions, file, package path etc.  and their relations
	packages *PackagesDefinitions

	// parsedSchemas store schemas which have been parsed from ast.TypeSpec
	parsedSchemas map[*TypeSpecDef]*Schema

	// outputSchemas store schemas which will be export to swagger
	outputSchemas map[*TypeSpecDef]*Schema

	// PropNamingStrategy naming strategy
	PropNamingStrategy string

	// ParseVendor parse vendor folder
	ParseVendor bool

	// ParseDependencies whether swag should be parse outside dependency folder: 0 none, 1 models, 2 operations, 3 all
	ParseDependency ParseFlag

	// ParseInternal whether swag should parse internal packages
	ParseInternal bool

	// Strict whether swag should error or warn when it detects cases which are most likely user errors
	Strict bool

	// RequiredByDefault set validation required for all fields by default
	RequiredByDefault bool

	// AutoOrderProperties stamps an x-order extension on every struct property
	// from its declaration index, so renderers show fields in source order
	// without hand-maintained `extensions:"x-order=NN"` tags.
	AutoOrderProperties bool

	// structStack stores full names of the structures that were already parsed or are being parsed now
	structStack []*TypeSpecDef

	// markdownFileDir holds the path to the folder, where markdown files are stored
	markdownFileDir string

	// codeExampleFilesDir holds path to the folder, where code example files are stored
	codeExampleFilesDir string

	// collectionFormatInQuery set the default collectionFormat otherwise then 'csv' for array in query params
	collectionFormatInQuery string

	// excludes excludes dirs and files in SearchDir
	excludes map[string]struct{}

	// packagePrefix is a list of package path prefixes, packages that do not
	// match any one of them will be excluded when searching.
	packagePrefix []string

	// tells parser to include only specific extension
	parseExtension string

	// debugging output goes here
	debug Debugger

	// fieldParserFactory create FieldParser
	fieldParserFactory FieldParserFactory

	// Overrides allows global replacements of types. A blank replacement will be skipped.
	Overrides map[string]string

	// parseGoList whether swag use go list to parse dependency
	parseGoList bool

	// tags to filter the APIs after
	tags map[string]struct{}

	// HostState is the state of the host
	HostState string

	// ParseFuncBody whether swag should parse api info inside of funcs
	ParseFuncBody bool
}

// Debugger is the interface that wraps the basic Printf method.
type Debugger interface {
	Printf(format string, v ...interface{})
}

// New creates a new Parser with default properties.
func New(options ...func(*Parser)) *Parser {
	parser := &Parser{
		openAPI: &openapi.OpenAPI{
			Info:         openapi.NewInfo(),
			OpenAPI:      "3.1.0",
			Components:   openapi.NewComponents(),
			ExternalDocs: nil,
			Paths:        openapi.NewPaths(),
			WebHooks:     map[string]*openapi.RefOrSpec[openapi.Extendable[openapi.PathItem]]{},
			Security:     []openapi.SecurityRequirement{},
			Tags:         []*openapi.Extendable[openapi.Tag]{},
			Servers:      []*openapi.Extendable[openapi.Server]{},
		},
		packages:           NewPackagesDefinitions(),
		debug:              log.New(os.Stdout, "", log.LstdFlags),
		parsedSchemas:      make(map[*TypeSpecDef]*Schema),
		outputSchemas:      make(map[*TypeSpecDef]*Schema),
		excludes:           make(map[string]struct{}),
		tags:               make(map[string]struct{}),
		fieldParserFactory: newTagBaseFieldParser,
		Overrides:          make(map[string]string),
	}
	for _, option := range options {
		option(parser)
	}
	parser.packages.debug = parser.debug
	return parser
}

// SetParseDependency sets whether to parse the dependent packages.
func SetParseDependency(parseDependency int) func(*Parser) {
	return func(p *Parser) {
		p.ParseDependency = ParseFlag(parseDependency)
		if p.packages != nil {
			p.packages.parseDependency = p.ParseDependency
		}
	}
}

// SetMarkdownFileDirectory sets the directory to search for markdown files.
func SetMarkdownFileDirectory(directoryPath string) func(*Parser) {
	return func(p *Parser) {
		p.markdownFileDir = directoryPath
	}
}

// SetCodeExamplesDirectory sets the directory to search for code example files.
func SetCodeExamplesDirectory(directoryPath string) func(*Parser) {
	return func(p *Parser) {
		p.codeExampleFilesDir = directoryPath
	}
}

// SetExcludedDirsAndFiles sets directories and files to be excluded when searching.
func SetExcludedDirsAndFiles(excludes string) func(*Parser) {
	return func(p *Parser) {
		for _, f := range strings.Split(excludes, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				f = filepath.Clean(f)
				p.excludes[f] = struct{}{}
			}
		}
	}
}

// SetPackagePrefix sets a list of package path prefixes from a comma-separated
// string, packages that do not match any one of them will be excluded when
// searching.
func SetPackagePrefix(packagePrefix string) func(*Parser) {
	return func(p *Parser) {
		for _, f := range strings.Split(packagePrefix, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				p.packagePrefix = append(p.packagePrefix, f)
			}
		}
	}
}

// SetTags sets the tags to be included
func SetTags(include string) func(*Parser) {
	return func(p *Parser) {
		for _, f := range strings.Split(include, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				p.tags[f] = struct{}{}
			}
		}
	}
}

// SetParseExtension parses only those operations which match given extension
func SetParseExtension(parseExtension string) func(*Parser) {
	return func(p *Parser) {
		p.parseExtension = parseExtension
	}
}

// SetStrict sets whether swag should error or warn when it detects cases which are most likely user errors.
func SetStrict(strict bool) func(*Parser) {
	return func(p *Parser) {
		p.Strict = strict
	}
}

// SetDebugger allows the use of user-defined implementations.
func SetDebugger(logger Debugger) func(parser *Parser) {
	return func(p *Parser) {
		if logger != nil {
			p.debug = logger
		}
	}
}

// SetFieldParserFactory allows the use of user-defined implementations.

// SetOverrides allows the use of user-defined global type overrides.
func SetOverrides(overrides map[string]string) func(parser *Parser) {
	return func(p *Parser) {
		for k, v := range overrides {
			p.Overrides[k] = v
		}
	}
}

// SetCollectionFormat set default collection format
func SetCollectionFormat(collectionFormat string) func(*Parser) {
	return func(p *Parser) {
		p.collectionFormatInQuery = collectionFormat
	}
}

// ParseUsingGoList sets whether swag use go list to parse dependency
func ParseUsingGoList(enabled bool) func(parser *Parser) {
	return func(p *Parser) {
		p.parseGoList = enabled
	}
}

// GenerateOpenAPI3Doc parses only those operations which match given extension
func GenerateOpenAPI3Doc(enable bool) func(*Parser) {
	return func(*Parser) {}
}

// ParseAPI parses general api info for given searchDir and mainAPIFile.
func (parser *Parser) ParseAPI(searchDir string, mainAPIFile string, parseDepth int) error {
	return parser.ParseAPIMultiSearchDir([]string{searchDir}, mainAPIFile, parseDepth)
}

// skipPackageByPrefix returns true the given pkgpath does not match
// any user-defined package path prefixes.
func (parser *Parser) skipPackageByPrefix(pkgpath string) bool {
	if len(parser.packagePrefix) == 0 {
		return false
	}
	for _, prefix := range parser.packagePrefix {
		if strings.HasPrefix(pkgpath, prefix) {
			return false
		}
	}
	return true
}

// ParseAPIMultiSearchDir is like ParseAPI but for multiple search dirs.
func (parser *Parser) ParseAPIMultiSearchDir(searchDirs []string, mainAPIFile string, parseDepth int) error {
	for _, searchDir := range searchDirs {
		parser.debug.Printf("Generate general API Info, search dir:%s", searchDir)

		packageDir, err := getPkgName(searchDir)
		if err != nil {
			parser.debug.Printf("warning: failed to get package name in dir: %s, error: %s", searchDir, err.Error())
		}

		err = parser.getAllGoFileInfo(packageDir, searchDir)
		if err != nil {
			return err
		}
	}

	absMainAPIFilePath, err := filepath.Abs(filepath.Join(searchDirs[0], mainAPIFile))
	if err != nil {
		return err
	}

	// Use 'go list' command instead of depth.Resolve()
	if parser.ParseDependency > 0 {
		if parser.parseGoList {
			pkgs, err := listPackages(context.Background(), filepath.Dir(absMainAPIFilePath), nil, "-deps")
			if err != nil {
				return fmt.Errorf("pkg %s cannot find all dependencies, %s", filepath.Dir(absMainAPIFilePath), err)
			}

			length := len(pkgs)
			for i := 0; i < length; i++ {
				err := parser.getAllGoFileInfoFromDepsByList(pkgs[i], parser.ParseDependency)
				if err != nil {
					return err
				}
			}
		} else {
			var t depth.Tree
			t.ResolveInternal = true
			t.MaxDepth = parseDepth

			pkgName, err := getPkgName(filepath.Dir(absMainAPIFilePath))
			if err != nil {
				return err
			}

			err = t.Resolve(pkgName)
			if err != nil {
				return fmt.Errorf("pkg %s cannot find all dependencies, %s", pkgName, err)
			}
			for i := 0; i < len(t.Root.Deps); i++ {
				err := parser.getAllGoFileInfoFromDeps(&t.Root.Deps[i], parser.ParseDependency)
				if err != nil {
					return err
				}
			}
		}
	}

	err = parser.ParseGeneralAPIInfo(absMainAPIFilePath)
	if err != nil {
		return err
	}

	err = parser.packages.ParseTypes()
	if err != nil {
		return err
	}

	if err = parser.packages.RangeFiles(parser.ParseRouterAPIInfo); err != nil {
		return err
	}

	return nil
}

func (parser *Parser) parseDeps(absMainAPIFilePath string, parseDepth int) error {
	if parser.parseGoList {
		pkgs, err := listPackages(context.Background(), filepath.Dir(absMainAPIFilePath), nil, "-deps")
		if err != nil {
			return fmt.Errorf("pkg %s cannot find all dependencies, %s", filepath.Dir(absMainAPIFilePath), err)
		}

		length := len(pkgs)
		for i := 0; i < length; i++ {
			err := parser.getAllGoFileInfoFromDepsByList(pkgs[i], parser.ParseDependency)
			if err != nil {
				return err
			}
		}
	} else {
		var t depth.Tree
		t.ResolveInternal = true
		t.MaxDepth = parseDepth

		pkgName, err := getPkgName(absMainAPIFilePath)
		if err != nil {
			return fmt.Errorf("could not parse dependencies: %w", err)
		}

		if err = t.Resolve(pkgName); err != nil {
			return fmt.Errorf("could not resolve dependencies: pkg %s cannot find all dependencies, %w", pkgName, err)
		}

		for i := 0; i < len(t.Root.Deps); i++ {
			if err = parser.getAllGoFileInfoFromDeps(&t.Root.Deps[i], parser.ParseDependency); err != nil {
				return fmt.Errorf("could not parse dependencies: %w", err)
			}
		}
	}

	return nil
}

func getPkgName(searchDir string) (string, error) {
	cmd := exec.Command("go", "list", "-f={{.ImportPath}}")
	cmd.Dir = searchDir

	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	fmt.Println("get pkg name for directory:", searchDir)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("execute go list command, %s, stdout:%s, stderr:%s", err, stdout.String(), stderr.String())
	}

	outStr, _ := stdout.String(), stderr.String()

	if outStr[0] == '_' { // will shown like _/{GOPATH}/src/{YOUR_PACKAGE} when NOT enable GO MODULE.
		outStr = strings.TrimPrefix(outStr, "_"+build.Default.GOPATH+"/src/")
	}

	f := strings.Split(outStr, "\n")

	outStr = f[0]

	return outStr, nil
}

// ParseGeneralAPIInfo parses general api info for given mainAPIFile path.
func (parser *Parser) ParseGeneralAPIInfo(mainAPIFile string) error {
	fileSet := token.NewFileSet()
	filePath := mainAPIFile

	fileTree, err := goparser.ParseFile(fileSet, filePath, nil, goparser.ParseComments)
	if err != nil {
		return fmt.Errorf("cannot parse source files %s: %s", filePath, err)
	}

	for i := range fileTree.Comments {
		comment := fileTree.Comments[i]
		if !isGeneralAPIComment(comment.Text()) {
			continue
		}

		// An operation's doc block (has an @Router line) can carry @Description /
		// @Summary attributes that isGeneralAPIComment — which only inspects the
		// group's first token — lets through, clobbering the real API info (the
		// health handler's "Health status of the service." is the usual victim).
		if isOperationCommentGroup(comment.Text()) {
			continue
		}

		comments := strings.Split(comment.Text(), "\n")

		if err = parser.parseGeneralAPIInfo(comments); err != nil {
			return err
		}
	}

	return nil
}

func parseSecurity(commentLine string) map[string][]string {
	securityMap := make(map[string][]string)

	for _, securityOption := range strings.Split(commentLine, "||") {
		securityOption = strings.TrimSpace(securityOption)

		left, right := strings.Index(securityOption, "["), strings.Index(securityOption, "]")

		if !(left == -1 && right == -1) {
			scopes := securityOption[left+1 : right]

			var options []string

			for _, scope := range strings.Split(scopes, ",") {
				options = append(options, strings.TrimSpace(scope))
			}

			securityKey := securityOption[0:left]
			securityMap[securityKey] = append(securityMap[securityKey], options...)
		} else {
			securityKey := strings.TrimSpace(securityOption)
			securityMap[securityKey] = []string{}
		}
	}

	return securityMap
}

// ParseAcceptComment parses comment for given `accept` comment string.

// ParseProduceComment parses comment for given `produce` comment string.

func isOperationCommentGroup(commentText string) bool {
	for _, line := range strings.Split(commentText, "\n") {
		fields := FieldsByAnySpace(strings.TrimSpace(line), 2)
		if len(fields) > 0 && strings.ToLower(fields[0]) == routerAttr {
			return true
		}
	}
	return false
}

func isGeneralAPIComment(comment string) bool {
	// for _, commentLine := range comments {
	commentLine := strings.TrimSpace(comment)
	if len(commentLine) == 0 {
		return false
	}

	attribute := strings.ToLower(FieldsByAnySpace(commentLine, 2)[0])
	switch attribute {
	// The @summary, @router, @success, @failure annotation belongs to Operation
	case summaryAttr, routerAttr, successAttr, failureAttr, responseAttr:
		return false
	}
	// }

	return true
}

func getMarkdownForTag(tagName string, dirPath string) ([]byte, error) {
	if tagName == "" {
		// this happens when parsing the @description.markdown attribute
		// it will be called properly another time with tagName="api"
		// so we can safely return an empty byte slice here
		return make([]byte, 0), nil
	}

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		expectedFileName := tagName
		if !strings.HasSuffix(tagName, ".md") {
			expectedFileName = tagName + ".md"
		}

		if fileName == expectedFileName {
			fullPath := filepath.Join(dirPath, fileName)

			commentInfo, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, fmt.Errorf("Failed to read markdown file %s error: %s ", fullPath, err)
			}

			return commentInfo, nil
		}
	}

	return nil, fmt.Errorf("Unable to find markdown file for tag %s in the given directory", tagName)
}

func isExistsScope(scope string) (bool, error) {
	s := strings.Fields(scope)
	for _, v := range s {
		if strings.HasPrefix(v, scopeAttrPrefix) {
			if strings.Contains(v, ",") {
				return false, fmt.Errorf("@scope can't use comma(,) get=" + v)
			}
		}
	}

	return strings.HasPrefix(scope, scopeAttrPrefix), nil
}

func getTagsFromComment(comment string) (tags []string) {
	commentLine := strings.TrimSpace(strings.TrimLeft(comment, "/"))
	if len(commentLine) == 0 {
		return nil
	}

	attribute := strings.Fields(commentLine)[0]
	lineRemainder, lowerAttribute := strings.TrimSpace(commentLine[len(attribute):]), strings.ToLower(attribute)

	if lowerAttribute == tagsAttr {
		for _, tag := range strings.Split(lineRemainder, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}
	return

}

func (parser *Parser) matchTag(tag string) bool {
	if len(parser.tags) == 0 {
		return true
	}

	if _, has := parser.tags["!"+tag]; has {
		return false
	}
	if _, has := parser.tags[tag]; has {
		return true
	}

	// If all tags are negation then we should return true
	for key := range parser.tags {
		if key[0] != '!' {
			return false
		}
	}
	return true
}

func (parser *Parser) matchTags(comments []*ast.Comment) (match bool) {
	if len(parser.tags) == 0 {
		return true
	}

	match = false
	for _, comment := range comments {
		for _, tag := range getTagsFromComment(comment.Text) {
			if _, has := parser.tags["!"+tag]; has {
				return false
			}
			if _, has := parser.tags[tag]; has {
				match = true // keep iterating as it may contain a tag that is excluded
			}
		}
	}

	if !match {
		// If all tags are negation then we should return true
		for key := range parser.tags {
			if key[0] != '!' {
				return false
			}
		}
	}

	return true
}

func matchExtension(extensionToMatch string, comments []*ast.Comment) (match bool) {
	if len(extensionToMatch) == 0 {
		return true
	}

	for _, comment := range comments {
		commentLine := strings.TrimSpace(strings.TrimLeft(comment.Text, "/"))
		fields := FieldsByAnySpace(commentLine, 2)
		if len(fields) > 0 {
			lowerAttribute := strings.ToLower(fields[0])

			if lowerAttribute == fmt.Sprintf("@x-%s", strings.ToLower(extensionToMatch)) {
				return true
			}
		}
	}

	return false
}

// ParseRouterAPIInfo parses router api info for given astFile.

func convertFromSpecificToPrimitive(typeName string) (string, error) {
	name := typeName
	if strings.ContainsRune(name, '.') {
		name = strings.Split(name, ".")[1]
	}

	switch strings.ToUpper(name) {
	case "TIME", "OBJECTID", "UUID":
		return STRING, nil
	case "DECIMAL":
		return NUMBER, nil
	}

	return typeName, ErrFailedConvertPrimitiveType
}

func (parser *Parser) isInStructStack(typeSpecDef *TypeSpecDef) bool {
	for _, specDef := range parser.structStack {
		if typeSpecDef == specDef {
			return true
		}
	}

	return false
}

// ParseDefinition parses given type spec that corresponds to the type under
// given name and package, and populates swagger schema definitions registry
// with a schema for the given type

func fullTypeName(parts ...string) string {
	return strings.Join(parts, ".")
}

// TODO: If .go file contains many types, it may work for a long time

// extractDeclarationDescription gets first description
// from attribute descriptionAttr in commentGroups (ast.CommentGroup)
func (parser *Parser) extractDeclarationDescription(typeName string, commentGroups ...*ast.CommentGroup) (string, error) {
	var description string

	for _, commentGroup := range commentGroups {
		if commentGroup == nil {
			continue
		}

		isHandlingDescription := false

		for _, comment := range commentGroup.List {
			commentText := strings.TrimSpace(strings.TrimLeft(comment.Text, "/"))
			if len(commentText) == 0 {
				continue
			}
			fields := FieldsByAnySpace(commentText, 2)
			attribute := fields[0]

			if attr := strings.ToLower(attribute); attr == descriptionMarkdownAttr {
				if len(fields) > 1 {
					typeName = fields[1]
				}
				if typeName == "" {
					continue
				}
				desc, err := getMarkdownForTag(typeName, parser.markdownFileDir)
				if err != nil {
					return "", err
				}
				// if found markdown description, we will only use the markdown file content
				return string(desc), nil
			} else if attr != descriptionAttr {
				if !isHandlingDescription {
					continue
				}

				break
			}

			isHandlingDescription = true
			description += " " + strings.TrimSpace(commentText[len(attribute):])
		}
	}

	return strings.TrimLeft(description, " "), nil
}

// parseTypeExpr parses given type expression that corresponds to the type under
// given name and package, and returns swagger schema for it.

func getFieldType(file *ast.File, field ast.Expr, genericParamTypeDefs map[string]*genericTypeSpec) (string, error) {
	switch fieldType := field.(type) {
	case *ast.Ident:
		return fieldType.Name, nil
	case *ast.SelectorExpr:
		packageName, err := getFieldType(file, fieldType.X, genericParamTypeDefs)
		if err != nil {
			return "", err
		}

		return fullTypeName(packageName, fieldType.Sel.Name), nil
	case *ast.StarExpr:
		fullName, err := getFieldType(file, fieldType.X, genericParamTypeDefs)
		if err != nil {
			return "", err
		}

		return fullName, nil
	default:
		return getGenericFieldType(file, field, genericParamTypeDefs)
	}
}

// GetSchemaTypePath get path of schema type.

// defineTypeOfExample example value define the type (object and array unsupported).
func defineTypeOfExample(schemaType, arrayType, exampleValue string) (interface{}, error) {
	switch schemaType {
	case STRING:
		return exampleValue, nil
	case NUMBER:
		v, err := strconv.ParseFloat(exampleValue, 64)
		if err != nil {
			return nil, fmt.Errorf("example value %s can't convert to %s err: %s", exampleValue, schemaType, err)
		}

		return v, nil
	case INTEGER:
		v, err := strconv.Atoi(exampleValue)
		if err != nil {
			return nil, fmt.Errorf("example value %s can't convert to %s err: %s", exampleValue, schemaType, err)
		}

		return v, nil
	case BOOLEAN:
		v, err := strconv.ParseBool(exampleValue)
		if err != nil {
			return nil, fmt.Errorf("example value %s can't convert to %s err: %s", exampleValue, schemaType, err)
		}

		return v, nil
	case ARRAY:
		values := strings.Split(exampleValue, ",")
		result := make([]interface{}, 0)
		for _, value := range values {
			v, err := defineTypeOfExample(arrayType, "", value)
			if err != nil {
				return nil, err
			}

			result = append(result, v)
		}

		return result, nil
	case OBJECT:
		if arrayType == "" {
			return nil, fmt.Errorf("%s is unsupported type in example value `%s`", schemaType, exampleValue)
		}

		values := strings.Split(exampleValue, ",")

		result := map[string]interface{}{}

		for _, value := range values {
			mapData := strings.SplitN(value, ":", 2)

			if len(mapData) == 2 {
				v, err := defineTypeOfExample(arrayType, "", mapData[1])
				if err != nil {
					return nil, err
				}

				result[mapData[0]] = v

				continue
			}

			return nil, fmt.Errorf("example value %s should format: key:value", exampleValue)
		}

		return result, nil
	case ANY:
		return exampleValue, nil
	}

	return nil, fmt.Errorf("%s is unsupported type in example value %s", schemaType, exampleValue)
}

// GetAllGoFileInfo gets all Go source files information for given searchDir.
func (parser *Parser) getAllGoFileInfo(packageDir, searchDir string) error {
	if parser.skipPackageByPrefix(packageDir) {
		return nil // ignored by user-defined package path prefixes
	}
	return filepath.Walk(searchDir, func(path string, f os.FileInfo, _ error) error {
		err := parser.Skip(path, f)
		if err != nil {
			return err
		}

		if f.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(searchDir, path)
		if err != nil {
			return err
		}

		return parser.parseFile(filepath.ToSlash(filepath.Dir(filepath.Clean(filepath.Join(packageDir, relPath)))), path, nil, ParseAll)
	})
}

func (parser *Parser) getAllGoFileInfoFromDeps(pkg *depth.Pkg, parseFlag ParseFlag) error {
	ignoreInternal := pkg.Internal && !parser.ParseInternal
	if ignoreInternal || !pkg.Resolved { // ignored internal and not resolved dependencies
		return nil
	}

	if pkg.Raw != nil && parser.skipPackageByPrefix(pkg.Raw.ImportPath) {
		return nil // ignored by user-defined package path prefixes
	}

	// Skip cgo
	if pkg.Raw == nil && pkg.Name == "C" {
		return nil
	}

	srcDir := pkg.Raw.Dir

	files, err := os.ReadDir(srcDir) // only parsing files in the dir(don't contain sub dir files)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		path := filepath.Join(srcDir, f.Name())
		if err := parser.parseFile(pkg.Name, path, nil, parseFlag); err != nil {
			return err
		}
	}

	for i := 0; i < len(pkg.Deps); i++ {
		if err := parser.getAllGoFileInfoFromDeps(&pkg.Deps[i], parseFlag); err != nil {
			return err
		}
	}

	return nil
}

func (parser *Parser) parseFile(packageDir, path string, src interface{}, flag ParseFlag) error {
	if strings.HasSuffix(strings.ToLower(path), "_test.go") || filepath.Ext(path) != ".go" {
		return nil
	}

	return parser.packages.ParseFile(packageDir, path, src, flag)
}

// Skip returns filepath.SkipDir error if match vendor and hidden folder.
func (parser *Parser) Skip(path string, f os.FileInfo) error {
	return walkWith(parser.excludes, parser.ParseVendor)(path, f)
}

func walkWith(excludes map[string]struct{}, parseVendor bool) func(path string, fileInfo os.FileInfo) error {
	return func(path string, f os.FileInfo) error {
		if f.IsDir() {
			if !parseVendor && f.Name() == "vendor" || // ignore "vendor"
				f.Name() == "docs" || // exclude docs
				len(f.Name()) > 1 && f.Name()[0] == '.' && f.Name() != ".." { // exclude all hidden folder
				return filepath.SkipDir
			}

			if excludes != nil {
				if _, ok := excludes[path]; ok {
					return filepath.SkipDir
				}
			}
		}

		return nil
	}
}

// addTestType just for tests.
func (parser *Parser) addTestType(typename string) {
	typeDef := &TypeSpecDef{}
	parser.packages.uniqueDefinitions[typename] = typeDef
	parser.parsedSchemas[typeDef] = &Schema{
		PkgPath: "",
		Name:    typename,
		Schema:  PrimitiveSchema(OBJECT).Spec,
	}
}
