const std = @import("std");
const constants = @import("_constants.zig");

// @TODO: Rework to use io.async() for network requests
//        see https://github.com/ziglang/zig/pull/25592

// depending on `RUN_LEVEL`, will use local files
pub fn request_wrapper(client: *std.http.Client, cache_id: []const u8, output: *std.Io.Writer.Allocating, options: std.http.Client.FetchOptions) !std.http.Client.FetchResult {
    var opts = options;
    std.debug.assert(opts.response_writer == null);
    opts.response_writer = &output.writer;

    const maybe_cache = try acquire_cache_file(client.io, .auto, cache_id);

    const maybe_fh = if (maybe_cache) |x| blk: {
        const fh, const is_write = x;

        if (is_write) {
            break :blk fh;
        } else {
            var buffer: [4096]u8 = undefined;
            var reader = fh.reader(client.io, &buffer);
            const result = output.writer.sendFileAll(&reader, .limited(256 * 1024 * 1024)); // 256 MB max
            fh.close(client.io);
            _ = try result;
            return .{ .status = .ok };
        }
        //return 0;
    } else null;

    std.log.debug("Fetching youtube:{s}", .{opts.location.uri.path.percent_encoded});
    const status = try client.fetch(opts);
    if (maybe_fh) |fh| {
        std.log.debug("Saving to cache file", .{});
        var buffer: [4096]u8 = undefined;
        var writer = fh.writer(client.io, &buffer);
        try writer.interface.writeAll(output.writer.buffer[0..output.writer.end]);
    }
    return status;
}

// Optionally create a file in <build_zig_dir>/tmp/<cache_id> depending on constants.RUN_LEVEL
pub fn acquire_cache_file(io: std.Io, replace: enum { auto, always }, unsanitised_cache_id: []const u8) !?struct{ std.Io.File, bool } {
    const cache_id = blk: {
        var id: constants.BoundedArray(u8, 50) = .empty;
        _ = id.topup_slice(unsanitised_cache_id);
        std.mem.replaceScalar(u8, id.as_mut_slice(), '/', '-');
        std.mem.replaceScalar(u8, id.as_mut_slice(), '\\', '-');
        break :blk id.as_const_slice();
    };

    const is_read, const is_write = switch (constants.RUN_LEVEL) {
        .regular => .{ false, false},
        .cache => .{ true, replace == .always },
        .cache_replace => .{ true, true},
    };

    var fba = blk: {
        var buffer: [std.Io.Dir.max_path_bytes * 4]u8 = undefined;
        break :blk std.heap.FixedBufferAllocator.init(&buffer);
    };
    const allocator = fba.allocator();

    const tmp_dir_buffer = allocator.alloc(u8, std.Io.Dir.max_path_bytes) catch unreachable;
    defer allocator.free(tmp_dir_buffer);

    if (is_read) {
        const root = std.Io.Dir.cwd();

        // Find root directory defined by presence of build.zig
        tmp_dir_buffer[0] = '.';
        var tmp_dir_path: []const u8 = tmp_dir_buffer[0..1];

        var i: u16 = 0;
        while (true) : (i += 1) {
            if (i > 100) {
                std.log.warn("Your project is within too many folders or does not have a build.zig in its parents", .{});
                return null;
            }

            // path 2
            const build_zig = std.fs.path.join(allocator, &.{tmp_dir_path, "build.zig"}) catch |e| switch (e) {
                error.OutOfMemory => unreachable,
            };
            if (root.statFile(io, build_zig, .{ .follow_symlinks = true })) |_| {
                allocator.free(build_zig);
                break;
            } else |err| {
                allocator.free(build_zig);
                switch (err) {
                    error.FileNotFound => {
                        // path 2
                        const dir = std.fs.path.join(allocator, &.{tmp_dir_path, ".."}) catch |e| switch (e) {
                            error.OutOfMemory => unreachable,
                        };
                        defer allocator.free(dir);

                        @memcpy(tmp_dir_buffer[0..dir.len], dir);
                        tmp_dir_path = tmp_dir_buffer[0..dir.len];
                    },
                    else => {
                        std.log.warn("Could not find the build.zig", .{});
                        return null;
                    },
                }
            }
        }

        //path 3
        tmp_dir_path = std.fs.path.join(allocator, &.{tmp_dir_path, "tmp"}) catch |e| switch (e) {
            error.OutOfMemory => unreachable,
        };
        defer allocator.free(tmp_dir_path);

        const tmp_dir = root.createDirPathOpen(io, tmp_dir_path, .{
            .open_options = .{
                .access_sub_paths = true,
                .iterate = false,
                .follow_symlinks = true,
            },
        }) catch {
            std.log.warn("Failed to read tmp dir: {s}", .{tmp_dir_path});
            return null;
        };
        defer tmp_dir.close(io);

        //path 4
        const file_path = std.fs.path.join(allocator, &.{tmp_dir_path, cache_id}) catch |e| switch (e) {
            error.OutOfMemory => unreachable,
        };
        defer allocator.free(file_path);

        if (!is_write) {
            std.log.debug("Read file '<tmp>/{s}'", .{cache_id});
            if (tmp_dir.openFile(io, cache_id, .{})) |fh| {
                return .{fh, false};
            } else |err| {
                std.log.warn("Failed to read '<tmp>/{s}'", .{cache_id});
                switch (err) {
                    error.FileNotFound => {},
                    else => |e| return e,
                }
            }
        }
        std.log.debug("Creating file '<tmp>/{s}'", .{cache_id});
        return .{try tmp_dir.createFile(io, cache_id, .{ .truncate = true }), true};
    }
    return null;
}

