package zon

// node is a parsed ZON value. The concrete types below form a closed set.
type node interface {
	isNode()
}

// structNode is an anonymous struct: .{ .a = 1, .b = 2 }. Fields are kept in
// source order so encoding is deterministic.
type structNode struct {
	fields []field
}

type field struct {
	name  string
	value node
}

// tupleNode is an anonymous tuple/array: .{ 1, 2, 3 }. An empty aggregate
// .{} is represented as an empty tupleNode.
type tupleNode struct {
	items []node
}

type stringNode struct {
	value string
}

// enumNode is an enum literal such as .debug; name excludes the leading dot.
type enumNode struct {
	name string
}

// numberNode keeps the raw literal text so it can be parsed against the target
// Go type without intermediate precision loss. isFloat records whether the
// lexer scanned it as a floating-point literal.
type numberNode struct {
	raw     string
	isFloat bool
}

type boolNode struct {
	value bool
}

type nullNode struct{}

// isNode implements the node interface for structNode.
func (structNode) isNode() {}

// isNode implements the node interface for tupleNode.
func (tupleNode) isNode() {}

// isNode implements the node interface for stringNode.
func (stringNode) isNode() {}

// isNode implements the node interface for enumNode.
func (enumNode) isNode() {}

// isNode implements the node interface for numberNode.
func (numberNode) isNode() {}

// isNode implements the node interface for boolNode.
func (boolNode) isNode() {}

// isNode implements the node interface for nullNode.
func (nullNode) isNode() {}
