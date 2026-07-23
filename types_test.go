package swag

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

func typeSpecDef(pkgPath, name string) *TypeSpecDef {
	return &TypeSpecDef{
		PkgPath:  pkgPath,
		TypeSpec: &ast.TypeSpec{Name: &ast.Ident{Name: name}},
	}
}

func TestGenericBaseFullPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pkgPath string
		typName string
		want    string
	}{
		{
			name:    "parametrized generic yields base full path",
			pkgPath: "core/packages/shared/go/sharedgin",
			typName: "$sharedgin.CommaArray-card_CardStatus",
			want:    "core/packages/shared/go/sharedgin.CommaArray",
		},
		{
			name:    "generic without the synthetic prefix",
			pkgPath: "core/pkg/pagination",
			typName: "pagination.Page-dto_RiskLevel",
			want:    "core/pkg/pagination.Page",
		},
		{
			name:    "non-generic type yields empty",
			pkgPath: "core/pkg/card",
			typName: "CardStatus",
			want:    "",
		},
		{
			name:    "array synthetic type is not a generic instantiation",
			pkgPath: "core/pkg/card",
			typName: "$array_Card",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, typeSpecDef(tt.pkgPath, tt.typName).GenericBaseFullPath())
		})
	}
}

func TestGenericBaseFullPathNilName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", (&TypeSpecDef{PkgPath: "x", TypeSpec: &ast.TypeSpec{}}).GenericBaseFullPath())
}
