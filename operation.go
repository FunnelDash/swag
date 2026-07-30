package swag

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// RouteProperties describes HTTP properties of a single router comment.
type RouteProperties struct {
	HTTPMethod string
	Path       string
	Deprecated bool
}

// Operation describes a single API operation on a path.
// For more information: https://github.com/swaggo/swag#api-operation

var mimeTypeAliases = map[string]string{
	"json":                  "application/json",
	"xml":                   "text/xml",
	"plain":                 "text/plain",
	"html":                  "text/html",
	"mpfd":                  "multipart/form-data",
	"x-www-form-urlencoded": "application/x-www-form-urlencoded",
	"json-api":              "application/vnd.api+json",
	"json-stream":           "application/x-json-stream",
	"octet-stream":          "application/octet-stream",
	"png":                   "image/png",
	"jpeg":                  "image/jpeg",
	"gif":                   "image/gif",
}

var mimeTypePattern = regexp.MustCompile("^[^/]+/[^/]+$")

// NewOperation creates a new Operation with default properties.
// map[int]Response.

// SetCodeExampleFilesDirectory sets the directory to search for codeExamples.

// ParseComment parses comment for given comment string and returns error if error occurs.

// ParseCodeSample parse code sample.

// ParseStateComment parse state comment.

// ParseDescriptionComment parse description comment.

// ParseMetadata parse metadata.

var paramPattern = regexp.MustCompile(`(\S+)\s+(\w+)\s+([\S. ]+?)\s+(\w+)\s+"([^"]+)"`)

func findInSlice(arr []string, target string) bool {
	for _, str := range arr {
		if str == target {
			return true
		}
	}

	return false
}

// ParseParamComment parses params return []string of param properties
// E.g. @Param	queryText		formData	      string	  true		        "The email for login"
//
//	[param name]    [paramType] [data type]  [is mandatory?]   [Comment]
//
// E.g. @Param   some_id     path    int     true        "Some ID".

const (
	formTag             = "form"
	jsonTag             = "json"
	uriTag              = "uri"
	headerTag           = "header"
	bindingTag          = "binding"
	defaultTag          = "default"
	enumsTag            = "enums"
	exampleTag          = "example"
	schemaExampleTag    = "schemaExample"
	formatTag           = "format"
	titleTag            = "title"
	validateTag         = "validate"
	minimumTag          = "minimum"
	maximumTag          = "maximum"
	minLengthTag        = "minLength"
	maxLengthTag        = "maxLength"
	multipleOfTag       = "multipleOf"
	readOnlyTag         = "readonly"
	extensionsTag       = "extensions"
	collectionFormatTag = "collectionFormat"
	patternTag          = "pattern"
	oneOfTag            = "oneOf"
)

var regexAttributes = map[string]*regexp.Regexp{
	// for Enums(A, B)
	enumsTag: regexp.MustCompile(`(?i)\s+enums\(.*\)`),
	// for maximum(0)
	maximumTag: regexp.MustCompile(`(?i)\s+maxinum|maximum\(.*\)`),
	// for minimum(0)
	minimumTag: regexp.MustCompile(`(?i)\s+mininum|minimum\(.*\)`),
	// for default(0)
	defaultTag: regexp.MustCompile(`(?i)\s+default\(.*\)`),
	// for minlength(0)
	minLengthTag: regexp.MustCompile(`(?i)\s+minlength\(.*\)`),
	// for maxlength(0)
	maxLengthTag: regexp.MustCompile(`(?i)\s+maxlength\(.*\)`),
	// for format(email)
	formatTag: regexp.MustCompile(`(?i)\s+format\(.*\)`),
	// for extensions(x-example=test)
	extensionsTag: regexp.MustCompile(`(?i)\s+extensions\(.*\)`),
	// for collectionFormat(csv)
	collectionFormatTag: regexp.MustCompile(`(?i)\s+collectionFormat\(.*\)`),
	// example(0)
	exampleTag: regexp.MustCompile(`(?i)\s+example\(.*\)`),
	// schemaExample(0)
	schemaExampleTag: regexp.MustCompile(`(?i)\s+schemaExample\(.*\)`),
}

