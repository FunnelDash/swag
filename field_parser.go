package swag

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

const (
	requiredLabel    = "required"
	optionalLabel    = "optional"
	omitEmptyLabel   = "omitempty"
	omitZeroLabel    = "omitzero"
	swaggerTypeTag   = "swaggertype"
	swaggerIgnoreTag = "swaggerignore"
)

func toSnakeCase(in string) string {
	var (
		runes  = []rune(in)
		length = len(runes)
		out    []rune
	)

	for idx := 0; idx < length; idx++ {
		if idx > 0 && unicode.IsUpper(runes[idx]) &&
			((idx+1 < length && unicode.IsLower(runes[idx+1])) || unicode.IsLower(runes[idx-1])) {
			out = append(out, '_')
		}

		out = append(out, unicode.ToLower(runes[idx]))
	}

	return string(out)
}

func toLowerCamelCase(in string) string {
	var flag bool

	out := make([]rune, len(in))

	runes := []rune(in)
	for i, curr := range runes {
		if (i == 0 && unicode.IsUpper(curr)) || (flag && unicode.IsUpper(curr)) {
			out[i] = unicode.ToLower(curr)
			flag = true

			continue
		}

		out[i] = curr
		flag = false
	}

	return string(out)
}

// splitNotWrapped slices s into all substrings separated by sep if sep is not
// wrapped by brackets and returns a slice of the substrings between those separators.
func splitNotWrapped(s string, sep rune) []string {
	openCloseMap := map[rune]rune{
		'(': ')',
		'[': ']',
		'{': '}',
	}

	var (
		result    = make([]string, 0)
		current   = strings.Builder{}
		openCount = 0
		openChar  rune
	)

	for _, char := range s {
		switch {
		case openChar == 0 && openCloseMap[char] != 0:
			openChar = char

			openCount++

			current.WriteRune(char)
		case char == openChar:
			openCount++

			current.WriteRune(char)
		case openCount > 0 && char == openCloseMap[openChar]:
			openCount--

			current.WriteRune(char)
		case openCount == 0 && char == sep:
			result = append(result, current.String())

			openChar = 0

			current = strings.Builder{}
		default:
			current.WriteRune(char)
		}
	}

	if current.String() != "" {
		result = append(result, current.String())
	}

	return result
}

// ComplementSchema complement schema with field properties

// complementSchema complement schema with field properties

const (
	utf8HexComma = "0x2C"
	utf8Pipe     = "0x7C"
)

// These code copy from
// https://github.com/go-playground/validator/blob/d4271985b44b735c6f76abc7a06532ee997f9476/baked_in.go#L207
// ---.
var oneofValsCache = map[string][]string{}
var oneofValsCacheRWLock = sync.RWMutex{}
var splitParamsRegex = regexp.MustCompile(`'[^']*'|\S+`)

func parseOneOfParam2(param string) []string {
	oneofValsCacheRWLock.RLock()
	values, ok := oneofValsCache[param]
	oneofValsCacheRWLock.RUnlock()

	if !ok {
		oneofValsCacheRWLock.Lock()
		values = splitParamsRegex.FindAllString(param, -1)

		for i := 0; i < len(values); i++ {
			values[i] = strings.ReplaceAll(values[i], "'", "")
		}

		oneofValsCache[param] = values

		oneofValsCacheRWLock.Unlock()
	}

	return values
}

// ---.
