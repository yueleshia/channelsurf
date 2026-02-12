const std = @import("std");
const util = @import("_parsing.zig");
const constants = @import("_constants.zig");

test "fetch" {
    const allocator = std.testing.allocator;

    var client = std.http.Client{ .io = std.testing.io, .allocator = allocator };
    defer client.deinit();

    for ([_][]const u8{
        "https://youtube.com/@ZIGShowtime/Videos",
        "https://www.youtube.com/@Level1LinksWithFriends/Videos",
    }) |uri| {
        var videos = try fetch(allocator, &client, &.{ .uri = .dupe_slice(uri) });
        for (videos.as_const_slice()) |v| {
            if (false) {
                const a = v;
                a.debug_print();
            }
        }
    }
}

const Output = constants.BoundedArray(constants.Video, constants.MAX_PAGE_SIZE);

// We assume that you are passing valid URIs
pub fn fetch(allocator: std.mem.Allocator, client: *std.http.Client, target: *const constants.ChannelLocation) !Output {
    //const uri = std.Uri {
    //    .scheme = "https",
    //    .host = .{ .raw = "www.youtube.com" },
    //    .path = .{ .percent_encoded = "/@ZIGShowtime" },
    //};
    const channel_uri = std.Uri.parse(target.uri.as_const_slice()) catch @panic("Should have already been parsed on config load");

    // @TODO: optimization choose a suitable max for a youtube video channe list
    // @TODO: 900k for a channel on first load, but the first 600k is not channel video data, worth the increase in complexity?
    var body_writer: std.Io.Writer.Allocating = try .initCapacity(allocator, 2 * 1024 * 1024); // 2 MB
    const cache_id = blk: {
        var id: constants.BoundedArray(u8, 50) = .empty;
        break :blk id.topup_slice_suffix(channel_uri.path.percent_encoded[1..], ".html");
    };
    _ = try util.request_wrapper(client, cache_id, &body_writer, .{
        .location = .{ .uri = channel_uri },
        .headers = .{
            .accept_encoding = .{ .override = "en-US" },
        },
        .extra_headers = &.{
            .{ .name = "Cookie", .value = "PREF=hl=en&gl=US" }
        },
    });
    var body = body_writer.toArrayList();
    defer body.deinit(allocator);

    var i: usize = 0;
    var state: u8 = 0;

    //var capture = 0;
    //const REGEX_INITIAL_DATA = new RegExp("<script.*?var ytInitialData = (.*?);<\/script>");
    const found: bool = while (true) {
        //std.debug.print("Current: state {d} idx {d} {s}\n", .{state, i, body.items[i..i + 50]});

        const offset = switch (state) {
            0 => if (std.mem.find(u8, body.items[i + 1..], "<script")) |j| j + 1 else null,
            1 => if (std.mem.find(u8, body.items[i..], ">")) |j| blk: {
                const s = "var ytInitialData = ";
                const idx = j + ">".len;
                break :blk if (std.mem.startsWith(u8, body.items[i + idx..], s)) idx + s.len else null;
            } else null,
            else => break true,
        };

        if (offset) |j| {
            state += 1;
            i += j;
        } else if (state == 0) {
            break false;
        } else {
            state -= 1;
        }
    };

    if (!found) {
        std.log.debug("Could not find ytInitialData: youtube:{s}\n", .{channel_uri.path.percent_encoded});
        return error.NoYouTubeInit;
    }

    const json_payload = if (std.mem.find(u8, body.items[i..], ";</script>")) |close| body.items[i..i + close] else "";

    // Saving the JSON portion is inexpensive, so always do it in debug to avoid sync issues
    const maybe_cache = util.acquire_cache_file(client.io, .always, blk: {
        var id: constants.BoundedArray(u8, 50) = .empty;
        break :blk id.topup_slice_suffix(channel_uri.path.percent_encoded[1..], ".json");
    }) catch null;

    if (maybe_cache) |x| {
        const fh, _ = x;
        var buffer: [4096]u8 = undefined;
        var writer = fh.writer(client.io, &buffer);
        try writer.interface.writeAll(json_payload);
    }

    return try parse(allocator, target.*, json_payload);
}

