package swag

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sv-tools/openapi/spec"
)

var typeObject = spec.SingleOrArray[string](spec.SingleOrArray[string]{OBJECT})
var typeArray = spec.SingleOrArray[string](spec.SingleOrArray[string]{ARRAY})
var typeInteger = spec.SingleOrArray[string](spec.SingleOrArray[string]{INTEGER})
var typeString = spec.SingleOrArray[string](spec.SingleOrArray[string]{STRING})
var typeFile = spec.SingleOrArray[string](spec.SingleOrArray[string]{"file"})
var typeNumber = spec.SingleOrArray[string](spec.SingleOrArray[string]{NUMBER})
var typeBool = spec.SingleOrArray[string](spec.SingleOrArray[string]{BOOLEAN})

func TestParseEmptyComment(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)
	err := operation.ParseComment("//", nil)

	require.NoError(t, err)
}

func TestParseTagsComment(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)
	err := operation.ParseComment(`/@Tags pet, store,user`, nil)
	require.NoError(t, err)
	assert.Equal(t, operation.Tags, []string{"pet", "store", "user"})
}

func TestParseRouterComment(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{wishlist_id} [get]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	assert.Len(t, operation.RouterProperties, 1)
	assert.Equal(t, "/customer/get-wishlist/{wishlist_id}", operation.RouterProperties[0].Path)
	assert.Equal(t, "GET", operation.RouterProperties[0].HTTPMethod)
}

func TestParseRouterMultipleComments(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{wishlist_id} [get]`
	anotherComment := `/@Router /customer/get-the-wishlist/{wishlist_id} [post]`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	err = operation.ParseComment(anotherComment, nil)
	require.NoError(t, err)

	assert.Len(t, operation.RouterProperties, 2)
	assert.Equal(t, "/customer/get-wishlist/{wishlist_id}", operation.RouterProperties[0].Path)
	assert.Equal(t, "GET", operation.RouterProperties[0].HTTPMethod)
	assert.Equal(t, "/customer/get-the-wishlist/{wishlist_id}", operation.RouterProperties[1].Path)
	assert.Equal(t, "POST", operation.RouterProperties[1].HTTPMethod)
}

func TestParseRouterOnlySlash(t *testing.T) {
	t.Parallel()

	comment := `// @Router / [get]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)
	assert.Len(t, operation.RouterProperties, 1)
	assert.Equal(t, "/", operation.RouterProperties[0].Path)
	assert.Equal(t, "GET", operation.RouterProperties[0].HTTPMethod)
}

func TestParseRouterCommentWithPlusSign(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{proxy+} [post]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)
	assert.Len(t, operation.RouterProperties, 1)
	assert.Equal(t, "/customer/get-wishlist/{proxy+}", operation.RouterProperties[0].Path)
	assert.Equal(t, "POST", operation.RouterProperties[0].HTTPMethod)
}

func TestParseRouterCommentWithDollarSign(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{wishlist_id}$move [post]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)
	assert.Len(t, operation.RouterProperties, 1)
	assert.Equal(t, "/customer/get-wishlist/{wishlist_id}$move", operation.RouterProperties[0].Path)
	assert.Equal(t, "POST", operation.RouterProperties[0].HTTPMethod)
}

func TestParseRouterCommentNoDollarSignAtPathStartErr(t *testing.T) {
	t.Parallel()

	comment := `/@Router $customer/get-wishlist/{wishlist_id}$move [post]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseRouterCommentWithColonSign(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{wishlist_id}:move [post]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)
	assert.Len(t, operation.RouterProperties, 1)
	assert.Equal(t, "/customer/get-wishlist/{wishlist_id}:move", operation.RouterProperties[0].Path)
	assert.Equal(t, "POST", operation.RouterProperties[0].HTTPMethod)
}

func TestParseRouterCommentNoColonSignAtPathStartErr(t *testing.T) {
	t.Parallel()

	comment := `/@Router :customer/get-wishlist/{wishlist_id}:move [post]`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseRouterCommentMethodSeparationErr(t *testing.T) {
	t.Parallel()

	comment := `/@Router /api/{id}|,*[get`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseRouterCommentMethodMissingErr(t *testing.T) {
	t.Parallel()

	comment := `/@Router /customer/get-wishlist/{wishlist_id}`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestOperation_ParseResponseWithDefault(t *testing.T) {
	t.Parallel()

	comment := `@Success default {object} nil "An empty response"`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	assert.Equal(t, "An empty response", operation.Responses.Spec.Default.Spec.Spec.Description)

	comment = `@Success 200,default {string} Response "A response"`
	operation = NewOperation(nil)

	err = operation.ParseComment(comment, nil)
	require.NoError(t, err)

	assert.Equal(t, "A response", operation.Responses.Spec.Default.Spec.Spec.Description)
	assert.Equal(t, "A response", operation.Responses.Spec.Response["200"].Spec.Spec.Description)
}

func TestParseResponseSuccessCommentWithEmptyResponse(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} nil "An empty response"`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `An empty response`, response.Spec.Spec.Description)
}

func TestParseResponseFailureCommentWithEmptyResponse(t *testing.T) {
	t.Parallel()

	comment := `@Failure 500 {object} nil`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	assert.Equal(t, "Internal Server Error", operation.Responses.Spec.Response["500"].Spec.Spec.Description)
}

func TestParseResponseCommentWithObjectType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.OrderRow "Error message, if code != 200`
	parser := New()
	operation := NewOperation(parser)
	operation.parser.addTestType("model.OrderRow")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)

	assert.Equal(t, "#/components/schemas/model.OrderRow", response.Spec.Spec.Content["application/json"].Spec.Schema.Ref.Ref)
}

func TestParseResponseCommentWithNestedPrimitiveType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data=string,data2=int} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)
	require.NotNil(t, response.Spec.Spec.Content["application/json"].Spec.Schema)

	allOf := operation.Responses.Spec.Response["200"].Spec.Spec.Content["application/json"].Spec.Schema.Spec.AllOf
	require.NotNil(t, allOf)
	assert.Equal(t, 2, len(allOf))
	found := map[string]struct{}{}
	for _, schema := range allOf {
		assert.NotNil(t, schema.Ref.Ref)
		found[schema.Ref.Ref] = struct{}{}
	}
	assert.NotNil(t, found["#/components/schemas/data"])
	assert.NotNil(t, found["#/components/schemas/data2"])
}

func TestParseResponseCommentWithNestedPrimitiveArrayType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data=[]string,data2=[]int} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)
	assert.NotNil(t, operation.parser.openAPI.Components.Spec.Schemas["data"].Spec.Properties["data"])
	assert.Equal(t, &typeString, operation.parser.openAPI.Components.Spec.Schemas["data"].Spec.Properties["data"].Spec.Items.Schema.Spec.Type)
}

func TestParseResponseCommentWithNestedObjectType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data=model.Payload,data2=model.Payload2} "Error message, if code != 200`
	operation := NewOperation(New())
	operation.parser.addTestType("model.CommonHeader")
	operation.parser.addTestType("model.Payload")
	operation.parser.addTestType("model.Payload2")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)
	assert.Equal(t, 2, len(response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.AllOf))
	assert.Equal(t, 5, len(operation.parser.openAPI.Components.Spec.Schemas))

	assert.Equal(t, "#/components/schemas/model.Payload", operation.parser.openAPI.Components.Spec.Schemas["data"].Spec.Properties["data"].Ref.Ref)
	assert.Equal(t, "#/components/schemas/model.Payload2", operation.parser.openAPI.Components.Spec.Schemas["data2"].Spec.Properties["data2"].Ref.Ref)
}

func TestParseResponseCommentWithNestedArrayObjectType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data=[]model.Payload,data2=[]model.Payload2} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")
	operation.parser.addTestType("model.Payload")
	operation.parser.addTestType("model.Payload2")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)

	allOf := response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.AllOf
	assert.Equal(t, 2, len(allOf))

	assert.Equal(t, "#/components/schemas/model.Payload", operation.parser.openAPI.Components.Spec.Schemas["data"].Spec.Properties["data"].Spec.Items.Schema.Ref.Ref)
	assert.Equal(t, &typeArray, operation.parser.openAPI.Components.Spec.Schemas["data"].Spec.Properties["data"].Spec.Type)

	assert.Equal(t, "#/components/schemas/model.Payload2", operation.parser.openAPI.Components.Spec.Schemas["data2"].Spec.Properties["data2"].Spec.Items.Schema.Ref.Ref)
	assert.Equal(t, &typeArray, operation.parser.openAPI.Components.Spec.Schemas["data2"].Spec.Properties["data2"].Spec.Type)
}

