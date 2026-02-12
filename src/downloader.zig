const std = @import("std");

pub const constants = @import("download/_constants.zig");
pub const parsing = @import("download/_parsing.zig");
pub const youtube = @import("download/youtube_scrape.zig");
pub const twitch = @import("download/twitch_graphql.zig");

// @TODO: add RWLock
cache_follow: std.ArrayList(constants.RingBuffer),
follows: std.ArrayList(constants.ChannelLocation),
uri_2_idx: std.StringHashMapUnmanaged(Index),
cache_videos: constants.RingBuffer,

_buffer: []constants.Video,

//follows: []const constants.ChannelLocation,

// Our index must be large enough
const Index = u11;
comptime {
    std.debug.assert(constants.MAX_FOLLOW_COUNT <= std.math.maxInt(Index));
}
const CACHE_VIDEO_CAPACITY = 1000;
const MAX_BACKLOG_PER_CHANNEL = 50;

////////////////////////////////////////////////////////////////////////////////

pub inline fn init(allocator: std.mem.Allocator) !@This() {
    //const data: std.ArrayList(constants.RingBuffer) = .initCapacity
    var ret = @This(){
        ._buffer = try allocator.alloc(constants.Video, constants.MAX_FOLLOW_COUNT * MAX_BACKLOG_PER_CHANNEL),
        .cache_follow = try .initCapacity(allocator, constants.MAX_FOLLOW_COUNT),
        .follows = try .initCapacity(allocator, constants.MAX_FOLLOW_COUNT),
        .uri_2_idx = .empty,
        .cache_videos = .init(CACHE_VIDEO_CAPACITY),
    };
    try ret.uri_2_idx.ensureTotalCapacity(allocator, constants.MAX_FOLLOW_COUNT * 2); // We want at most 75% full

    var i: u32 = 0;
    for (ret.cache_follow.items.ptr[0..constants.MAX_FOLLOW_COUNT]) |*ring| {
        ring.data = ret._buffer[i..i + MAX_BACKLOG_PER_CHANNEL];
        i += MAX_BACKLOG_PER_CHANNEL;
    }
    return ret;
}

pub fn deinit(self: *@This(), allocator: std.mem.Allocator) void {
    self.cache_follow.deinit(allocator);
    self.follows.deinit(allocator);
    self.uri_2_idx.deinit(allocator);
    allocator.free(self._buffer);
}

pub fn update_follow_list(self: *@This(), follows: []const constants.ChannelLocation) void {
    const len = @min(follows.len, constants.MAX_FOLLOW_COUNT);
    self.cache_follow.items.len = len;

    self.follows.items.len = len;
    @memcpy(self.follows.items, follows[0..len]);
    std.mem.sort(constants.ChannelLocation, self.follows.items, {}, struct {
        fn lambda(_: void, a: constants.ChannelLocation, b: constants.ChannelLocation) bool {
            return std.mem.lessThan(u8, a.uri.as_const_slice(), b.uri.as_const_slice());
        }
    }.lambda);

    {
        var to_remove: constants.BoundedArray([]const u8, constants.MAX_FOLLOW_COUNT) = .empty;
        var iter = self.uri_2_idx.keyIterator();
        while (iter.next()) |key| {
            // Not worth sorting because hashmap (channel_latest_videos) doesn't retain order
            var contains = false;
            for (self.follows.items) |loc| {
                if (std.mem.eql(u8, key.*, loc.uri.as_const_slice())) {
                    contains = true;
                    break;
                }
            }
            if (!contains) {
                to_remove.topup(key.*) catch unreachable;
            }
        }

        // Copy over cache lines. Because self._follow
        var reverse_idx = len;
        while (reverse_idx > 0) {
            reverse_idx -= 1;
            const target = self.follows.items[reverse_idx];

            if (self.uri_2_idx.get(target.uri.as_const_slice())) |old_idx| {
                if (reverse_idx == old_idx) {
                    continue;
                }
                const cur_ring = &self.cache_follow.items[reverse_idx];
                const old_ring = self.cache_follow.items[old_idx];
                const old_slice = old_ring.as_const_slice();

                @memcpy(cur_ring.data[0..old_slice.len], old_slice);
                cur_ring.start = old_ring.start;
                cur_ring.close = old_ring.close;
            }
        }

        // Remove
        for (to_remove.as_const_slice()) |uri| {
            const is_present = self.uri_2_idx.remove(uri);
            std.debug.assert(is_present);
        }
    }

    for (0.., follows) |i, *target| {
        self.uri_2_idx.putAssumeCapacity(target.uri.as_const_slice(), @truncate(i));
    }
}

