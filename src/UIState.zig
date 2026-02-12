const std = @import("std");
const builtin = @import("builtin");

const constants = @import("download/_constants.zig");
const Downloader = @import("downloader.zig");

http_client: *std.http.Client,
config: struct {
    channels: constants.BoundedArray(constants.ChannelLocation, constants.MAX_FOLLOW_COUNT) = .empty,
} = .{},
downloader: Downloader,
channel_latest_videos: std.StringHashMapUnmanaged(constants.Video),

pub fn init(allocator: std.mem.Allocator, client: *std.http.Client) !@This() {
    var ret = @This(){
        .http_client = client,
        .downloader = try .init(allocator),
        .channel_latest_videos = .empty,
    };
    try ret.channel_latest_videos.ensureTotalCapacity(allocator, constants.MAX_FOLLOW_COUNT * 2);
    return ret;
}

pub fn deinit(self: *@This(), allocator: std.mem.Allocator) void {
    self.http_client.deinit();
    self.channel_latest_videos.deinit(allocator);
    self.downloader.deinit(allocator);
}

////////////////////////////////////////////////////////////////////////////////

const ConfigFile = struct {
    channels: []const struct {
        alias: []const u8,
        url: []const u8,
    } = &.{},
};


pub fn queue_fetches(self: *@This(), allocator: std.mem.Allocator, channels: []const constants.ChannelLocation) !void {
    // @TODO: use io.async()
    for (channels) |*chan| {
        std.log.debug("Fetching: {s}", .{chan.uri.as_const_slice()});
        var videos, const updates_live = try self.downloader.query(allocator, self.http_client, chan);
        self.add_channel_videos(chan.channel.as_const_slice(), videos.as_mut_slice(), updates_live);
    }
}

pub fn add_channel_videos(self: *@This(), channel_uri: []const u8, videos: []constants.Video, updates_live: bool) void {
    std.debug.assert(channel_uri.len <= @FieldType(constants.ChannelLocation, "channel").empty.items.len);
    var latest_video: constants.Video = if (updates_live) blk: {
        for (videos) |*video| {
            if (video.is_live) {
                std.log.debug("Update live", .{});
                self.channel_latest_videos.putAssumeCapacity(video.channel.uri.as_const_slice(), video.*);
                break :blk video.*;
            } 
        }
        break :blk .empty;
    } else .empty;
    latest_video = latest_video;

    for (videos) |*video| {
        if (video.is_live) {
            //self..put(video.url.as_const_slice());
        }
    }
}

// Single command that does the full integration
pub fn read_file_and_refresh_config(self: *@This(), allocator: std.mem.Allocator, io: std.Io, env: *const std.process.Environ.Map) !void {
    const path = config_path(env).as_const_slice();

    var config_str = blk: {
        var content = std.Io.Writer.Allocating.initCapacity(allocator, 1 * 1024 * 1024) catch {
            std.log.err("Ran out of memory while reading '{s}'\n", .{path});
            return error.Parsing;
        };

        const cwd = std.Io.Dir.cwd();
        const fh = cwd.openFile(io, path, .{}) catch |err| {
            std.log.warn("Could not open config file with error {t}: {s}\n", .{err, path});
            return error.Open;
        };
        var buffer: [4096]u8 = undefined;
        var reader = fh.reader(io, &buffer);
        _ = content.writer.sendFileAll(&reader, .limited(std.math.maxInt(u32))) catch |err| {
            std.log.warn("Could not open read file with error {t}: {s}\n", .{err, path});
            return error.Open;
        };
        break :blk content.toArrayList();
    };
    defer config_str.deinit(allocator);

    const len = config_str.items.len;
    const sentinel: u8 = 0;
    config_str.append(allocator, sentinel) catch {
        std.log.err("Ran out of memory while parsing '{s}'\n", .{path});
        return error.Parsing;
    };

    try self.load_config(allocator, config_str.items[0..len :sentinel], path);
}

