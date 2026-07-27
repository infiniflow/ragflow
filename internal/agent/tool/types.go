// Package tool — Types and interfaces for RAGFlow agent tools.
//
// Tool interface and types for the agent component.
// package has zero external dependencies beyond stdlib.
package tool

import "context"

// Tool is the interface all RAGFlow agent tools implement.
type Tool interface {
	ToolMeta() ToolMeta
	InvokableRun(ctx context.Context, argsJSON string) (string, error)
}

// ToolMeta holds the metadata for a tool.
type ToolMeta struct {
	Name        string
	Description string
	Parameters  map[string]ParameterInfo
}

// ParameterInfo describes a single parameter of a tool's input schema.
type ParameterInfo struct {
	Type        string
	Description string
	Required    bool
	Enum        []string
}

const (
	ParamTypeString  = "string"
	ParamTypeInteger = "integer"
	ParamTypeNumber  = "number"
	ParamTypeBoolean = "boolean"
	ParamTypeArray   = "array"
	ParamTypeObject  = "object"
)
