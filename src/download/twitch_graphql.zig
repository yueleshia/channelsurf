const std = @import("std");
const constants = @import("_constants.zig");
const util = @import("_parsing.zig");

//run: zig test % -L..

test "fetch" {
    const allocator = std.testing.allocator;

    var client = std.http.Client{ .io = std.testing.io, .allocator = allocator };
    defer client.deinit();

    var videos = try fetch(allocator, &client, &.{ .alias = .dupe_slice("kristoff_it"), .channel = .dupe_slice("kristoff_it") });
    for (videos.as_const_slice()) |v| {
        if (true) {
            const a = v;
            a.debug_print();
        }
    }
}


const Video = constants.Video;
const CLIENT_ID = "ue6666qo983tsx6so1t0vnawi233wa"; // old: kimne78kx3ncx6brgo4mv6wki5h1ko
const USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36";

const vods_query_raw = 
    \\user(login: $channelOwnerLogin) {
    \\    id
    \\
    \\    videos(first: $limit, after: $cursor, type: $broadcastType, sort: $videoSort, options: $options) {
    \\        edges {
    \\            cursor
    \\            node {
    \\                __typename
    \\                id
    \\                title
    \\                previewThumbnailURL(width: 320, height: 180)
    \\                publishedAt
    \\                lengthSeconds
    \\                game {
    \\                    name
    \\                }
    \\                owner {
    \\                    id
    \\                    displayName
    \\                    login
    \\                    profileImageURL(width: 50)
    \\                }
    \\                moments(first: 0, after: null, sort: ASC, types: GAME_CHANGE, momentRequestType: VIDEO_CHAPTER_MARKERS) {
    \\                    edges {
    \\                        node {
    \\                            description
    \\                            positionMilliseconds
    \\                        }
    \\                    }
    \\                    pageInfo {
    \\                        hasNextPage
    \\                    }
    \\                }
    \\            }
    \\        }
    \\        pageInfo {
    \\            hasNextPage
    \\        }
    \\    }
    \\
    \\    stream {
    \\        createdAt
    \\    }
    \\    broadcastSettings {
    \\        game {
    \\            name
    \\        }
    \\        title
    \\    }
    \\}
;
const Output = constants.BoundedArray(Video, constants.MAX_PAGE_SIZE);
const vods_query = blk: {
    @setEvalBranchQuota(vods_query_raw.len * 8);
    break :blk graphql_template("videos", vods_args, vods_query_raw);
};
const vods_args: []const struct {[]const u8, []const u8} = &.{
    .{"broadcastType", "BroadcastType"},
    .{"channelOwnerLogin", "String!"},
    .{"cursor", "Cursor"},
    .{"limit", "Int"},
    .{"videoSort", "VideoSort"},
    .{"options", "VideoConnectionOptionsInput"},
};

pub fn fetch(allocator: std.mem.Allocator, client: *std.http.Client, target: *const constants.ChannelLocation) !Output {
    var body_writer: std.Io.Writer.Allocating = try .initCapacity(allocator, 2 * 1024 * 1024); // 2 MB

    const channel_quoted = blk: {
        var id: constants.BoundedArray(u8, 50) = .empty;
        id.topup('"') catch unreachable;
        break :blk id.topup_slice_suffix(target.channel.as_const_slice(), "\"");
    };

    // @TODO: Use FixedBufferAllocator for this
    const payload = try std.fmt.allocPrint(allocator, vods_query, .{
        "null",
        channel_quoted,
        "null",
        std.fmt.comptimePrint("{d}", .{constants.metadata.getAssertContains(.twitch).page_size - 1}),
        "\"TIME\"",
        "null",
    });
    defer allocator.free(payload);
    errdefer allocator.free(payload);

    // For some reason, we cannot assert valid json at comptime
    // @TODO: assert is valid json, since we cannot do this at comptime

    const cache_id = blk: {
        var id: constants.BoundedArray(u8, 50) = .empty;
        _ = id.topup_slice("twitch-");
        break :blk id.topup_slice_suffix(target.channel.as_const_slice(), ".json");
    };
    const status = try util.request_wrapper(client, cache_id, &body_writer, .{
        .method = .POST,
        .location = .{ .uri = comptime std.Uri.parse("https://gql.twitch.tv/gql#origin=twilight") catch unreachable },
        .headers = .{
            .content_type = .{ .override = "text/plain; charset=UTF-8" },
            .accept_encoding = .{ .override = "en-US" },
        },
        .extra_headers = &.{
            .{ .name = "Client-Id", .value = CLIENT_ID },
            .{ .name = "Accept-Language", .value =  "en-US" },
        },
        .payload = payload,
    });
    switch (status.status) {
        else => {},
    }


    //if (false) {
    //    var buffer: [4096]u8 = undefined;
    //    var writer = std.Io.File.stdout().writer(client.io, &buffer);
    //    _ = try std.json.fmt(query.value, .{ .whitespace = .indent_2 }).format(&writer.interface);
    //    try writer.flush();
    //}
    var resp = body_writer.toArrayList();
    defer resp.deinit(allocator);
    errdefer resp.deinit(allocator);

    return try parse(allocator, target.*, resp.items);
}

