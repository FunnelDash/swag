package swag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInterfaceOneOf(t *testing.T) {
	t.Parallel()

	p := New(GenerateOpenAPI3Doc(true))
	require.NoError(t, p.ParseAPI("testdata/interface_oneof", "main.go", defaultParseDepth))

	schemas := p.openAPI.Components.Schemas
	require.NotNil(t, schemas)

	t.Run("the interface becomes a named oneOf with a discriminator", func(t *testing.T) {
		proxy := schemas.GetOrZero("api.RewardRule")
		require.NotNil(t, proxy, "api.RewardRule must be its own component")

		rule := proxy.Schema()
		require.NotNil(t, rule)
		require.Len(t, rule.OneOf, 2)
		// Members are ordered by type name, not by declaration: they are
		// collected from wherever they declare themselves, so a name sort is
		// what keeps the rendered spec stable run to run.
		assert.Equal(t, "#/components/schemas/api.FlatAmountRule", rule.OneOf[0].GetReference())
		assert.Equal(t, "#/components/schemas/api.QualifiedSpendRule", rule.OneOf[1].GetReference())

		require.NotNil(t, rule.Discriminator)
		assert.Equal(t, "rewardsType", rule.Discriminator.PropertyName)
		assert.Equal(t, "#/components/schemas/api.QualifiedSpendRule",
			rule.Discriminator.Mapping.GetOrZero("qualified-spend-percentage"))
		assert.Equal(t, "#/components/schemas/api.FlatAmountRule",
			rule.Discriminator.Mapping.GetOrZero("flat-amount"))

		assert.Empty(t, rule.Type, "a oneOf carries no type of its own")
	})

	t.Run("implementations reachable only through the interface are registered", func(t *testing.T) {
		// Nothing else references either type, so without the union driving
		// their registration the refs above would dangle.
		assert.NotNil(t, schemas.GetOrZero("api.QualifiedSpendRule"))
		assert.NotNil(t, schemas.GetOrZero("api.FlatAmountRule"))
	})

	t.Run("a field of the interface type refs the union", func(t *testing.T) {
		contract := schemas.GetOrZero("api.Contract").Schema()
		require.NotNil(t, contract)

		rules := contract.Properties.GetOrZero("rules").Schema()
		require.NotNil(t, rules)
		assert.Equal(t, []string{ARRAY}, rules.Type)
		require.NotNil(t, rules.Items)
		assert.Equal(t, "#/components/schemas/api.RewardRule", rules.Items.A.GetReference(),
			"the array element must not inline the union")
	})

	t.Run("an interface without the annotation is unchanged", func(t *testing.T) {
		contract := schemas.GetOrZero("api.Contract").Schema()
		anything := contract.Properties.GetOrZero("anything").Schema()
		require.NotNil(t, anything)

		assert.Empty(t, anything.OneOf)
		assert.Nil(t, anything.Discriminator)
		assert.Nil(t, schemas.GetOrZero("api.Untagged"))
	})
}

func TestParseInterfaceOneOfMissingDiscriminatorValue(t *testing.T) {
	t.Parallel()

	// Emitting a discriminator with an incomplete mapping would leave clients
	// unable to resolve the missing variant, so this has to fail loudly.
	p := New(GenerateOpenAPI3Doc(true))
	err := p.ParseAPI("testdata/interface_oneof_missing_value", "main.go", defaultParseDepth)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "@discriminatorValue")
	assert.Contains(t, err.Error(), "Circle")
}

func TestUnionMembers(t *testing.T) {
	t.Parallel()

	t.Run("an unresolvable @oneOfMember target errors", func(t *testing.T) {
		p := New(GenerateOpenAPI3Doc(true))
		err := p.ParseAPI("testdata/interface_oneof_bad_member", "main.go", defaultParseDepth)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "@oneOfMember")
	})
}
