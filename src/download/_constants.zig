const std = @import("std");
const builtin = @import("builtin");

pub const RUN_LEVEL: enum { regular, cache, cache_replace } = if (builtin.mode == .Debug) .cache else .regular;

pub const MAX_FOLLOW_COUNT = 1024;

const Metadata = struct {
    page_size: comptime_int,
};
pub const metadata = std.enums.EnumMap(Service, Metadata).init(.{
    .twitch = Metadata{
        .page_size = 21,
    },
    .youtube = Metadata{
        .page_size = 31,
    },
});
pub const MAX_PAGE_SIZE = blk: {
    var ret = 0;
    for (std.meta.fields(Service)) |x| {
        ret = @max(ret, metadata.getAssertContains(@enumFromInt(x.value)).page_size);
    }
    break :blk ret;
};

pub const Service = enum {
    youtube,
    twitch,

    const providers = [_]struct{[]const u8, Service}{
        .{"www.youtube.com", .youtube},
        .{"youtube.com", .youtube},
        .{"youtu.be", .youtube},
        .{"www.twitch.tv", .twitch},
        .{"twitch.tv", .twitch},
    };
    const valid_providers = blk: {
        var ret: []const u8 = "";
        for (providers) |x| {
            ret = ret ++ "\n* " ++ x[0];
        }
        break :blk ret;
    };
};


pub const ChannelLocation = struct {
    /// Display name
    alias: BoundedArray(u8, 32) = .empty,

    service: Service = .youtube,

    /// For network requests
    channel: BoundedArray(u8, 32) = .empty,

    /// For cache ids
    /// https://www.youtube.com/watch?v=<id> is 43
    uri: BoundedArray(u8, 64) = .empty,

    pub fn from_uri(link: []const u8) !ChannelLocation {
        const raw: @FieldType(ChannelLocation, "uri") = .dupe_slice(link);
        if (link.len > raw.items.len) {
            std.log.warn("We do not support URLs > {d} length", .{raw.items.len});
            return error.ParseURIi;
        }
        const uri = std.Uri.parse(raw.as_const_slice()) catch {
            std.log.warn("Could not parse '{s}' as a URL", .{link});
            return error.ParseURI;
        };

        const host = uri.host orelse {
            std.log.warn("There is no host in the URL {s}. We need this to determine the service provider.", .{link});
            return error.ParseService;
        };

        for (Service.providers) |x| {
            if (std.mem.eql(u8, host.percent_encoded, x[0])) {
                return .{
                    .uri = raw,
                    .service = x[1],
                    .alias = .empty,
                    .channel = switch (x[1]) {
                        .youtube => .empty,
                        // @TODO: understand when zig uri parses to raw vs percent_encoded
                        // @TODO: validate channel name in path
                        .twitch => if (std.mem.countScalar(u8, uri.path.percent_encoded, '/') > 1) {
                            std.log.err("@TODO: invalid url for '{s}'\n", .{link});
                            return error.ParseTwitchUrl;
                        } else .dupe_slice(uri.path.percent_encoded[1..]),
                    },
                };
            }
        }

        std.log.err("We do not recognise the host {s}\nValid providers are:{s}", .{host.percent_encoded, Service.valid_providers});
        return error.UnknownService;
    }
};

test "from_uri" {
    try std.testing.expectEqual(.youtube, (try ChannelLocation.from_uri("https://www.youtube.com/@ZIGShowtime")).service);
    try std.testing.expectEqual(.youtube, (try ChannelLocation.from_uri("https://youtu.be/@ZIGShowtime")).service);
    try std.testing.expectEqual(.twitch, (try ChannelLocation.from_uri("https://twitch.tv/kristoff_it")).service);
}

////////////////////////////////////////////////////////////////////////////////
const Chapter = struct {
    name: BoundedArray(u8, 64),
    time: std.Io.Timestamp,
};

// This is directly serialisable
pub const Video = struct {
    /// Used as the cache id
    channel: ChannelLocation = .{},

    /// YouTube limit is 100
    /// Twitch limit is 70 before elipsis
    title: BoundedArray(u8, 128) = .empty,

    //thumbnail_URL: []const []const u8,
    start_time: std.Io.Timestamp = .{ .nanoseconds = 0 },
    duration: std.Io.Duration = .{ .nanoseconds = 0 },
    is_live: bool = false,

    uri: BoundedArray(u8, 64),

    // Youtube formatting limits to 100
    chapters: BoundedArray(Chapter, 128) = .empty,

    pub const empty = @This(){
        .uri = .empty,
    };

    pub fn debug_print(self: *const @This()) void {
        std.debug.print("{s} | {s} | {s} | {s}\n", .{
            self.uri.as_const_slice(),
            self.channel.alias.as_const_slice(),
            if (self.is_live) "○" else "??:??:??",
            self.title.as_const_slice(),
        });
    }
};

