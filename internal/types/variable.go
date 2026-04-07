package types

import "github.com/zclconf/go-cty/cty"

// VariableDecl represents a parsed variable declaration block.
type VariableDecl struct {
	Name          string
	Type          string     // "string", "number", "bool", "enum"
	CtyType       cty.Type   // the underlying cty type for the EvalContext
	Default       *cty.Value // nil if no default
	AllowedValues []string   // populated when type is enum[...]
	Required      bool       // true if no default is set
}