func TestParseResponseCommentWithNestedFields(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data1=int,data2=[]int,data3=model.Payload,data4=[]model.Payload} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")
	operation.parser.addTestType("model.Payload")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)

	allOf := response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.AllOf
	assert.Equal(t, 4, len(allOf))

	schemas := operation.parser.openAPI.Components.Spec.Schemas

	assert.Equal(t, &typeInteger, schemas["data1"].Spec.Properties["data1"].Spec.Type)
	assert.Equal(t, &typeObject, schemas["data1"].Spec.Type)

	assert.Equal(t, &typeArray, schemas["data2"].Spec.Properties["data2"].Spec.Type)
	assert.Equal(t, &typeInteger, schemas["data2"].Spec.Properties["data2"].Spec.Items.Schema.Spec.Type)
	assert.Equal(t, &typeObject, schemas["data2"].Spec.Type)

	assert.Equal(t, "#/components/schemas/model.Payload", schemas["data3"].Spec.Properties["data3"].Ref.Ref)
	assert.Equal(t, &typeObject, schemas["data3"].Spec.Type)

	assert.Equal(t, "#/components/schemas/model.Payload", schemas["data4"].Spec.Properties["data4"].Spec.Items.Schema.Ref.Ref)
	assert.Equal(t, &typeArray, schemas["data4"].Spec.Properties["data4"].Spec.Type)
	assert.Equal(t, &typeObject, schemas["data4"].Spec.Type)
}

func TestParseResponseCommentWithDeepNestedFields(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.CommonHeader{data1=int,data2=[]int,data3=model.Payload{data1=int,data2=model.DeepPayload},data4=[]model.Payload{data1=[]int,data2=[]model.DeepPayload}} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")
	operation.parser.addTestType("model.Payload")
	operation.parser.addTestType("model.DeepPayload")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)

	allOf := response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.AllOf
	assert.Equal(t, 4, len(allOf))

	schemas := operation.parser.openAPI.Components.Spec.Schemas

	assert.Equal(t, &typeInteger, schemas["data1"].Spec.Properties["data1"].Spec.Type)
	assert.Equal(t, &typeObject, schemas["data1"].Spec.Type)

	assert.Equal(t, &typeArray, schemas["data2"].Spec.Properties["data2"].Spec.Type)
	assert.Equal(t, &typeInteger, schemas["data2"].Spec.Properties["data2"].Spec.Items.Schema.Spec.Type)
	assert.Equal(t, &typeObject, schemas["data2"].Spec.Type)

	assert.Equal(t, &typeObject, schemas["data3"].Spec.Type)
	assert.Equal(t, &typeObject, schemas["data3"].Spec.Properties["data3"].Spec.Type)
	assert.Equal(t, 2, len(schemas["data3"].Spec.Properties["data3"].Spec.AllOf))

	assert.Equal(t, &typeObject, schemas["data4"].Spec.Type)
	assert.Equal(t, &typeArray, schemas["data4"].Spec.Properties["data4"].Spec.Type)
	assert.Equal(t, &typeObject, schemas["data4"].Spec.Properties["data4"].Spec.Items.Schema.Spec.Type)
	assert.Equal(t, 2, len(schemas["data4"].Spec.Properties["data4"].Spec.Items.Schema.Spec.AllOf))
}

func TestParseResponseCommentWithNestedArrayMapFields(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} []map[string]model.CommonHeader{data1=[]map[string]model.Payload,data2=map[string][]int} "Error message, if code != 200`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")
	operation.parser.addTestType("model.Payload")

	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)

	content := response.Spec.Spec.Content["application/json"]
	assert.NotNil(t, content)
	assert.NotNil(t, content.Spec)
	assert.NotNil(t, content.Spec.Schema.Spec.Items.Schema.Spec.AdditionalProperties.Schema)

	assert.Equal(t, 2, len(content.Spec.Schema.Spec.Items.Schema.Spec.AdditionalProperties.Schema.Spec.AllOf))
	assert.Equal(t, &typeArray, content.Spec.Schema.Spec.Type)
	assert.Equal(t, &typeObject, content.Spec.Schema.Spec.Items.Schema.Spec.Type)
	assert.Equal(t, &typeObject, content.Spec.Schema.Spec.Items.Schema.Spec.AdditionalProperties.Schema.Spec.Type)

	schemas := operation.parser.openAPI.Components.Spec.Schemas

	data1 := schemas["data1"]
	assert.NotNil(t, data1)
	assert.NotNil(t, data1.Spec)
	assert.NotNil(t, data1.Spec.Properties)

	assert.Equal(t, &typeObject, data1.Spec.Type)
	assert.Equal(t, &typeArray, data1.Spec.Properties["data1"].Spec.Type)
	assert.Equal(t, &typeObject, data1.Spec.Properties["data1"].Spec.Items.Schema.Spec.Type)
	assert.Equal(t, "#/components/schemas/model.Payload", data1.Spec.Properties["data1"].Spec.Items.Schema.Spec.AdditionalProperties.Schema.Ref.Ref)

	data2 := schemas["data2"]
	assert.NotNil(t, data2)
	assert.NotNil(t, data2.Spec)
	assert.NotNil(t, data2.Spec.Properties)

	assert.Equal(t, &typeObject, data2.Spec.Type)
	assert.Equal(t, &typeObject, data2.Spec.Properties["data2"].Spec.Type)
	assert.Equal(t, &typeArray, data2.Spec.Properties["data2"].Spec.AdditionalProperties.Schema.Spec.Type)
	assert.Equal(t, &typeInteger, data2.Spec.Properties["data2"].Spec.AdditionalProperties.Schema.Spec.Items.Schema.Spec.Type)

	commonHeader := schemas["model.CommonHeader"]
	assert.NotNil(t, commonHeader)
	assert.NotNil(t, commonHeader.Spec)
	assert.Equal(t, 2, len(commonHeader.Spec.AllOf))
	assert.Equal(t, &typeObject, commonHeader.Spec.Type)

	payload := schemas["model.Payload"]
	assert.NotNil(t, payload)
	assert.NotNil(t, payload.Spec)
	assert.Equal(t, &typeObject, payload.Spec.Type)
}

func TestParseResponseCommentWithObjectTypeInSameFile(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} testOwner "Error message, if code != 200"`
	operation := NewOperation(New())

	operation.parser.addTestType("swag.testOwner")

	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operation_test.go", `package swag
	type testOwner struct {

	}
	`, goparser.ParseComments)
	assert.NoError(t, err)

	err = operation.ParseComment(comment, astFile)
	assert.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)
	assert.Equal(t, "#/components/schemas/swag.testOwner", response.Spec.Spec.Content["application/json"].Spec.Schema.Ref.Ref)
}

func TestParseResponseCommentWithObjectTypeErr(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {object} model.OrderRow "Error message, if code != 200"`
	operation := NewOperation(New())

	operation.parser.addTestType("model.notexist")

	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseResponseCommentWithArrayType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {array} model.OrderRow "Error message, if code != 200`
	operation := NewOperation(New())
	operation.parser.addTestType("model.OrderRow")

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	response := operation.Responses.Spec.Response["200"]
	assert.Equal(t, `Error message, if code != 200`, response.Spec.Spec.Description)
	assert.Equal(t, &typeArray, response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)
	assert.Equal(t, "#/components/schemas/model.OrderRow", response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Items.Schema.Ref.Ref)

}