// Use the std lib JSON scan API + Automata so that we are more immune to
// schema changes. We are still reliant on field names and the  hierarchy
// nesting remaining the same, but we will be immune to fields being
// embedded deeper/shallower. Also this is less memory than fully
// deserialising JSON.
fn parse(allocator: std.mem.Allocator, channel: constants.ChannelLocation, json_payload: []const u8) !Output {
    var videos: Output = .empty;
    // @TODO: Should be able to perform without dynamic allocation if we place upperlimit on JSON depth
    var scanner = std.json.Scanner.initCompleteInput(allocator, json_payload);
    defer scanner.deinit();

    var depth: u8 = 0;
    var i: i32 = 0;

    var parser1 = Parser1.init();
    var parser2 = Parser2.init();

    while (i < std.math.maxInt(@TypeOf(i)) - 1) : (i += 1) {
        const token = scanner.next() catch |err| switch (err) {
            error.BufferUnderrun => unreachable,
            error.OutOfMemory => |e| return e,
            error.SyntaxError => |e| return e,
            error.UnexpectedEndOfInput => |e| return e,
        };
        switch (token) {
            .object_begin => depth += 1,
            .array_begin => depth += 1,
            .object_end => depth -= 1,
            .array_end => depth -= 1,
            .end_of_document => break,
            else => {},
        }

        if (parser1.next(i, depth, token)) |video| {
            var ret = video;
            ret.channel = channel;
            videos.topup(ret) catch {};
            std.debug.assert(videos.len != videos.items.len);
        }
        if (parser2.next(i, depth, token)) |video| {
            var ret = video;
            ret.channel = channel;
            videos.topup(ret) catch {};
            std.debug.assert(videos.len != videos.items.len);
        }
    }
    return videos;
}

const Parser1 = struct {
    parent: util.Automaton,
    index_since_reset: u32,
    automata: std.enums.EnumMap(Asdf, util.Automaton) = .init(.{
        .video_id = .init(&.{ .{ .string = "videoId" } }),
        .duration = .init(&.{ .{ .string = "thumbnailOverlayTimeStatusRenderer" }, .{ .object_begin = {} }, .{ .string = "simpleText" } }),
        .title = .init(&.{ .{ .string = "title" }, .{ .object_begin = {} }, .{ .string = "simpleText" } }),
        .published = .init(&.{ .{ .string = "publishedTimeText" }, .{ .object_begin = {} }, .{ .string = "simpleText" } }),
        //.description = .init(&.{ .{ .string = "description" }, .{ .object_begin = {} }, .{ .string = "runs" } }),
    }),
    video: constants.Video,

    const Asdf = enum { video_id, duration, title, published };

    inline fn init() @This() {
        return .{
            .parent = util.Automaton.init(&.{
                .{ .string = "content" },
                .{ .object_begin = {} },
                .{ .string = "gridVideoRenderer" },
                .{ .object_begin = {} },
            }),
            .index_since_reset = 2, // Any value > 1 so our if condition does not trigger
            .video = .empty,
        };
    }
    fn next(self: *@This(), index: i32, depth: u8, token: std.json.Token) ?constants.Video {
        self.parent.next(index, depth, token);

        if (self.parent.is_child) {
            self.index_since_reset = 0;
            inline for (@typeInfo(Asdf).@"enum".fields) |f| {
                const key: Asdf = @enumFromInt(f.value);
                const a = self.automata.getPtrAssertContains(key);
                _ = a.next(index, depth, token);
                switch (key) {
                    .video_id => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| {
                                _ = self.video.uri.topup_slice("https://youtu.be/");
                                _ = self.video.uri.topup_slice(x);
                            },
                            else => {},
                        }
                    },
                    .title => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| _ = self.video.title.topup_slice(x),
                            else => {},
                        }
                    },
                    .duration => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| self.video.duration = util.parse_duration(x),
                            else => {},
                        }
                    },
                    .published => if (a.is_next_to_last(index)) {
                        //switch (token) {
                        //    .string => |x| std.debug.print("{t}: {s}\n", .{k, x}),
                        //    else => {},
                        //}
                    },
                }
            }
            switch (token) {
                .string => |s| std.debug.assert(!std.mem.eql(u8, s, "visibleItemCount")),
                else => {},
            }
        } else {
            self.index_since_reset += 1;
        }

        if (self.index_since_reset == 1) {
            const video: constants.Video = self.video;
            inline for (@typeInfo(Asdf).@"enum".fields) |f| {
                const key: Asdf = @enumFromInt(f.value);
                self.automata.getPtrAssertContains(key).reset();
            }
            self.video = .empty;
            return video;
        }
        return null;
    }
};