// DFA for JSON. This essentially replicates CSS selectors, but for JSON.
// We want a relatively non-brittle way to drill down through a JSON object as
// we are protecting against platforms changing their UI data model.
//
// With most modern JS frameworks, UI state data is serialized as JSON and sent
// either on first load (or the first request following the routing in app.js).
pub const Automaton = struct {
    idx: Depth = 0,
    fsm: []State,
    is_child: bool = false,

    const Depth = u8;
    const Index = i32;
    const State = struct {
        token: std.json.Token,
        depth: Depth,
        index: Index = -2,
    };

    pub inline fn init(items: []const std.json.Token) @This() {
        // @TODO: this might bite us because this generally not to make a mutable comptime-slice
        var buffer: [items.len]State = undefined;
        inline for (items, &buffer) |s, *t| t.token = s;
        const fsm = buffer;

        std.debug.assert(items.len > 0);
        std.debug.assert(items[0] != .array_begin and items[0] != .object_begin);
        std.debug.assert(items.len < std.math.maxInt(Depth));
        return .{ .fsm = @constCast(&fsm) };
    }

    pub fn reset(self: *@This()) void {
        self.idx = 0;
        self.is_child = false;
    }

    pub fn next(self: *@This(), index: Index, depth: Depth, token: std.json.Token) void {
        const filter = false and std.mem.eql(u8, self.fsm[0].token.string, "title");
        if (filter) {
            switch (token) {
                .string => |t| std.debug.print("{d} {d} {s}\n", .{index, self.idx, t}),
                .object_begin => std.debug.print("{d} {d} {{\n", .{index, self.idx}),
                else => {},
            }
        }

        // The last state does not require us to enter/exit a {} or []
        const before_last_state = self.fsm[0..@min(self.idx, self.fsm.len - 1)];
        for (0.., before_last_state) |i, *state| {
            if (depth < state.depth) {
                self.idx = @truncate(i);
                break;
            }
        }

        if (self.idx < self.fsm.len) {
            const curr = &self.fsm[self.idx];
            switch (curr.token) {
                inline .string => |s| switch (token) {
                    .string => |t| if (std.mem.eql(u8, s, t)) {
                        // + 1 to force us to enter/exit a {} or []
                        curr.depth = depth;
                        curr.index = index;
                        self.idx += 1;
                    },
                    else => {}
                },
                inline .object_begin => {
                    const prev = self.fsm[self.idx - 1];
                    if (prev.index + 1 == index) {
                        curr.depth = depth;
                        curr.index = index;
                        if (token == .object_begin) self.idx += 1 else self.idx -= 1;
                    }
                },
                inline else => unreachable,
            }
        }
        self.is_child = self.idx >= self.fsm.len;
    }

    pub fn is_next_to_last(self: *const @This(), index: Index) bool {
        return index == self.fsm[self.fsm.len - 1].index + 1;
    }
};