func TestParseResponseCommentWithBasicType(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 {string} string "it's ok'"`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok'", response.Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)
}

func TestParseResponseCommentWithBasicTypeAndCodes(t *testing.T) {
	t.Parallel()

	comment := `@Success 200,201,default {string} string "it's ok"`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Response["201"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Default
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)
}

func TestParseEmptyResponseComment(t *testing.T) {
	t.Parallel()

	comment := `@Success 200 "it is ok"`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it is ok", response.Spec.Spec.Description)
}

func TestParseEmptyResponseCommentWithCodes(t *testing.T) {
	t.Parallel()

	comment := `@Success 200,201,default "it is ok"`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it is ok", response.Spec.Spec.Description)

	response = operation.Responses.Spec.Response["201"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it is ok", response.Spec.Spec.Description)

	response = operation.Responses.Spec.Default
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it is ok", response.Spec.Spec.Description)
}

func TestParseResponseCommentWithHeader(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)
	err := operation.ParseComment(`@Success 200 "it's ok"`, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	err = operation.ParseComment(`@Header 200 {string} Token "qwerty"`, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	err = operation.ParseComment(`@Header 200 "Mallformed"`, nil)
	assert.Error(t, err, "ParseComment should fail")

	err = operation.ParseComment(`@Header 200,asdsd {string} Token "qwerty"`, nil)
	assert.Error(t, err, "ParseComment should fail")
}

func TestParseResponseCommentWithHeaderForCodes(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)

	comment := `@Success 200,201,default "it's ok"`
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	comment = `@Header 200,201,default {string} Token "qwerty"`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	comment = `@Header all {string} Token2 "qwerty"`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token2"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token2"].Spec.Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Response["201"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token2"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token2"].Spec.Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Default
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token2"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token2"].Spec.Spec.Schema.Spec.Type)

	comment = `@Header 200 "Mallformed"`
	err = operation.ParseComment(comment, nil)
	assert.Error(t, err, "ParseComment should not fail")
}

func TestParseResponseCommentWithHeaderOnlyAll(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)

	comment := `@Success 200,201,default "it's ok"`
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	comment = `@Header all {string} Token "qwerty"`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Response["201"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	response = operation.Responses.Spec.Default
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "it's ok", response.Spec.Spec.Description)

	assert.Equal(t, "qwerty", response.Spec.Spec.Headers["Token"].Spec.Spec.Description)
	assert.Equal(t, &typeString, response.Spec.Spec.Headers["Token"].Spec.Spec.Schema.Spec.Type)

	comment = `@Header 200 "Mallformed"`
	err = operation.ParseComment(comment, nil)
	assert.Error(t, err, "ParseComment should not fail")
}

func TestParseEmptyResponseOnlyCode(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)
	err := operation.ParseComment(`@Success 200`, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "OK", response.Spec.Spec.Description)
}

func TestParseEmptyResponseOnlyCodes(t *testing.T) {
	t.Parallel()

	comment := `@Success 200,201,default`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err, "ParseComment should not fail")

	response := operation.Responses.Spec.Response["200"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "OK", response.Spec.Spec.Description)

	response = operation.Responses.Spec.Response["201"]
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "Created", response.Spec.Spec.Description)

	response = operation.Responses.Spec.Default
	assert.NotNil(t, response)
	assert.NotNil(t, response.Spec)

	assert.Equal(t, "", response.Spec.Spec.Description)
}

func TestParseResponseCommentParamMissing(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)

	paramLenErrComment := `@Success notIntCode`
	paramLenErr := operation.ParseComment(paramLenErrComment, nil)
	assert.EqualError(t, paramLenErr, `can not parse response comment "notIntCode"`)

	paramLenErrComment = `@Success notIntCode {string} string "it ok"`
	paramLenErr = operation.ParseComment(paramLenErrComment, nil)
	assert.EqualError(t, paramLenErr, `can not parse response comment "notIntCode {string} string "it ok""`)

	paramLenErrComment = `@Success notIntCode "it ok"`
	paramLenErr = operation.ParseComment(paramLenErrComment, nil)
	assert.EqualError(t, paramLenErr, `can not parse response comment "notIntCode "it ok""`)
}

func TestOperation_ParseParamComment(t *testing.T) {
	t.Parallel()

	t.Run("integer", func(t *testing.T) {
		t.Parallel()
		for _, paramType := range []string{"header", "path", "query"} {
			t.Run(paramType, func(t *testing.T) {
				o := NewOperation(New())
				err := o.ParseComment(`@Param some_id `+paramType+` int true "Some ID"`, nil)

				assert.NoError(t, err)

				expected := &spec.RefOrSpec[spec.Extendable[spec.Parameter]]{
					Spec: &spec.Extendable[spec.Parameter]{
						Spec: &spec.Parameter{
							Name:        "some_id",
							Description: "Some ID",
							In:          paramType,
							Required:    true,
							Schema: &spec.RefOrSpec[spec.Schema]{
								Spec: &spec.Schema{
									JsonSchema: spec.JsonSchema{
										JsonSchemaCore: spec.JsonSchemaCore{
											Type: &typeInteger,
										},
									},
								},
							},
						},
					},
				}

				expectedArray := []*spec.RefOrSpec[spec.Extendable[spec.Parameter]]{expected}
				assert.Equal(t, o.Parameters, expectedArray)
			})
		}
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()
		for _, paramType := range []string{"header", "path", "query"} {
			t.Run(paramType, func(t *testing.T) {
				o := NewOperation(New())
				err := o.ParseComment(`@Param some_string `+paramType+` string true "Some String"`, nil)

				assert.NoError(t, err)
				expected := &spec.RefOrSpec[spec.Extendable[spec.Parameter]]{
					Spec: &spec.Extendable[spec.Parameter]{
						Spec: &spec.Parameter{
							Description: "Some String",
							Name:        "some_string",
							In:          paramType,
							Required:    true,
							Schema: &spec.RefOrSpec[spec.Schema]{
								Spec: &spec.Schema{
									JsonSchema: spec.JsonSchema{
										JsonSchemaCore: spec.JsonSchemaCore{
											Type: &typeString,
										},
									},
								},
							},
						},
					},
				}

				expectedArray := []*spec.RefOrSpec[spec.Extendable[spec.Parameter]]{expected}
				assert.Equal(t, o.Parameters, expectedArray)
			})
		}
	})

	t.Run("object", func(t *testing.T) {
		t.Parallel()
		for _, paramType := range []string{"header", "path", "query"} {
			t.Run(paramType, func(t *testing.T) {
				assert.Error(t,
					NewOperation(New()).
						ParseComment(`@Param some_object `+paramType+` main.Object true "Some Object"`,
							nil))
			})
		}
	})

	t.Run("struct queries", func(t *testing.T) {
		t.Parallel()
		parser := New()
		parser.packages.uniqueDefinitions["main.Object"] = &TypeSpecDef{
			File: &ast.File{Name: &ast.Ident{Name: "test"}},
			TypeSpec: &ast.TypeSpec{
				Name:       &ast.Ident{Name: "Field"},
				TypeParams: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{{Name: "T"}}}}},
				Type: &ast.StructType{
					Struct: 100,
					Fields: &ast.FieldList{
						List: []*ast.Field{
							{
								Names: []*ast.Ident{
									{Name: "T"},
								},
								Type: ast.NewIdent("string"),
							},
							{
								Names: []*ast.Ident{
									{Name: "T2"},
								},
								Type: ast.NewIdent("string"),
							},
						},
					},
				},
			},
		}
		o := NewOperation(parser)
		err := o.ParseComment(`@Param some_object query main.Object true "Some Object"`,
			nil)

		assert.NoError(t, err)

		expectedT := &spec.RefOrSpec[spec.Extendable[spec.Parameter]]{
			Spec: &spec.Extendable[spec.Parameter]{
				Spec: &spec.Parameter{
					Name: "t",
					In:   "query",
					Schema: &spec.RefOrSpec[spec.Schema]{
						Spec: &spec.Schema{
							JsonSchema: spec.JsonSchema{
								JsonSchemaCore: spec.JsonSchemaCore{
									Type: &typeString,
								},
							},
						},
					},
				},
			},
		}
		expectedT2 := &spec.RefOrSpec[spec.Extendable[spec.Parameter]]{
			Spec: &spec.Extendable[spec.Parameter]{
				Spec: &spec.Parameter{
					Name: "t2",
					In:   "query",
					Schema: &spec.RefOrSpec[spec.Schema]{
						Spec: &spec.Schema{
							JsonSchema: spec.JsonSchema{
								JsonSchemaCore: spec.JsonSchemaCore{
									Type: &typeString,
								},
							},
						},
					},
				},
			},
		}

		assert.Len(t, o.Parameters, 2)
		tFound := false
		t2Found := false
		for _, param := range o.Parameters {
			switch param.Spec.Spec.Name {
			case "t":
				assert.EqualValues(t, expectedT, param)
				tFound = true
			case "t2":
				assert.EqualValues(t, expectedT2, param)
				t2Found = true
			default:
				assert.Fail(t, "unexpected result")
			}
		}

		assert.True(t, tFound, "results should contain t")
		assert.True(t, t2Found, "results should contain t2")
	})
}

// Test ParseParamComment Query Params
func TestParseParamCommentBodyArray(t *testing.T) {
	t.Parallel()

	comment := `@Param names body []string true "Users List"`
	o := NewOperation(New())
	err := o.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.NotNil(t, o.RequestBody)
	assert.Equal(t, "Users List", o.RequestBody.Spec.Spec.Description)
	assert.True(t, o.RequestBody.Spec.Spec.Required)
	assert.Equal(t, &typeArray, o.RequestBody.Spec.Spec.Content["application/json"].Spec.Schema.Spec.Type)
}

func TestParseParamCommentArray(t *testing.T) {
	paramTypes := []string{"header", "path", "query"}

	for _, paramType := range paramTypes {
		t.Run(paramType, func(t *testing.T) {
			operation := NewOperation(New())
			err := operation.ParseComment(`@Param names `+paramType+` []string true "Users List"`, nil)
			assert.NoError(t, err)

			parameters := operation.Operation.Parameters
			assert.NotNil(t, parameters)

			parameterSpec := parameters[0].Spec.Spec
			assert.NotNil(t, parameterSpec)
			assert.Equal(t, "Users List", parameterSpec.Description)
			assert.Equal(t, "names", parameterSpec.Name)
			assert.Equal(t, &typeArray, parameterSpec.Schema.Spec.Type)
			assert.Equal(t, true, parameterSpec.Required)
			assert.Equal(t, paramType, parameterSpec.In)
			assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Items.Schema.Spec.Type)

			err = operation.ParseComment(`@Param names `+paramType+` []model.User true "Users List"`, nil)
			assert.Error(t, err)
		})
	}
}

func TestParseParamCommentDefaultValue(t *testing.T) {
	t.Parallel()

	operation := NewOperation(New())
	err := operation.ParseComment(`@Param names query string true "Users List" default(test)`, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Users List", parameterSpec.Description)
	assert.Equal(t, "names", parameterSpec.Name)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, true, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, "test", parameterSpec.Schema.Spec.Default)
}

func TestParseParamCommentQueryArrayFormat(t *testing.T) {
	t.Parallel()

	comment := `@Param names query []string true "Users List" collectionFormat(multi)`
	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Users List", parameterSpec.Description)
	assert.Equal(t, "names", parameterSpec.Name)
	assert.Equal(t, &typeArray, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, true, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Items.Schema.Spec.Type)
	assert.Equal(t, "form", parameterSpec.Style)

}

func TestParseParamCommentByID(t *testing.T) {
	t.Parallel()

	comment := `@Param unsafe_id[lte] query int true "Unsafe query param"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Unsafe query param", parameterSpec.Description)
	assert.Equal(t, "unsafe_id[lte]", parameterSpec.Name)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, true, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
}