const Parser2 = struct {
    parent: util.Automaton,
    index_since_reset: u32,
    automata: std.enums.EnumMap(Asdf, util.Automaton) = .init(.{
        .video_id = .init(&.{ .{ .string = "videoId" } }),
        .duration = .init(&.{ .{ .string = "thumbnailOverlayTimeStatusRenderer" }, .{ .object_begin = {} }, .{ .string = "simpleText" } }),
        .title = .init(&.{ .{ .string = "title" }, .{ .object_begin = {} }, .{ .string = "text" } }),
        .published = .init(&.{ .{ .string = "publishedTimeText" }, .{ .object_begin = {} }, .{ .string = "simpleText" } }),
        //.description = .init(&.{ .{ .string = "description" }, .{ .object_begin = {} }, .{ .string = "runs" } }),
    }),
    video: constants.Video,

    const Asdf = enum { video_id, duration, title, published };

    inline fn init() @This() {
        return .{
            .parent = util.Automaton.init(&.{
                .{ .string = "content" },
                .{ .object_begin = {} },
                .{ .string = "videoRenderer" },
                .{ .object_begin = {} },
            }),
            .index_since_reset = 2, // Any value > 1 so our if condition does not trigger
            .video = .empty,
        };
    }
    fn next(self: *@This(), index: i32, depth: u8, token: std.json.Token) ?constants.Video {
        self.parent.next(index, depth, token);

        if (self.parent.is_child) {
            self.index_since_reset = 0;
            inline for (@typeInfo(Asdf).@"enum".fields) |f| {
                const key: Asdf = @enumFromInt(f.value);
                const a = self.automata.getPtrAssertContains(key);
                _ = a.next(index, depth, token);
                switch (key) {
                    .video_id => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| {
                                _ = self.video.uri.topup_slice("https://youtu.be/");
                                _ = self.video.uri.topup_slice(x);
                            },
                            else => {},
                        }
                    },
                    .title => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| _ = self.video.title.topup_slice(x),
                            else => {},
                        }
                    },
                    .duration => if (a.is_next_to_last(index)) {
                        switch (token) {
                            .string => |x| self.video.duration = util.parse_duration(x),
                            else => {},
                        }
                    },
                    .published => if (a.is_next_to_last(index)) {
                        //switch (token) {
                        //    .string => |x| std.debug.print("{t}: {s}\n", .{k, x}),
                        //    else => {},
                        //}
                    },
                }
            }
            switch (token) {
                .string => |s| std.debug.assert(!std.mem.eql(u8, s, "visibleItemCount")),
                else => {},
            }
        } else {
            self.index_since_reset += 1;
        }

        if (self.index_since_reset == 1) {
            const video: constants.Video = self.video;
            inline for (@typeInfo(Asdf).@"enum".fields) |f| {
                const key: Asdf = @enumFromInt(f.value);
                self.automata.getPtrAssertContains(key).reset();
            }
            self.video = .empty;
            return video;
        }
        return null;
    }
};
