const std = @import("std");
const builtin = @import("builtin");

const constants = @import("download/_constants.zig");
const Downloader = @import("downloader.zig");
const UIState = @import("UIState.zig");

//run: zig run % -- follow

pub fn main(init: std.process.Init) !void {
    const allocator = init.arena.allocator();

    var client = std.http.Client{ .allocator = allocator, .io = init.io };

    var arg_iter = try init.minimal.args.iterateAllocator(allocator);
    _ = arg_iter.next();
    const cmd = arg_iter.next() orelse {
        std.debug.print("Not enough args\n", .{});
        std.process.exit(1);
    };

    if (std.mem.eql(u8, cmd, "interactive")) {
        var state = try UIState.init(allocator, &client);
        state.read_file_and_refresh_config(allocator, init.io, init.environ_map) catch std.process.exit(1);

    } else if (std.mem.eql(u8, cmd, "o") or std.mem.eql(u8, cmd, "open")) {
        const channel = arg_iter.next() orelse {
            std.log.err("Not enough args:\nsurf open <url>\n", .{});
            std.process.exit(1);
        };

        var downloader = try Downloader.init(allocator);
        // @TODO: add ability cli flag to manually specify service provider
        const loc = constants.ChannelLocation.from_uri(channel) catch std.process.exit(1);
        const videos, _ = downloader.query(allocator, &client, &loc) catch std.process.exit(1);

        for (videos.as_const_slice()) |vid| {
            vid.debug_print();
        }

    } else if (std.mem.eql(u8, cmd, "f") or std.mem.eql(u8, cmd, "follow")) {
        var state = try UIState.init(allocator, &client);
        state.read_file_and_refresh_config(allocator, init.io, init.environ_map) catch std.process.exit(1);

        try state.queue_fetches(allocator, state.config.channels.as_const_slice());
        var iter = state.channel_latest_videos.iterator();
        while (iter.next()) |kv| {
            const vid = kv.value_ptr;
            vid.debug_print();
        }

    } else if (std.mem.eql(u8, cmd, "v") or std.mem.eql(u8, cmd, "vods")) {
        const channel = arg_iter.next() orelse {
            std.log.err("Not enough args:\nsurf open <url>\n", .{});
            std.process.exit(1);
        };

        var downloader = try Downloader.init(allocator);
        // @TODO: add ability cli flag to manually specify service provider
        const loc = constants.ChannelLocation.from_uri(channel) catch std.process.exit(1);
        const videos, _ = downloader.query(allocator, &client, &loc) catch std.process.exit(1);

        for (videos.as_const_slice()) |vid| {
            vid.debug_print();
        }

    } else {
        std.debug.print("Unsupported command: {s}\n", .{cmd});
        std.process.exit(1);
    }
    //switch cmd {
    //case "interactive":
    //    src.Set_log_level(io.Discard, src.DEBUG)
    //    UI.Interactive()

    //case "o": fallthrough
    //case "open":
    //    // @TODO: test behaviour on VOD
    //    var channel string
    //    if len(os.Args) >= 3 {
    //    channel = os.Args[2]
    //    }

    //    if strings.ContainsAny(channel, "/") {
    //    fmt.Fprintf(os.Stderr, "Invalid channel name %q", channel)
    //    return
    //    }

    //    sync_refresh(channel)

    //    cur := src.Video{}
    //    buffer_length := len(UI.Cache.Buffer)
    //    for i := UI.Cache.Start; i < UI.Cache.Close; i += 1 {
    //    vid := UI.Cache.Buffer[i % buffer_length]
    //    if vid.Start_time.After(cur.Start_time) {
    //    cur = vid
    //    }
    //    }

    //    play(cur)

    //case "f": fallthrough
    //case "follow":
    //    sync_refresh(UI.Channel_list...)

    //    // @VOLATILE: Load_config seeds the keys
    //    var idx uint = 0
    //    for _, pair := range UI.Follow_latest {
    //    fmt.Println(pair)
    //    if pair.Live.Duration > 0 {
    //    UI.Follow_videos[idx] = pair.Live
    //    } else {
    //    UI.Follow_videos[idx] = pair.Latest
    //    }
    //    idx += 1
    //    }
    //    slices.SortFunc(UI.Follow_videos, src.Sort_videos_by_latest)

    //    choice, err := basic_menu(
    //    "Follow list\n",
    //    len(UI.Follow_videos),
    //    "Enter a Video: ",
    //    func (out io.Writer, idx int) {
    //    tui.Print_formatted_line(out, " | ", UI.Follow_videos[idx])
    //    },
    //    )
    //    if err != nil {
    //    fmt.Fprintf(os.Stderr, "%s\n", err)
    //    return
    //    }
    //    play(UI.Follow_videos[choice])

    //case "v": fallthrough
    //case "vods":
    //    if len(os.Args) < 3 {
    //    fmt.Fprintf(os.Stderr, "Please specify a channel to query the VODs for")
    //    os.Exit(1)
    //    }
    //    channel := os.Args[2]

    //    sync_refresh(channel)
    //    if pair, ok := UI.Follow_latest[channel]; ok && pair.Live.Duration > 0 {
    //    UI.Cache.Push(pair.Live)
    //    }
    //    slices.SortFunc(UI.Cache.Buffer[UI.Cache.Start:UI.Cache.Close], src.Sort_videos_by_latest)

    //    buffer_length := len(UI.Cache.Buffer)
    //    ring_length := UI.Cache.Close - UI.Cache.Start
    //    choice, err := basic_menu(
    //    fmt.Sprintf("VODs for %s\n", channel),
    //    ring_length,
    //    "Enter a Video: ",
    //    func (out io.Writer, idx int) {
    //    reversed_idx := ring_length - ((UI.Cache.Close - idx) % buffer_length)
    //    vid := UI.Cache.Buffer[reversed_idx]
    //    tui.Print_formatted_line(out, " | ", vid)
    //    },
    //    )
    //    if err != nil {
    //    fmt.Fprintf(os.Stderr, "%s\n", err)
    //    return
    //    }

    //    vid := UI.Cache.Buffer[(UI.Cache.Close - choice - 1) % buffer_length]
    //    play(vid)

    //default:
    //    fmt.Fprintf(os.Stderr, "Unsupported command %q\n", cmd)
    //}

}