func TestParseParamCommentByQueryType(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query int true "Some ID"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, true, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
}

func TestParseParamCommentByBodyType(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id body model.OrderRow true "Some ID"`
	operation := NewOperation(New())

	operation.parser.addTestType("model.OrderRow")

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, "Some ID", requestBodySpec.Description)
	assert.Equal(t, true, requestBodySpec.Required)
	assert.Equal(t, "#/components/schemas/model.OrderRow", requestBodySpec.Content["application/json"].Spec.Schema.Ref.Ref)
}

func TestParseParamCommentByBodyTextPlain(t *testing.T) {
	t.Parallel()

	comment := `@Param text body string true "Text to process"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, "Text to process", requestBodySpec.Description)
	assert.Equal(t, true, requestBodySpec.Required)
	assert.Equal(t, &typeString, requestBodySpec.Content["text/plain"].Spec.Schema.Spec.Type)
}

func TestParseParamCommentByBodyTypeWithDeepNestedFields(t *testing.T) {
	t.Parallel()

	comment := `@Param body body model.CommonHeader{data=string,data2=int} true "test deep"`
	operation := NewOperation(New())

	operation.parser.addTestType("model.CommonHeader")

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.Len(t, operation.Parameters, 0)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, "test deep", requestBodySpec.Description)
	assert.True(t, requestBodySpec.Required)

	assert.Equal(t, 2, len(requestBodySpec.Content["application/json"].Spec.Schema.Spec.AllOf))
	assert.Equal(t, 3, len(operation.parser.openAPI.Components.Spec.Schemas))
}

func TestParseParamCommentByBodyTypeArrayOfPrimitiveGo(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id body []int true "Some ID"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, "Some ID", requestBodySpec.Description)
	assert.True(t, requestBodySpec.Required)
	assert.Equal(t, &typeArray, requestBodySpec.Content["application/json"].Spec.Schema.Spec.Type)
	assert.Equal(t, &typeInteger, requestBodySpec.Content["application/json"].Spec.Schema.Spec.Items.Schema.Spec.Type)
}

func TestParseParamCommentByBodyTypeArrayOfPrimitiveGoWithDeepNestedFields(t *testing.T) {
	t.Parallel()

	comment := `@Param body body []model.CommonHeader{data=string,data2=int} true "test deep"`
	operation := NewOperation(New())
	operation.parser.addTestType("model.CommonHeader")

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.Len(t, operation.Parameters, 0)

	assert.NotNil(t, operation.RequestBody)

	parameterSpec := operation.RequestBody.Spec.Spec.Content["application/json"].Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "test deep", operation.RequestBody.Spec.Spec.Description)
	assert.Equal(t, &typeArray, parameterSpec.Schema.Spec.Type)
	assert.True(t, operation.RequestBody.Spec.Spec.Required)
	assert.Equal(t, 2, len(parameterSpec.Schema.Spec.Items.Schema.Spec.AllOf))
}

func TestParseParamCommentByBodyTypeErr(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id body model.OrderRow true "Some ID"`
	operation := NewOperation(New())
	operation.parser.addTestType("model.notexist")

	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseParamCommentByFormDataType(t *testing.T) {
	t.Parallel()

	comment := `@Param file formData file true "this is a test file"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.Len(t, operation.Parameters, 0)
	assert.NotNil(t, operation.RequestBody)

	requestBody := operation.RequestBody
	assert.True(t, requestBody.Spec.Spec.Required)
	assert.Equal(t, "this is a test file", requestBody.Spec.Spec.Description)
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, &typeFile, requestBodySpec.Content["application/x-www-form-urlencoded"].Spec.Schema.Spec.Type)
}

func TestParseParamCommentByFormDataTypeUint64V3(t *testing.T) {
	t.Parallel()

	comment := `@Param file formData uint64 true "this is a test file"`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.Len(t, operation.Parameters, 0)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)
	assert.Equal(t, "this is a test file", requestBody.Spec.Spec.Description)

	requestBodySpec := requestBody.Spec.Spec.Content["application/x-www-form-urlencoded"].Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, &typeInteger, requestBodySpec.Schema.Spec.Type)
}

func TestParseParamCommentByNotSupportedType(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id not_supported int true "Some ID"`
	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)

	assert.Error(t, err)
}

func TestParseParamCommentNotMatch(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id body mock true`
	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)

	assert.Error(t, err)
}