//run: zig test % --test-filter update_follow_list
test "update_follow_list" {
    var client = std.http.Client{ .io = std.testing.io, .allocator = std.testing.allocator };
    defer client.deinit();
    var downloader: @This() = try .init(std.testing.allocator);
    defer downloader.deinit(std.testing.allocator);

    const channels1 = [_]constants.ChannelLocation{
        .{ .alias = .dupe_slice("Zig"), .uri = .dupe_slice("https://youtu.be/@ZigShowtime") },
        .{ .alias = .dupe_slice("Loris"), .uri = .dupe_slice("https://twitch.tv/kristoff_it") },
    };
    downloader.update_follow_list(&channels1);


    try std.testing.expectEqualStrings("Loris", downloader.follows.items[0].alias.as_const_slice());
    try std.testing.expectEqualStrings("Zig", downloader.follows.items[1].alias.as_const_slice());
    try std.testing.expectEqual(channels1.len, downloader.follows.items.len);


    const channels2 = [_]constants.ChannelLocation{
        .{ .alias = .dupe_slice("Zig"), .uri = .dupe_slice("https://youtu.be/@ZigShowtime") },
    };
    downloader.update_follow_list(&channels2);
    try std.testing.expectEqualStrings("Zig", downloader.follows.items[0].alias.as_const_slice());
    try std.testing.expectEqual(channels2.len, downloader.follows.items.len);
}

////////////////////////////////////////////////////////////////////////////////

pub fn query(self: *@This(), allocator: std.mem.Allocator, client: *std.http.Client, target: *const constants.ChannelLocation) !struct {constants.BoundedArray(constants.Video, constants.MAX_PAGE_SIZE), bool} {
    const follow_ring: ?*constants.RingBuffer = if (self.uri_2_idx.get(target.uri.as_const_slice())) |idx| &self.cache_follow.items[idx] else null;

    const videos, const updates_live = switch (target.service) {
        .youtube => blk: {
            const videos = try youtube.fetch(allocator, client, target);
            break :blk .{videos, false};
        },
        .twitch => blk: {
            const videos = try twitch.fetch(allocator, client, target);
            for (videos.as_const_slice()) |*vid| {
                if (follow_ring) |ring| ring.push(vid);
                self.cache_videos.push(vid);
            }
            break :blk .{videos, true};
        },
    };
    for (videos.as_const_slice()) |*vid| {
        if (follow_ring) |ring| ring.push(vid);
        self.cache_videos.push(vid);
    }
    return .{videos, updates_live};
}

test "query" {
    var client = std.http.Client{ .io = std.testing.io, .allocator = std.testing.allocator };
    defer client.deinit();
    var downloader: @This() = try .init(std.testing.allocator);
    defer downloader.deinit(std.testing.allocator);
    _, _ = try downloader.query(std.testing.allocator, &client, &.{ .service = .twitch, .alias = .dupe_slice("scarra"), .channel = .dupe_slice("scarra") });

    for (downloader.cache_follow.items) |ring| {
        for (ring.as_const_slice()) |vid| {
            //vid.debug_print();
            _ = vid;
        }
    }
    for (downloader.cache_videos.as_const_slice()) |vid| {
        //vid.debug_print();
        _ = vid;
    }
}