pub fn config_path(env: *const std.process.Environ.Map) constants.BoundedArray(u8, std.fs.max_path_bytes) {
        const maybe_home, const default_config_path = switch (builtin.os.tag) {
        .plan9 => .{env.get("home"), ".config/channelsurf/config.zon"},
        .windows => .{env.get("HOME"), "" },
        else => .{env.get("HOME"), ".config/channelsurf/config.zon" },
    };
    const env_home = maybe_home orelse @panic("Requires $HOME to be definined as an environment variable. Try exporting it.");

    var buffer: [std.Io.Dir.max_path_bytes]u8 = undefined;
    var fba: std.heap.FixedBufferAllocator = .init(&buffer);
    const joined = std.Io.Dir.path.join(fba.allocator(), &.{env_home, default_config_path}) catch unreachable;
    return .{
        .items = buffer,
        .len = @intCast(joined.len),
    };
}

// Designed to be rerun, e.g. if a user changes config during TUI usage.
pub fn load_config(self: *@This(), allocator: std.mem.Allocator, config_str: [:0]const u8, display_path: []const u8) !void {
    // @TODO: Maybe check on how to deinit zon parsing
    var diag = std.zon.parse.Diagnostics{};
    const config_file = std.zon.parse.fromSliceAlloc(ConfigFile, allocator, config_str, &diag, .{ .free_on_error = true }) catch |err| switch (err) {
        error.ParseZon => {
            std.log.err("Failed to parse '{s}'\n", .{display_path});
            var iter = diag.iterateErrors();
            while (iter.next()) |e| {
                const loc = e.getLocation(&diag);
                const msg = e.fmtMessage(&diag);
                var notes = e.iterateNotes(&diag);
                // This should be parsable by vim's `errorformat` or emac's
                // `compilation-error-regexp-alist`.
                // These text editors do regexp/scanf scan of stderr/out
                // and jump to the source code location specified.
                std.log.err("{s}:{d}:{d} {f}", .{display_path, loc.line + 1, loc.column + 1, msg});
                while (notes.next()) |note| {
                    const note_loc = note.getLocation(&diag);
                    const note_msg = note.fmtMessage(&diag);
                    std.log.err("{s}:{d}:{d}\n{f}", .{display_path, note_loc.line + 1, note_loc.column + 1, note_msg});
                }
            }
            return error.Parsing;
        },
        error.OutOfMemory => {
            std.log.err("Ran out of memory while reading '{s}'\n", .{display_path});
            return error.Parsing;
        },
    };
    defer std.zon.parse.free(allocator, config_file);
    errdefer std.zon.parse.free(allocator, config_file);

    if (config_file.channels.len > self.config.channels.items.len) {
        std.log.warn("Hardcoded max limit of {d} channels. File an issue on to increase this.", .{constants.MAX_FOLLOW_COUNT});
    }
    self.config.channels.len = 0;
    for (config_file.channels) | chan| {
        var loc: constants.ChannelLocation = try .from_uri(chan.url);
        loc.alias = .dupe_slice(chan.alias);
        self.config.channels.topup(loc) catch unreachable;
    }

    ////////////////////////////////////////////////////////////////////////////

    self.downloader.update_follow_list(self.config.channels.as_const_slice());
}

// @TODO: add test for new config, asserting old channel_latest_videos no longer has old members
test "load_config" {
    const allocator = std.testing.allocator;
    var client = std.http.Client{ .io = std.testing.io, .allocator = std.testing.allocator };

    const config_str1 =
        \\.{
        \\    .channels = .{
        \\        .{ .alias = "Zig", .url = "https://youtu.be/@ZigShowtime" },
        \\        .{ .alias = "Loris", .url = "https://twitch.tv/kristoff_it" },
        \\    },
        \\}
    ;
    var arena = std.heap.ArenaAllocator.init(std.testing.allocator);
    defer arena.deinit();
    var config: @This() = try .init(allocator, &client);
    defer config.deinit(allocator);

    try config.load_config(arena.allocator(), config_str1, "");

    try std.testing.expectEqualStrings("Zig", config.config.channels.items[0].alias.as_const_slice());

    const config_str2 =
        \\.{
        \\    .channels = .{
        \\        .{ .alias = "Zig", .url = "https://youtu.be/@ZigShowtime" },
        \\    },
        \\}
    ;
    if (false) {
        const a = config_str2;
        _ = a;
    }
    try config.load_config(arena.allocator(), config_str2, "");
}