func TestParseParamCommentByEnums(t *testing.T) {
	t.Parallel()

	t.Run("string", func(t *testing.T) {
		comment := `@Param some_id query string true "Some ID" Enums(A, B, C)`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)

		assert.Len(t, operation.Parameters, 1)

		parameters := operation.Operation.Parameters
		assert.NotNil(t, parameters)

		parameterSpec := parameters[0].Spec.Spec
		assert.NotNil(t, parameterSpec)
		assert.Equal(t, "Some ID", parameterSpec.Description)
		assert.Equal(t, "some_id", parameterSpec.Name)
		assert.True(t, parameterSpec.Required)
		assert.Equal(t, "query", parameterSpec.In)
		assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Type)
		assert.Equal(t, 3, len(parameterSpec.Schema.Spec.Enum))

		enums := []interface{}{"A", "B", "C"}
		assert.EqualValues(t, enums, parameterSpec.Schema.Spec.Enum)
	})

	t.Run("int", func(t *testing.T) {
		comment := `@Param some_id query int true "Some ID" Enums(1, 2, 3)`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)

		parameters := operation.Operation.Parameters
		assert.NotNil(t, parameters)

		parameterSpec := parameters[0].Spec.Spec
		assert.NotNil(t, parameterSpec)
		assert.Equal(t, "Some ID", parameterSpec.Description)
		assert.Equal(t, "some_id", parameterSpec.Name)
		assert.True(t, parameterSpec.Required)
		assert.Equal(t, "query", parameterSpec.In)
		assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
		assert.Equal(t, 3, len(parameterSpec.Schema.Spec.Enum))

		enums := []interface{}{1, 2, 3}
		assert.EqualValues(t, enums, parameterSpec.Schema.Spec.Enum)
	})

	t.Run("number", func(t *testing.T) {
		comment := `@Param some_id query number true "Some ID" Enums(1.1, 2.2, 3.3)`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)

		parameters := operation.Operation.Parameters
		assert.NotNil(t, parameters)

		parameterSpec := parameters[0].Spec.Spec
		assert.NotNil(t, parameterSpec)
		assert.Equal(t, "Some ID", parameterSpec.Description)
		assert.Equal(t, "some_id", parameterSpec.Name)
		assert.True(t, parameterSpec.Required)
		assert.Equal(t, "query", parameterSpec.In)
		assert.Equal(t, &typeNumber, parameterSpec.Schema.Spec.Type)
		assert.Equal(t, 3, len(parameterSpec.Schema.Spec.Enum))

		enums := []interface{}{1.1, 2.2, 3.3}
		assert.EqualValues(t, enums, parameterSpec.Schema.Spec.Enum)
	})

	t.Run("bool", func(t *testing.T) {
		comment := `@Param some_id query bool true "Some ID" Enums(true, false)`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)

		parameters := operation.Operation.Parameters
		assert.NotNil(t, parameters)

		parameterSpec := parameters[0].Spec.Spec
		assert.NotNil(t, parameterSpec)
		assert.Equal(t, "Some ID", parameterSpec.Description)
		assert.Equal(t, "some_id", parameterSpec.Name)
		assert.True(t, parameterSpec.Required)
		assert.Equal(t, "query", parameterSpec.In)
		assert.Equal(t, &typeBool, parameterSpec.Schema.Spec.Type)
		assert.Equal(t, 2, len(parameterSpec.Schema.Spec.Enum))

		enums := []interface{}{true, false}
		assert.EqualValues(t, enums, parameterSpec.Schema.Spec.Enum)
	})

	operation := NewOperation(New())

	comment := `@Param some_id query int true "Some ID" Enums(A, B, C)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query number true "Some ID" Enums(A, B, C)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query boolean true "Some ID" Enums(A, B, C)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query Document true "Some ID" Enums(A, B, C)`
	assert.Error(t, operation.ParseComment(comment, nil))
}

func TestParseParamCommentByMaxLength(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query string true "Some ID" MaxLength(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, *parameterSpec.Schema.Spec.MaxLength)

	comment = `@Param some_id query int true "Some ID" MaxLength(10)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query string true "Some ID" MaxLength(Goopher)`
	assert.Error(t, operation.ParseComment(comment, nil))
}

func TestParseParamCommentByMinLength(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query string true "Some ID" MinLength(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, *parameterSpec.Schema.Spec.MinLength)

	comment = `@Param some_id query int true "Some ID" MinLength(10)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query string true "Some ID" MinLength(Goopher)`
	assert.Error(t, operation.ParseComment(comment, nil))
}

func TestParseParamCommentByMinimum(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query int true "Some ID" Minimum(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, *parameterSpec.Schema.Spec.Minimum)

	comment = `@Param some_id query int true "Some ID" Mininum(10)`
	assert.NoError(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query string true "Some ID" Minimum(10)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query integer true "Some ID" Minimum(Goopher)`
	assert.Error(t, operation.ParseComment(comment, nil))
}

func TestParseParamCommentByMaximum(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query int true "Some ID" Maximum(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, *parameterSpec.Schema.Spec.Maximum)

	comment = `@Param some_id query int true "Some ID" Maxinum(10)`
	assert.NoError(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query string true "Some ID" Maximum(10)`
	assert.Error(t, operation.ParseComment(comment, nil))

	comment = `@Param some_id query integer true "Some ID" Maximum(Goopher)`
	assert.Error(t, operation.ParseComment(comment, nil))
}

func TestParseParamCommentByDefault(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query int true "Some ID" Default(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, parameterSpec.Schema.Spec.Default)
}

func TestParseParamCommentByExampleInt(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query int true "Some ID" Example(10)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, 10, parameterSpec.Example)
}

func TestParseParamCommentByExampleString(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id query string true "Some ID" Example(True feelings)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, "query", parameterSpec.In)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, "True feelings", parameterSpec.Example)
}

func TestParseParamCommentBySchemaExampleString(t *testing.T) {
	t.Parallel()

	comment := `@Param some_id body string true "Some ID" SchemaExample(True feelings)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	requestBody := operation.RequestBody
	assert.NotNil(t, requestBody)

	requestBodySpec := requestBody.Spec.Spec
	assert.NotNil(t, requestBodySpec)
	assert.Equal(t, "Some ID", requestBodySpec.Description)
	assert.True(t, requestBodySpec.Required)
	assert.Equal(t, "True feelings", requestBodySpec.Content["text/plain"].Spec.Schema.Spec.Example)
	assert.Equal(t, &typeString, requestBodySpec.Content["text/plain"].Spec.Schema.Spec.Type)
}

func TestParseParamCommentBySchemaExampleUnsupportedType(t *testing.T) {
	t.Parallel()
	var param spec.Parameter

	setSchemaExample(nil, "something", "random value")
	assert.Nil(t, param.Schema)

	setSchemaExample(nil, STRING, "string value")
	assert.Nil(t, param.Schema)

	param.Schema = spec.NewSchemaSpec()
	setSchemaExample(param.Schema.Spec, STRING, "string value")
	assert.Equal(t, "string value", param.Schema.Spec.Example)

	setSchemaExample(param.Schema.Spec, INTEGER, "10")
	assert.Equal(t, 10, param.Schema.Spec.Example)

	setSchemaExample(param.Schema.Spec, NUMBER, "10")
	assert.Equal(t, float64(10), param.Schema.Spec.Example)

	setSchemaExample(param.Schema.Spec, STRING, "string \\r\\nvalue")
	assert.Equal(t, "string \r\nvalue", param.Schema.Spec.Example)
}

func TestParseParamArrayWithEnums(t *testing.T) {
	t.Parallel()

	comment := `@Param field query []string true "An enum collection" collectionFormat(csv) enums(also,valid)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "An enum collection", parameterSpec.Description)
	assert.Equal(t, "field", parameterSpec.Name)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, &typeArray, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, "form", parameterSpec.Style)

	enums := []interface{}{"also", "valid"}
	assert.EqualValues(t, enums, parameterSpec.Schema.Spec.Items.Schema.Spec.Enum)
	assert.Equal(t, &typeString, parameterSpec.Schema.Spec.Items.Schema.Spec.Type)
}

func TestParseAndExtractionParamAttribute(t *testing.T) {
	t.Parallel()

	op := NewOperation(New())

	t.Run("number", func(t *testing.T) {
		numberParam := spec.Parameter{
			Schema: spec.NewSchemaSpec(),
		}
		err := op.parseParamAttribute(
			" default(1) maximum(100) minimum(0) format(csv)",
			"",
			NUMBER,
			&numberParam,
		)
		assert.NoError(t, err)
		assert.Equal(t, int(0), *numberParam.Schema.Spec.Minimum)
		assert.Equal(t, int(100), *numberParam.Schema.Spec.Maximum)
		assert.Equal(t, "csv", numberParam.Schema.Spec.Format)
		assert.Equal(t, float64(1), numberParam.Schema.Spec.Default)

		err = op.parseParamAttribute(" minlength(1)", "", NUMBER, nil)
		assert.Error(t, err)

		err = op.parseParamAttribute(" maxlength(1)", "", NUMBER, nil)
		assert.Error(t, err)
	})

	t.Run("string", func(t *testing.T) {
		stringParam := spec.Parameter{
			Schema: spec.NewSchemaSpec(),
		}
		err := op.parseParamAttribute(
			" default(test) maxlength(100) minlength(0) format(csv)",
			"",
			STRING,
			&stringParam,
		)
		assert.NoError(t, err)
		assert.Equal(t, int(0), *stringParam.Schema.Spec.MinLength)
		assert.Equal(t, int(100), *stringParam.Schema.Spec.MaxLength)
		assert.Equal(t, "csv", stringParam.Schema.Spec.Format)
		err = op.parseParamAttribute(" minimum(0)", "", STRING, nil)
		assert.Error(t, err)

		err = op.parseParamAttribute(" maximum(0)", "", STRING, nil)
		assert.Error(t, err)
	})

	t.Run("array", func(t *testing.T) {
		arrayParam := spec.Parameter{
			Schema: spec.NewSchemaSpec(),
		}

		arrayParam.In = "path"
		err := op.parseParamAttribute(" collectionFormat(simple)", ARRAY, STRING, &arrayParam)
		assert.Equal(t, "simple", arrayParam.Style)
		assert.NoError(t, err)

		err = op.parseParamAttribute(" collectionFormat(simple)", STRING, STRING, nil)
		assert.Error(t, err)

		err = op.parseParamAttribute(" default(0)", "", ARRAY, nil)
		assert.Error(t, err)
	})
}