test "automata" {
    const Pair = struct { Automaton.Depth, std.json.Token };
    {
        var automaton = Automaton.init(&.{
            .{ .string = "thumbnailOverlayTimeStatusRenderer" },
            .{ .string = "simpleText" },
        });
        const output = for (0.., [_]Pair{
            .{ 1, .{ .string = "thumbnailOverlayTimeStatusRenderer" } },
            .{ 5, .{ .string = "videoItem" } },
            .{ 0, .{ .string = "thumbnailOverlayTimeStatusRenderer" } },
            .{ 8, .{ .string = "simpleText" } },
        }) |i, x| {
            automaton.next(0, x[0], x[1]);
            if (automaton.is_child) break i;
        } else 0;
        try std.testing.expectEqual(3, output);
        automaton.next(0, 5, .{ .string = "simpleText" });
        try std.testing.expectEqual(true, automaton.is_child);
    }
}

pub const Cell = union (enum) {
    numeric: void,
    literal: u8,
};
pub fn validate_dfa(str: []const u8, cells: []const Cell) bool {
    if (str.len != cells.len) return false;

    var ret = true;
    for (str, cells) |ch, cell| {
        switch (cell) {
            .numeric => ret = ret and std.ascii.isDigit(ch),
            .literal => |c| ret = ret and ch == c,
        }
    }
    return ret;
}

test "validator" {
    try std.testing.expect(validate_dfa("55:55", &.{ .numeric, .numeric, .{ .literal = ':' }, .numeric,.numeric}));
}

pub fn parse_duration(a: []const u8) std.Io.Duration {
    var iter = std.mem.splitBackwardsScalar(u8, a, ':');
    const asdf = [_]u16 {
        1, // seconds
        60, // minutes
        60 * 60, // hours
    };
    var i: u8 = 0;
    var seconds: i64 = 0;
    while (iter.next()) |section| {
        const value_in_seconds = if (i < asdf.len) asdf[i] else {
            std.log.debug("Duration parsing failed. Too many colon sections in '{s}'", .{a});
            return .{ .nanoseconds = 0 };
        };

        var position: i32 = 1;
        var parse_int: i32 = 0;
        std.debug.assert(section.len < @typeInfo(@TypeOf(position)).int.bits);
        for (1..section.len + 1) |j| {
            const char = section[section.len - j];
            const digit = std.fmt.charToDigit(char, 10) catch {
                std.log.debug("Duration parsing failed. Invalid character {c} in '{s}'", .{char, a});
                return .{ .nanoseconds = 0 };
            };
            parse_int += position * digit;
            position *= 10;
        }
        seconds += parse_int * value_in_seconds;
        i += 1;
    }
    return .fromSeconds(seconds);
}

test "parse_duration" {
    try std.testing.expectEqual(parse_duration("55"), std.Io.Duration.fromSeconds(55));
    try std.testing.expectEqual(parse_duration("19:23").nanoseconds, std.Io.Duration.fromSeconds(19 * 60 + 23).nanoseconds);
    try std.testing.expectEqual(parse_duration("1:19:23").nanoseconds, std.Io.Duration.fromSeconds(1 * 3600 + 19 * 60 + 23).nanoseconds);
}

pub fn comptime_replace(input: []const u8, match: []const u8, replacement: []const u8) []const u8 {
    comptime {
        std.debug.assert(match.len != 0);
        const diff = @as(comptime_int, replacement.len) - @as(comptime_int, match.len);
        var ret: [input.len * (1 + @max(diff, 0))]u8 = undefined;
        var idx = 0;

        for (0.., input) |i, ch| {
            // using mem.startsWith seems to increases our EvalBranchQuota by more than needed
            var starts_with = true;
            for (0.., match) |j, c| {
                starts_with = starts_with and input[i + j] == c;
            }
            if (starts_with) {
                @memcpy(ret[idx..idx + replacement.len], replacement);
                idx += replacement.len;
            } else {
                ret[idx] = ch;
                idx += 1;
            }
        }
        return ret[0..idx] ++ "";
    }
}

test "comptime_replace" {
    try std.testing.expectEqualStrings("a", comptime comptime_replace("ab", "b", ""));
    try std.testing.expectEqualStrings("accc", comptime comptime_replace("abbb", "b", "c"));
    try std.testing.expectEqualStrings("acdcdcd", comptime comptime_replace("abbb", "b", "cd"));
}