pub fn BoundedArray(T: type, n: comptime_int) type {
    return struct {
        items: [n]T,
        len: u16,

        pub const empty = @This(){
            .items = undefined,
            .len = 0,
        };

        pub fn topup(self: *@This(), x: T) !void {
            if (self.len < self.items.len) {
                self.items[self.len] = x;
                self.len += 1;
            } else return error.OutOfMemory;
        }
        pub fn topup_slice(self: *@This(), x: []const T) []const u8 {
            const len = @min(self.items.len - self.len, x.len);
            @memcpy(self.items[self.len..self.len + len], x[0..len]);
            self.len += @intCast(len);
            return self.as_const_slice();
        }
        pub fn topup_slice_suffix(self: *@This(), x: []const T, suffix: []const T) []const u8 {
            if (self.len + suffix.len <= self.items.len) {
                const len = @min(self.items.len - self.len - suffix.len, x.len);
                @memcpy(self.items[self.len.. self.len + len], x[0..len]);
                @memcpy(self.items[self.len + len..][0..suffix.len], suffix);
                self.len += @intCast(len + suffix.len);
                return self.as_const_slice();
            } else {
                return self.topup_slice(suffix);
            }
        }

        pub fn as_const_slice(self: *const @This()) []const T {
            return self.items[0..self.len];
        }

        pub fn as_mut_slice(self: *@This()) []T {
            return self.items[0..self.len];
        }

        pub fn dupe_slice(s: []const T) @This() {
            var ret = empty;
            _ = ret.topup_slice(s);
            return ret;
        }
    };
}

test "bounded_array" {
    {
        var a = BoundedArray(u8, 5).empty;
        try std.testing.expectEqualStrings(".json", a.topup_slice(".json"));
    }

    {
        var a = BoundedArray(u8, 5).empty;
        _ = a.topup_slice("ab");
        try std.testing.expectEqualStrings("ab.js", a.topup_slice(".json"));
    }
    {
        var a = BoundedArray(u8, 5).empty;
        try std.testing.expectEqualStrings(".json", a.topup_slice(".jsonextra"));
    }

    {
        var a = BoundedArray(u8, 8).empty;
        try std.testing.expectEqualStrings("a.json", a.topup_slice_suffix("a", ".json"));
    }
    {
        var a = BoundedArray(u8, 8).empty;
        _ = a.topup_slice("yt-");
        try std.testing.expectEqualStrings("yt-.json", a.topup_slice_suffix("a", ".json"));
    }
    {
        var a = BoundedArray(u8, 9).empty;
        _ = a.topup_slice("y-");
        try std.testing.expectEqualStrings("y-a.json", a.topup_slice_suffix("a", ".json"));
    }
}



// @TODO: Check if using a BTreeMap would be faster than a ring buffer
//        This is relevant for the channel list in interactive
//
// Our basic algorithm is start..close are valid index
// Once close has passed len(Buffer), it will always be len + idx, thus close is
// always a > start. It is on the usage code to do a modulus.
pub const RingBuffer = struct {
    data: []Video,
    start: Index,
    close: Index,

    const Index = u32;

    pub inline fn init(count: comptime_int) @This() {
        // Prevent wrap around
        std.debug.assert(count < std.math.maxInt(Index));

        var data: [count]Video = undefined;
        return .{
            .data = &data,
            .start = 0,
            .close = 0,
        };
    }

    pub fn push(self: *@This(), video: *const Video) void {
        const len: Index = @truncate(self.data.len);
        self.data[self.close % len] = video.*;
        self.close += 1;
        if (self.close > len) {
            self.close = (self.close % len) + len;
            self.start = self.close;
        }
        self.start %= len;
    }

    pub fn clear(self: *@This()) void {
        self.start = 0;
        self.close = 0;
    }

    pub fn as_const_slice(self: *const @This()) []const Video {
        if (self.close <= self.data.len) {
            return self.data[0..self.close];
        } else {
            return self.data;
        }
    }

};

test "ring_push" {
    var cache = RingBuffer.init(3);
    cache.push(&.{ .uri = .dupe_slice("a") });
    try std.testing.expectEqualStrings("a", cache.data[0].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("b") });
    try std.testing.expectEqualStrings("a", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("b", cache.data[1].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("c") });
    try std.testing.expectEqualStrings("a", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("b", cache.data[1].uri.as_const_slice());
    try std.testing.expectEqualStrings("c", cache.data[2].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("d") });
    try std.testing.expectEqualStrings("d", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("b", cache.data[1].uri.as_const_slice());
    try std.testing.expectEqualStrings("c", cache.data[2].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("e") });
    try std.testing.expectEqualStrings("d", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("e", cache.data[1].uri.as_const_slice());
    try std.testing.expectEqualStrings("c", cache.data[2].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("f") });
    try std.testing.expectEqualStrings("d", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("e", cache.data[1].uri.as_const_slice());
    try std.testing.expectEqualStrings("f", cache.data[2].uri.as_const_slice());

    cache.push(&.{ .uri = .dupe_slice("g") });
    try std.testing.expectEqualStrings("g", cache.data[0].uri.as_const_slice());
    try std.testing.expectEqualStrings("e", cache.data[1].uri.as_const_slice());
    try std.testing.expectEqualStrings("f", cache.data[2].uri.as_const_slice());
}