func TestParseParamCommentByExtensions(t *testing.T) {
	comment := `@Param some_id path int true "Some ID" extensions(x-example=test,x-custom=Gopher,x-custom2)`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	parameters := operation.Operation.Parameters
	assert.NotNil(t, parameters)

	parameterSpec := parameters[0].Spec.Spec
	assert.NotNil(t, parameterSpec)
	assert.Equal(t, "Some ID", parameterSpec.Description)
	assert.Equal(t, "some_id", parameterSpec.Name)
	assert.Equal(t, "path", parameterSpec.In)
	assert.True(t, parameterSpec.Required)
	assert.Equal(t, &typeInteger, parameterSpec.Schema.Spec.Type)
	assert.Equal(t, "Gopher", parameterSpec.Schema.Spec.Extensions["x-custom"])
	assert.Equal(t, true, parameterSpec.Schema.Spec.Extensions["x-custom2"])
	assert.Equal(t, "test", parameterSpec.Schema.Spec.Extensions["x-example"])
}

func TestParseIdComment(t *testing.T) {
	t.Parallel()

	comment := `@Id myOperationId`
	operation := NewOperation(nil)
	err := operation.ParseComment(comment, nil)

	assert.NoError(t, err)
	assert.Equal(t, "myOperationId", operation.Operation.OperationID)
}

func TestParseSecurityComment(t *testing.T) {
	t.Parallel()

	comment := `@Security OAuth2Implicit[read, write]`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	expected := []spec.SecurityRequirement{{
		"OAuth2Implicit": {"read", "write"},
	}}

	assert.Equal(t, expected, operation.Security)
}

func TestParseSecurityCommentSimple(t *testing.T) {
	t.Parallel()

	comment := `@Security ApiKeyAuth`
	operation := NewOperation(New())

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	expected := []spec.SecurityRequirement{{
		"ApiKeyAuth": {},
	}}

	assert.Equal(t, expected, operation.Security)
}

func TestParseSecurityCommentOr(t *testing.T) {
	t.Parallel()

	comment := `@Security OAuth2Implicit[read, write] || Firebase[]`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	expected := []spec.SecurityRequirement{{
		"OAuth2Implicit": {"read", "write"},
		"Firebase":       {""},
	}}

	assert.Equal(t, expected, operation.Security)
}

func TestParseMultiDescription(t *testing.T) {
	t.Parallel()

	comment := `@Description line one`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	comment = `@Tags multi`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	comment = `@Description line two x`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	assert.Equal(t, "line one\nline two x", operation.Description)
}

func TestParseDescriptionMarkdown(t *testing.T) {
	t.Parallel()

	operation := NewOperation(New())
	operation.parser.markdownFileDir = "example/markdown"

	comment := `@description.markdown admin.md`

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	comment = `@description.markdown missing.md`

	err = operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseSummary(t *testing.T) {
	t.Parallel()

	comment := `@summary line one`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)
	assert.Equal(t, "line one", operation.Summary)

	comment = `@Summary line one`
	err = operation.ParseComment(comment, nil)
	assert.NoError(t, err)
}

func TestParseDeprecationDescription(t *testing.T) {
	t.Parallel()

	comment := `@Deprecated`
	operation := NewOperation(nil)

	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)
	assert.True(t, operation.Deprecated)
}

func TestParseExtensions(t *testing.T) {
	t.Parallel()
	// Fail if there are no args for attributes.
	{
		comment := `@x-amazon-apigateway-integration`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.EqualError(t, err, "annotation @x-amazon-apigateway-integration need a value")
	}

	// Fail if args of attributes are broken.
	{
		comment := `@x-amazon-apigateway-integration ["broken"}]`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.EqualError(t, err, "annotation @x-amazon-apigateway-integration need a valid json value. error: invalid character '}' after array element")
	}

	// OK
	{
		comment := `@x-amazon-apigateway-integration {"uri": "${some_arn}", "passthroughBehavior": "when_no_match", "httpMethod": "POST", "type": "aws_proxy"}`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"httpMethod":          "POST",
			"passthroughBehavior": "when_no_match",
			"type":                "aws_proxy",
			"uri":                 "${some_arn}",
		}, operation.Responses.Extensions["x-amazon-apigateway-integration"])
	}

	// Test x-tagGroups
	{
		comment := `@x-tagGroups [{"name":"Natural Persons","tags":["Person","PersonRisk","PersonDocuments"]}]`
		operation := NewOperation(New())

		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)
		assert.Equal(t,
			[]interface{}{map[string]interface{}{
				"name": "Natural Persons",
				"tags": []interface{}{
					"Person",
					"PersonRisk",
					"PersonDocuments",
				},
			}}, operation.Responses.Extensions["x-tagGroups"])
	}
}

func TestParseResponseHeaderComment(t *testing.T) {
	t.Parallel()

	operation := NewOperation(New())

	err := operation.ParseResponseComment(`default {string} string "other error"`, nil)
	assert.NoError(t, err)
	err = operation.ParseResponseHeaderComment(`all {string} Token "qwerty"`, nil)
	assert.NoError(t, err)
}

func TestParseCodeSamples(t *testing.T) {
	t.Parallel()
	const comment = `@x-codeSamples file`
	t.Run("Find sample by file", func(t *testing.T) {

		operation := NewOperation(New(), SetCodeExampleFilesDirectory("testdata/code_examples"))
		operation.Summary = "example"

		err := operation.ParseComment(comment, nil)
		require.NoError(t, err, "no error should be thrown")

		assert.Equal(t, "example", operation.Summary)
		assert.Equal(t, CodeSamples(CodeSamples{map[string]string{"lang": "JavaScript", "source": "console.log('Hello World');"}}),
			operation.Responses.Extensions["x-codeSamples"],
		)
	})

	t.Run("With broken file sample", func(t *testing.T) {
		operation := NewOperation(New(), SetCodeExampleFilesDirectory("testdata/code_examples"))
		operation.Summary = "broken"

		err := operation.ParseComment(comment, nil)
		assert.Error(t, err, "no error should be thrown")
	})

	t.Run("Example file not found", func(t *testing.T) {
		operation := NewOperation(New(), SetCodeExampleFilesDirectory("testdata/code_examples"))
		operation.Summary = "badExample"

		err := operation.ParseComment(comment, nil)
		assert.Error(t, err, "error was expected, as file does not exist")
	})

	t.Run("Without line reminder", func(t *testing.T) {
		comment := `@x-codeSamples`
		operation := NewOperation(New(), SetCodeExampleFilesDirectory("testdata/code_examples"))
		operation.Summary = "example"

		err := operation.ParseComment(comment, nil)
		assert.Error(t, err, "no error should be thrown")
	})

	t.Run(" broken dir", func(t *testing.T) {
		operation := NewOperation(New(), SetCodeExampleFilesDirectory("testdata/fake_examples"))
		operation.Summary = "code"

		err := operation.ParseComment(comment, nil)
		assert.Error(t, err, "no error should be thrown")
	})
}