func findAttr(re *regexp.Regexp, commentLine string) (string, error) {
	attr := re.FindString(commentLine)

	l, r := strings.Index(attr, "("), strings.Index(attr, ")")
	if l == -1 || r == -1 {
		return "", fmt.Errorf("can not find regex=%s, comment=%s", re.String(), commentLine)
	}

	return strings.TrimSpace(attr[l+1 : r]), nil
}

func setExtensionParam(attr string) map[string]interface{} {
	extensions := map[string]interface{}{}

	for _, val := range splitNotWrapped(attr, ',') {
		parts := strings.SplitN(val, "=", 2)
		if len(parts) == 2 {
			extensions[strings.ToLower(parts[0])] = parts[1]

			continue
		}

		if len(parts[0]) > 0 && string(parts[0][0]) == "!" {
			extensions[strings.ToLower(parts[0][1:])] = false

			continue
		}

		extensions[strings.ToLower(parts[0])] = true
	}

	return extensions
}

// defineType enum value define the type (object and array unsupported).
func defineType(schemaType string, value string) (v interface{}, err error) {
	schemaType = TransToValidSchemeType(schemaType)

	switch schemaType {
	case STRING:
		return value, nil
	case NUMBER:
		v, err = strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("enum value %s can't convert to %s err: %s", value, schemaType, err)
		}
	case INTEGER:
		v, err = strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("enum value %s can't convert to %s err: %s", value, schemaType, err)
		}
	case BOOLEAN:
		v, err = strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("enum value %s can't convert to %s err: %s", value, schemaType, err)
		}
	default:
		return nil, fmt.Errorf("%s is unsupported type in enum value %s", schemaType, value)
	}

	return v, nil
}

// ParseTagsComment parses comment for given `tag` comment string.

// ParseAcceptComment parses comment for given `accept` comment string.

// ParseProduceComment parses comment for given `produce` comment string.

// parseMimeTypeList parses a list of MIME Types for a comment like
// `produce` (`Content-Type:` response header) or
// `accept` (`Accept:` request header).

var routerPattern = regexp.MustCompile(`^(/[\w./\-{}\(\)+:$]*)[[:blank:]]+\[(\w+)]`)

// ParseRouterComment parses comment for given `router` comment string.

// ParseSecurityComment parses comment for given `security` comment string.

// findTypeDef attempts to find the *ast.TypeSpec for a specific type given the
// type's name and the package's import path.
// TODO: improve finding external pkg.

var responsePattern = regexp.MustCompile(`^([\w,]+)\s+([\w{}]+)\s+([\w\-.\\{}=,\[\s\]]+)\s*(".*)?`)

// ResponseType{data1=Type1,data2=Type2}.
var combinedPattern = regexp.MustCompile(`^([\w\-./\[\]]+){(.*)}$`)

func parseFields(s string) []string {
	nestLevel := 0

	return strings.FieldsFunc(s, func(char rune) bool {
		if char == '{' {
			nestLevel++

			return false
		} else if char == '}' {
			nestLevel--

			return false
		}

		return char == ',' && nestLevel == 0
	})
}

// ParseResponseComment parses comment for given `response` comment string.

// ParseResponseHeaderComment parses comment for given `response header` comment string.

var emptyResponsePattern = regexp.MustCompile(`([\w,]+)\s+"(.*)"`)

// ParseEmptyResponseComment parse only comment out status code and description,eg: @Success 200 "it's ok".

// ParseEmptyResponseOnly parse only comment out status code ,eg: @Success 200.

// DefaultResponse return the default response member pointer.

// AddResponse add a response for a code.

// createParameter returns swagger spec.Parameter for given  paramType, description, paramName, schemaType, required.

func getCodeExampleForSummary(summaryName string, dirPath string) ([]byte, bool, error) {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, false, err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		isJSON := strings.Contains(fileName, ".json")
		isYaml := strings.Contains(fileName, ".yaml")
		if !isJSON && !isYaml {
			continue
		}

		if strings.Contains(fileName, summaryName) {
			fullPath := filepath.Join(dirPath, fileName)

			commentInfo, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, false, fmt.Errorf("Failed to read code example file %s error: %s ", fullPath, err)
			}

			return commentInfo, isJSON, nil
		}
	}

	return nil, false, fmt.Errorf("unable to find code example file for tag %s in the given directory", summaryName)
}
