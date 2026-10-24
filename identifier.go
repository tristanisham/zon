package zon

// Zig's reserved keywords require @"..." when used as field or enum names.
func isKeyword(s string) bool {
	switch s {
	case "addrspace", "align", "allowzero", "and", "anyframe", "anytype", "asm",
		"break", "callconv", "catch", "comptime", "const", "continue", "defer", "else",
		"enum", "errdefer", "error", "export", "extern", "fn", "for", "if", "inline",
		"linksection", "noalias", "noinline", "nosuspend", "opaque", "or", "orelse",
		"packed", "pub", "resume", "return", "struct", "suspend", "switch", "test",
		"threadlocal", "try", "union", "unreachable", "var", "volatile", "while":
		return true
	}
	return false
}