func TestParseAcceptComment(t *testing.T) {
	t.Parallel()

	comment := `//@Accept json,xml,plain,html,mpfd,x-www-form-urlencoded,json-api,json-stream,octet-stream,png,jpeg,gif,application/xhtml+xml,application/health+json`
	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)
	assert.NoError(t, err)

	resultMapKeys := []string{
		"application/json",
		"text/xml",
		"text/plain",
		"text/html",
		"multipart/form-data",
		"application/x-www-form-urlencoded",
		"application/vnd.api+json",
		"application/x-json-stream",
		"application/octet-stream",
		"image/png",
		"image/jpeg",
		"image/gif",
		"application/xhtml+xml",
		"application/health+json"}

	content := operation.RequestBody.Spec.Spec.Content
	for _, key := range resultMapKeys {
		assert.NotNil(t, content[key])
	}

	assert.Equal(t, &typeObject, content["application/json"].Spec.Schema.Spec.Type)
	assert.Equal(t, &typeObject, content["text/xml"].Spec.Schema.Spec.Type)
	assert.Equal(t, &typeString, content["image/png"].Spec.Schema.Spec.Type)
	assert.Equal(t, "binary", content["image/png"].Spec.Schema.Spec.Format)
}

func TestParseAcceptCommentErr(t *testing.T) {
	t.Parallel()

	comment := `//@Accept unknown`
	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)
	assert.Error(t, err)
}

func TestParseProduceCommand(t *testing.T) {
	t.Parallel()

	t.Run("Produce success", func(t *testing.T) {
		t.Parallel()

		const comment = "//@Produce application/json,text/csv,application/zip"

		operation := NewOperation(New())
		err := operation.ParseComment(comment, nil)
		assert.NoError(t, err)

		assert.Equal(t, 3, len(operation.responseMimeTypes))
	})

	t.Run("Produce Invalid Mime Type", func(t *testing.T) {
		t.Parallel()

		const comment = "//@Produce text,stuff,gophers"

		operation := NewOperation(New())
		err := operation.ParseComment(comment, nil)
		assert.Error(t, err)
	})
}

func TestProcessProduceComment(t *testing.T) {
	t.Parallel()

	const comment = "//@Produce application/json,text/csv,application/zip"

	operation := NewOperation(New())
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	operation.Responses.Spec.Response = make(map[string]*spec.RefOrSpec[spec.Extendable[spec.Response]])
	operation.Responses.Spec.Response["200"] = spec.NewResponseSpec()
	operation.Responses.Spec.Response["201"] = spec.NewResponseSpec()
	operation.Responses.Spec.Response["204"] = spec.NewResponseSpec()
	operation.Responses.Spec.Response["400"] = spec.NewResponseSpec()
	operation.Responses.Spec.Response["500"] = spec.NewResponseSpec()

	err = operation.ProcessProduceComment()
	require.NoError(t, err)

	content := operation.Responses.Spec.Response["200"].Spec.Spec.Content
	assert.Equal(t, 3, len(content))
	assert.NotNil(t, content["application/json"].Spec.Schema)
	assert.NotNil(t, content["text/csv"].Spec.Schema)
	assert.NotNil(t, content["application/zip"].Spec.Schema)

	content = operation.Responses.Spec.Response["201"].Spec.Spec.Content
	assert.Equal(t, 3, len(content))
	assert.NotNil(t, content["application/json"].Spec.Schema)
	assert.NotNil(t, content["text/csv"].Spec.Schema)
	assert.NotNil(t, content["application/zip"].Spec.Schema)

	content = operation.Responses.Spec.Response["204"].Spec.Spec.Content
	assert.Nil(t, content)

	content = operation.Responses.Spec.Response["400"].Spec.Spec.Content
	assert.Nil(t, content)

	content = operation.Responses.Spec.Response["500"].Spec.Spec.Content
	assert.Nil(t, content)
}

func TestParseServerComment(t *testing.T) {
	t.Parallel()

	operation := NewOperation(nil)

	comment := `/@servers.url https://api.example.com/v1`
	err := operation.ParseComment(comment, nil)
	require.NoError(t, err)

	comment = `/@servers.description override path 1`
	err = operation.ParseComment(comment, nil)
	require.NoError(t, err)

	comment = `/@servers.url https://api.example.com/v2`
	err = operation.ParseComment(comment, nil)
	require.NoError(t, err)

	comment = `/@servers.description override path 2`
	err = operation.ParseComment(comment, nil)
	require.NoError(t, err)

	assert.Len(t, operation.Servers, 2)
	assert.Equal(t, "https://api.example.com/v1", operation.Servers[0].Spec.URL)
	assert.Equal(t, "override path 1", operation.Servers[0].Spec.Description)
	assert.Equal(t, "https://api.example.com/v2", operation.Servers[1].Spec.URL)
	assert.Equal(t, "override path 2", operation.Servers[1].Spec.Description)
}

func TestResponseSchemaWithCustomMimeType(t *testing.T) {
	t.Parallel()

	t.Run("Schema ref is correctly associated with custom MIME type", func(t *testing.T) {
		t.Parallel()

		// Create operation with parser to handle the type reference
		parser := New()
		operation := NewOperation(parser)

		// Create a mock type in the parser as a stand-in for model.OrderRow
		parser.addTestType("model.OrderRow")

		// First, set the response MIME type with @Produce
		err := operation.ParseComment("/@Produce json-api", nil)
		require.NoError(t, err)

		// Then set the response with @Success
		err = operation.ParseComment("/@Success 200 {object} model.OrderRow", nil)
		require.NoError(t, err)

		// Check that we have a response for status code 200
		response, exists := operation.Responses.Spec.Response["200"]
		require.True(t, exists, "Response for status code 200 should exist")

		// Verify the correct MIME type (json-api -> application/vnd.api+json) has the schema reference
		content := response.Spec.Spec.Content
		require.NotNil(t, content, "Response content should not be nil")

		// Check that application/vnd.api+json exists in the content map
		apiJsonContent, exists := content["application/vnd.api+json"]
		require.True(t, exists, "application/vnd.api+json content should exist")

		// Verify the schema reference is correct
		require.NotNil(t, apiJsonContent.Spec.Schema, "Schema should not be nil")
		require.NotNil(t, apiJsonContent.Spec.Schema.Ref, "Schema ref should not be nil")
		require.Equal(t, "#/components/schemas/model.OrderRow", apiJsonContent.Spec.Schema.Ref.Ref)

		// Make sure the schema is NOT also defined under application/json
		_, exists = content["application/json"]
		require.False(t, exists, "application/json content should not exist when only json-api was specified")
	})

	t.Run("Default to application/json when no MIME type is specified", func(t *testing.T) {
		t.Parallel()

		// Create operation with parser to handle the type reference
		parser := New()
		operation := NewOperation(parser)

		// Create a mock type in the parser
		parser.addTestType("model.OrderRow")

		// Only set the response with @Success, without any @Produce
		err := operation.ParseComment("/@Success 200 {object} model.OrderRow", nil)
		require.NoError(t, err)

		// Check that we have a response for status code 200
		response, exists := operation.Responses.Spec.Response["200"]
		require.True(t, exists, "Response for status code 200 should exist")

		// Verify application/json has the schema reference
		content := response.Spec.Spec.Content
		require.NotNil(t, content, "Response content should not be nil")

		// Check that application/json exists in the content map
		jsonContent, exists := content["application/json"]
		require.True(t, exists, "application/json content should exist")

		// Verify the schema reference is correct
		require.NotNil(t, jsonContent.Spec.Schema, "Schema should not be nil")
		require.NotNil(t, jsonContent.Spec.Schema.Ref, "Schema ref should not be nil")
		require.Equal(t, "#/components/schemas/model.OrderRow", jsonContent.Spec.Schema.Ref.Ref)
	})

	t.Run("Multiple MIME types have the same schema reference", func(t *testing.T) {
		t.Parallel()

		// Create operation with parser to handle the type reference
		parser := New()
		operation := NewOperation(parser)

		// Create a mock type in the parser
		parser.addTestType("model.OrderRow")

		// Set multiple MIME types
		err := operation.ParseComment("/@Produce json,json-api", nil)
		require.NoError(t, err)

		// Set the response
		err = operation.ParseComment("/@Success 200 {object} model.OrderRow", nil)
		require.NoError(t, err)

		// Check that we have a response for status code 200
		response, exists := operation.Responses.Spec.Response["200"]
		require.True(t, exists, "Response for status code 200 should exist")

		// Verify both MIME types have the schema reference
		content := response.Spec.Spec.Content
		require.NotNil(t, content, "Response content should not be nil")

		// Check application/json
		jsonContent, exists := content["application/json"]
		require.True(t, exists, "application/json content should exist")
		require.NotNil(t, jsonContent.Spec.Schema, "Schema should not be nil")
		require.NotNil(t, jsonContent.Spec.Schema.Ref, "Schema ref should not be nil")
		require.Equal(t, "#/components/schemas/model.OrderRow", jsonContent.Spec.Schema.Ref.Ref)

		// Check application/vnd.api+json
		apiJsonContent, exists := content["application/vnd.api+json"]
		require.True(t, exists, "application/vnd.api+json content should exist")
		require.NotNil(t, apiJsonContent.Spec.Schema, "Schema should not be nil")
		require.NotNil(t, apiJsonContent.Spec.Schema.Ref, "Schema ref should not be nil")
		require.Equal(t, "#/components/schemas/model.OrderRow", apiJsonContent.Spec.Schema.Ref.Ref)
	})
}

