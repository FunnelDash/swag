package swag

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
)

const (
	oneOfMemberAttr        = "@oneofmember"
	discriminatorAttr      = "@discriminator"
	discriminatorValueAttr = "@discriminatorvalue"
)

// unionMemberIndex maps a union interface to the types declaring membership of
// it, sorted by name so the rendered oneOf is stable across runs.
type unionMemberIndex map[*TypeSpecDef][]*TypeSpecDef

// typeAttr reads the single-argument attribute attr from a type's doc comment,
// checking both the TypeSpec and its enclosing GenDecl.
func typeAttr(typeSpecDef *TypeSpecDef, attr string) string {
	if typeSpecDef == nil || typeSpecDef.TypeSpec == nil {
		return ""
	}

	for _, doc := range []*ast.CommentGroup{typeSpecDef.TypeSpec.Doc, genDeclDoc(typeSpecDef)} {
		if doc == nil {
			continue
		}

		for _, comment := range doc.List {
			fields := strings.Fields(strings.TrimSpace(strings.TrimLeft(comment.Text, "/")))
			if len(fields) >= 2 && strings.ToLower(fields[0]) == attr {
				return fields[1]
			}
		}
	}

	return ""
}

// genDeclDoc returns the doc comment of the declaration a type was declared
// in. A standalone `type X ...` carries its comment there rather than on the
// TypeSpec, and ParentSpec is not it, so the file is walked for the match.
func genDeclDoc(typeSpecDef *TypeSpecDef) *ast.CommentGroup {
	if typeSpecDef.File == nil {
		return nil
	}

	for _, decl := range typeSpecDef.File.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			if spec == ast.Spec(typeSpecDef.TypeSpec) {
				return genDecl.Doc
			}
		}
	}

	return nil
}

// unionMembers indexes every `@oneOfMember` declaration in the parsed packages.
//
// Membership is declared by the implementation, the way Go declares it with
// `var _ Iface = (*T)(nil)`: adding a variant touches only the variant, and
// the interface does not carry a list to keep in sync.
func (p *Parser) unionMembers() (unionMemberIndex, error) {
	if p.unionMemberIndex != nil {
		return p.unionMemberIndex, nil
	}

	index := unionMemberIndex{}
	names := make([]string, 0, len(p.packages.uniqueDefinitions))
	for name := range p.packages.uniqueDefinitions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		member := p.packages.uniqueDefinitions[name]
		if member == nil {
			continue
		}

		declared := typeAttr(member, oneOfMemberAttr)
		if declared == "" {
			continue
		}

		iface := p.packages.FindTypeSpec(declared, member.File)
		if iface == nil {
			return nil, fmt.Errorf("%s declares @oneOfMember %q, which does not resolve",
				member.TypeName(), declared)
		}
		if _, ok := iface.TypeSpec.Type.(*ast.InterfaceType); !ok {
			return nil, fmt.Errorf("%s declares @oneOfMember %q, which is not an interface",
				member.TypeName(), declared)
		}

		index[iface] = append(index[iface], member)
	}

	p.unionMemberIndex = index

	return index, nil
}

// applyUnion turns an interface's declared members into oneOf plus an optional
// discriminator, registering each member as its own component.
//
// Resolving through getTypeSchema with a ref is what registers them: a member
// reachable only through the interface is never otherwise walked, so a bare
// $ref would point at nothing.
func (p *Parser) applyUnion(def *base.Schema, iface *TypeSpecDef, members []*TypeSpecDef) error {
	property := typeAttr(iface, discriminatorAttr)

	refs := make([]*base.SchemaProxy, 0, len(members))
	mapping := orderedmap.New[string, string]()

	for _, member := range members {
		schema, err := p.getTypeSchema(member.TypeName(), iface.File, true)
		if err != nil {
			return fmt.Errorf("@oneOfMember %s: %w", member.TypeName(), err)
		}
		refs = append(refs, schema)

		if property == "" {
			continue
		}

		value := typeAttr(member, discriminatorValueAttr)
		if value == "" {
			return fmt.Errorf("%s needs a @discriminatorValue, since %s declares a @discriminator",
				member.TypeName(), iface.TypeName())
		}
		mapping.Set(value, schema.GetReference())
	}

	// An interface is an object of one of its members, never a scalar; leaving
	// Type unset alongside oneOf is what OpenAPI 3.1 expects.
	def.Type = nil
	def.OneOf = refs

	if property != "" {
		def.Discriminator = &base.Discriminator{PropertyName: property, Mapping: mapping}
	}

	return nil
}
