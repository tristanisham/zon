//! ZON reference oracle. Reads length-prefixed (u32 LE) records from the file
//! given as argv[1]; for each, writes one line to stdout:
//!   OK <canonical>   or   ERR <first error message>
const std = @import("std");
const Io = std.Io;
const Zoir = std.zig.Zoir;
const ZonGen = std.zig.ZonGen;

fn hex(w: *Io.Writer, bytes: []const u8) !void {
    for (bytes) |b| try w.print("{x:0>2}", .{b});
}

fn emit(w: *Io.Writer, gpa: std.mem.Allocator, ast: *const std.zig.Ast, zoir: *const Zoir, idx: Zoir.Node.Index) !void {
    switch (idx.get(zoir)) {
        .true => try w.writeAll("true"),
        .false => try w.writeAll("false"),
        .null => try w.writeAll("null"),
        .pos_inf => try w.writeAll("inf"),
        .neg_inf => try w.writeAll("-inf"),
        .nan => try w.writeAll("nan"),
        .int_literal => |i| switch (i) {
            .small => |v| try w.print("i:{d}", .{v}),
            .big => |b| {
                const s = try b.toStringAlloc(gpa, 10, .lower);
                defer gpa.free(s);
                try w.print("i:{s}", .{s});
            },
        },
        .float_literal => |f| {
            // ZonGen parses float literals as f128. Also print the literal
            // parsed directly as f64 when that differs, because the two can
            // disagree: by double rounding, or by std.fmt.parseFloat(f128, ..)
            // bugs (0.17.0 drops a mantissa digit for e.g. "9_703e283").
            const d: f64 = @floatCast(f);
            try w.print("f:{x:0>16}", .{@as(u64, @bitCast(d))});
            const ast_node = idx.getAstNode(zoir);
            const tok = ast.nodeMainToken(ast_node);
            const neg = ast.nodeTag(ast_node) == .negation;
            const num_tok = if (neg) ast.nodeMainToken(ast.nodeData(ast_node).node) else tok;
            const direct = std.fmt.parseFloat(f64, ast.tokenSlice(num_tok)) catch unreachable;
            const signed = if (neg) -direct else direct;
            if (@as(u64, @bitCast(signed)) != @as(u64, @bitCast(d))) {
                try w.print("|{x:0>16}", .{@as(u64, @bitCast(signed))});
            }
        },
        .char_literal => |c| try w.print("i:{d}", .{c}),
        .enum_literal => |e| {
            try w.writeAll("e:");
            try hex(w, e.get(zoir));
        },
        .string_literal => |s| {
            try w.writeAll("s:");
            try hex(w, s);
        },
        .empty_literal => try w.writeAll("{}"),
        .array_literal => |r| {
            try w.writeAll("[");
            for (0..r.len) |i| {
                if (i > 0) try w.writeAll(",");
                try emit(w, gpa, ast, zoir, r.at(@intCast(i)));
            }
            try w.writeAll("]");
        },
        .struct_literal => |s| {
            try w.writeAll("{");
            for (s.names, 0..) |n, i| {
                if (i > 0) try w.writeAll(",");
                try hex(w, n.get(zoir));
                try w.writeAll("=");
                try emit(w, gpa, ast, zoir, s.vals.at(@intCast(i)));
            }
            try w.writeAll("}");
        },
    }
}

pub fn main(init: std.process.Init) !void {
    const gpa = init.gpa;
    const io = init.io;
    const args = try init.minimal.args.toSlice(init.arena.allocator());
    const data = try Io.Dir.cwd().readFileAlloc(io, args[1], gpa, .limited(1 << 30));
    defer gpa.free(data);

    // Unbuffered per-verdict flushes let the caller tell exactly which case
    // crashed the oracle (some std.zig code paths panic on malformed input).
    var out_buf: [4096]u8 = undefined;
    var out = Io.File.stdout().writerStreaming(io, &out_buf);
    const w = &out.interface;

    var pos: usize = 0;
    while (pos < data.len) {
        const n = std.mem.readInt(u32, data[pos..][0..4], .little);
        pos += 4;
        const src = try gpa.allocSentinel(u8, n, 0);
        defer gpa.free(src);
        @memcpy(src, data[pos..][0..n]);
        pos += n;

        var ast = try std.zig.Ast.parse(gpa, src, .{ .mode = .zon });
        defer ast.deinit(gpa);
        if (ast.errors.len > 0) {
            try w.writeAll("ERR parse: ");
            try ast.renderError(ast.errors[0], w);
            try w.writeAll("\n");
            try w.flush();
            continue;
        }
        var zoir = try ZonGen.generate(gpa, ast, .{});
        defer zoir.deinit(gpa);
        if (zoir.hasCompileErrors()) {
            const e = zoir.compile_errors[0];
            var msg: []const u8 = e.msg.get(&zoir);
            if (std.mem.indexOfScalar(u8, msg, '\n')) |nl| msg = msg[0..nl];
            try w.print("ERR zongen: {s}\n", .{msg});
            try w.flush();
            continue;
        }
        try w.writeAll("OK ");
        try emit(w, gpa, &ast, &zoir, .root);
        try w.writeAll("\n");
        try w.flush();
    }
}