func TestParseParamStructEnumQuery(t *testing.T) {
	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operationv3_test.go", `package swag
	import structs "github.com/swaggo/swag/testdata/param_structs"
	`, goparser.ParseComments)
	require.NoError(t, err)

	parser := New()
	err = parser.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = parser.packages.ParseTypes()
	require.NoError(t, err)

	o := NewOperation(parser)
	err = o.ParseComment(`@Param model query structs.EnumQueryModel true "q"`, astFile)
	require.NoError(t, err)

	require.Len(t, o.Parameters, 2)
	byName := map[string]*spec.Parameter{}
	for _, p := range o.Parameters {
		byName[p.Spec.Spec.Name] = p.Spec.Spec
	}

	dir := byName["direction"]
	require.NotNil(t, dir)
	assert.Equal(t, &typeString, dir.Schema.Spec.Type)
	assert.EqualValues(t, []interface{}{"asc", "desc"}, dir.Schema.Spec.Enum)
	assert.Equal(t, "desc", dir.Schema.Spec.Default)
	assert.NotNil(t, dir.Schema.Spec.Example, "example inferred from first enum value")

	status := byName["status"]
	require.NotNil(t, status)
	assert.EqualValues(t, []interface{}{"asc", "desc"}, status.Schema.Spec.Enum)

	// The shared component must NOT be polluted by the fields' own tags:
	// Direction's default and Status's example are per-field and belong on the
	// parameters (via allOf), never on the type every other usage references.
	comp := parser.openAPI.Components.Spec.Schemas["structs.OrderDirection"]
	require.NotNil(t, comp)
	require.NotNil(t, comp.Spec)
	assert.Nil(t, comp.Spec.Default, "component must not inherit a field's default")
	assert.Nil(t, comp.Spec.Example, "component must not inherit a field's example")
	assert.EqualValues(t, []interface{}{"asc", "desc"}, comp.Spec.Enum, "component enum must not be duplicated")
}

func TestParseParamStructQueryRequiredSemantics(t *testing.T) {
	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operationv3_test.go", `package swag
	import structs "github.com/swaggo/swag/testdata/param_structs"
	`, goparser.ParseComments)
	require.NoError(t, err)

	parser := New()
	parser.RequiredByDefault = true
	err = parser.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = parser.packages.ParseTypes()
	require.NoError(t, err)

	o := NewOperation(parser)
	err = o.ParseComment(`@Param model query structs.RequiredQueryModel false "q"`, astFile)
	require.NoError(t, err)

	byName := map[string]*spec.Parameter{}
	for _, p := range o.Parameters {
		byName[p.Spec.Spec.Name] = p.Spec.Spec
	}

	require.NotNil(t, byName["q"])
	assert.True(t, byName["q"].Required, "binding:required query param must be required")
	require.NotNil(t, byName["limit"])
	assert.True(t, byName["limit"].Required, "validate:required query param must be required")
	require.NotNil(t, byName["filter.name"])
	assert.False(t, byName["filter.name"].Required, "unmarked query filter must stay optional even under requiredByDefault")
}

func TestParseParamStructFormDataMultipart(t *testing.T) {
	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operationv3_test.go", `package swag
	import structs "github.com/swaggo/swag/testdata/param_structs"
	`, goparser.ParseComments)
	require.NoError(t, err)

	parser := New()
	parser.RequiredByDefault = true
	err = parser.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = parser.packages.ParseTypes()
	require.NoError(t, err)

	o := NewOperation(parser)
	err = o.ParseComment(`@Param request formData structs.UploadForm true "upload"`, astFile)
	require.NoError(t, err)

	content := o.RequestBody.Spec.Spec.Content
	require.NotNil(t, content["multipart/form-data"], "a file field selects multipart/form-data")
	require.Nil(t, content["application/x-www-form-urlencoded"], "no stray urlencoded content type")

	sch := content["multipart/form-data"].Spec.Schema.Spec
	require.NotNil(t, sch)
	require.Equal(t, OBJECT, (*sch.Type)[0])

	file := sch.Properties["file"]
	require.NotNil(t, file)
	assert.Equal(t, STRING, (*file.Spec.Type)[0])
	assert.Equal(t, "binary", file.Spec.Format)

	label := sch.Properties["label"]
	require.NotNil(t, label)
	assert.Equal(t, STRING, (*label.Spec.Type)[0])

	tags := sch.Properties["tags"]
	require.NotNil(t, tags)
	assert.Equal(t, ARRAY, (*tags.Spec.Type)[0])
	assert.Equal(t, STRING, (*tags.Spec.Items.Schema.Spec.Type)[0])

	assert.Equal(t, []string{"file"}, sch.Required, "only the binding:required field is required, not every field under requiredByDefault")
}

func TestParseParamStructEnumArrayQuery(t *testing.T) {
	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operationv3_test.go", `package swag
	import structs "github.com/swaggo/swag/testdata/param_structs"
	`, goparser.ParseComments)
	require.NoError(t, err)

	parser := New()
	parser.Overrides = map[string]string{
		"github.com/swaggo/swag/testdata/param_structs.CSV": "array",
	}
	err = parser.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = parser.packages.ParseTypes()
	require.NoError(t, err)

	o := NewOperation(parser)
	err = o.ParseComment(`@Param model query structs.EnumArrayQueryModel true "q"`, astFile)
	require.NoError(t, err)

	require.Len(t, o.Parameters, 2)
	byName := map[string]*spec.Parameter{}
	for _, pp := range o.Parameters {
		byName[pp.Spec.Spec.Name] = pp.Spec.Spec
	}
	// primitive element resolves to a string array (from T, not a fallback)
	names := byName["names[]"]
	require.NotNil(t, names)
	assert.Equal(t, ARRAY, (*names.Schema.Spec.Type)[0])
	assert.Equal(t, "string", (*names.Schema.Spec.Items.Schema.Spec.Type)[0])

	p := byName["directions[]"]
	require.NotNil(t, p)
	require.NotNil(t, p.Schema.Spec.Type)
	assert.Equal(t, ARRAY, (*p.Schema.Spec.Type)[0])
	items := p.Schema.Spec.Items.Schema
	require.NotNil(t, items, "array items should be present")
	var enum []interface{}
	switch {
	case items.Spec != nil && len(items.Spec.Enum) > 0:
		enum = items.Spec.Enum
	case items.Ref != nil:
		enum = parser.getSchemaByRef(items.Ref).Enum
	}
	assert.ElementsMatch(t, []interface{}{"asc", "desc"}, enum, "array items must carry the element enum")
}

func TestParseParamGenericArrayDirectQuery(t *testing.T) {
	fset := token.NewFileSet()
	astFile, err := goparser.ParseFile(fset, "operationv3_test.go", `package swag
	import structs "github.com/swaggo/swag/testdata/param_structs"
	`, goparser.ParseComments)
	require.NoError(t, err)

	parser := New()
	parser.Overrides = map[string]string{
		"github.com/swaggo/swag/testdata/param_structs.CSV": "array",
	}
	err = parser.parseFile("github.com/swaggo/swag/testdata/param_structs", "testdata/param_structs/structs.go", nil, ParseModels)
	require.NoError(t, err)
	err = parser.packages.ParseTypes()
	require.NoError(t, err)

	o := NewOperation(parser)
	err = o.ParseComment(`@Param ids query structs.CSV[structs.OrderDirection] true "ids"`, astFile)
	require.NoError(t, err)

	// R4: a generic wrapper resolving (via override) to an array is emitted as
	// one array parameter, not silently dropped.
	require.Len(t, o.Parameters, 1)
	p := o.Parameters[0].Spec.Spec
	assert.Equal(t, "ids", p.Name)
	assert.Equal(t, ARRAY, (*p.Schema.Spec.Type)[0])
}