pub fn parse_channel() !constants.Channel{
}

// We specify the struct we want from GraphQL, so we can just JSON deserialise an exact struct
pub fn parse(allocator: std.mem.Allocator, target: constants.ChannelLocation, s: []const u8) !Output {
    var ret: Output = .empty;

    // @TODO: use arena and use parseFromSliceLeaky (less book keeping)
    const query_full = try std.json.parseFromSlice(Query, allocator, s, .{});
    defer query_full.deinit();

    const query: Query = query_full.value;

    for (query.data.user.videos.edges) |edge| {
        const raw = edge.node;
        if (!std.mem.eql(u8, raw.__typename, "Video")) {
            continue;
        }

        var vid: Video = .{
            .uri = blk: {
                var x = @FieldType(constants.ChannelLocation, "uri").dupe_slice("https://twitch.tv/videos/");
                _ = x.topup_slice(raw.id);
                break :blk x;
            },
            .channel = target,
            .title = .dupe_slice(raw.title),
            .start_time = .{ .nanoseconds = 0 }, // @TODO: parse
            .duration = .{ .nanoseconds = 0 }, // @TODO: parse
            .is_live = false,
            .chapters = .empty,
        };

        for (raw.moments.edges) |moment| {
            vid.chapters.topup(.{
                .name = .dupe_slice(moment.node.description),
                .time = .{ .nanoseconds = @as(i96, moment.node.positionMilliseconds) * std.time.ns_per_ms },
            }) catch std.log.warn("Hit max chapters for {s}", .{target.uri.as_const_slice()});
        }

        ret.topup(vid) catch unreachable;
    }

    return ret;
}


const Query = struct {
    data: struct {
        user: struct {
            id: []const u8,
            videos: struct {
                edges: []const struct {
                    cursor: []const u8,
                    node: struct {
                        __typename: []const u8,
                        id: []const u8,
                        title: []const u8,
                        previewThumbnailURL: []const u8,
                        publishedAt: []const u8,
                        lengthSeconds: u32,
                        game: ?struct {
                            name: []const u8,
                        },
                        owner: struct {
                            id: []const u8,
                            displayName: []const u8,
                            login: []const u8,
                            profileImageURL: []const u8,
                        },
                        moments: struct {
                            edges: []struct {
                                node: struct {
                                    description: []const u8,
                                    positionMilliseconds: i64,
                                },
                            },
                            pageInfo: struct {
                                hasNextPage: bool,
                            }
                        },
                    },
                },
                pageInfo: struct {
                    hasNextPage: bool,
                },
            },

            // Related to live status
            stream: ?struct {
                createdAt: []const u8,
            },
            broadcastSettings: struct {
                game: struct {
                    name: []const u8,
                },
                title: []const u8,
            }
        }
    },
    extensions: struct {
        durationMilliseconds: i96,
        operationName: []const u8,
        requestID: []const u8,
    },
};



// You will need to @setEvalBranchQuota quite high depending on the `query_body`
inline fn graphql_template(query_name: []const u8, args: []const struct{[]const u8, []const u8}, query_body: []const u8) []const u8 {
    comptime {
        var decleration: []const u8 = query_name ++ "(";
        for (0.., args) |i, kv| {
            const key, const gql_ty = kv;
            if (i != 0) decleration = decleration ++ ",";
            decleration = decleration ++ "$" ++ key ++ ":" ++ gql_ty;

        }
        decleration = decleration ++ ")";

        var query = query_body;
        query = util.comptime_replace(query, "\n", "\\n");
        query = util.comptime_replace(query, "{", "{{");
        query = util.comptime_replace(query, "}", "}}");
        query = "query " ++ decleration ++ "{{\\n" ++ query ++ "\\n}}";
        std.debug.assert(std.mem.findScalar(u8, query, '\n') == null);

        

        var ret: []const u8 = "{{"
            ++ ("\"operationName\":\"" ++ query_name ++ "\",")
            ++ "\"query\": \"" ++ query ++ "\","
            ++ "\"variables\":{{"
        ;
        ret = ret;

        for (0.., args) |i, kv| {
            const key, _ = kv;
            if (i != 0) ret = ret ++ ",";
            ret = ret ++ "\"" ++ key ++ "\":{s}";
        }
        // @TODO: assert all the $key are present in `args`

        {
            const test_types: [args.len]type = @splat([]const u8);
            var test_values: @Tuple(&test_types) = undefined;
            for (&test_values) |*c| {
                c.* = "";
            }
            const reified = std.fmt.comptimePrint(ret, test_values);
            std.debug.assert(reified.len > 0);
            if (false) {
                var buf: [1000]u8 = @splat(0);
                var fba = std.heap.FixedBufferAllocator.init(&buf);
                std.debug.assert(std.json.validate(fba.allocator(), reified));
            }
        }

        
        return ret ++ "}}}}";
    }
}
