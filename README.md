This is a local-first Terminal User Interface (TUI) for various media platforms to simulate subscribing/follow to channels.

This works based off of streamlink.
The zig rewrite is in progress.

Currently the UI looks like the following:

```
Follow
sphaero... | A fetch() api that doesn't ... | 17 hr ago | 1h51m
j_blow     | Berries (with fried rice br... | 33 hr ago | 4h00m
tsoding    | Game Dev in C — Debug Console  | 3 d ago   | 2h30m
vedal987   | evil? yeah. you know it.       | 4 d ago   | 2h21m
kristof... | Awebo - OSS Discord alterna... | 5 d ago   | 6h16m
gamozo     | Pending...                     |           |
jonhoo     | Pending...                     |           |
wookash... | Pending...                     |           |
andrewrok  | Pending...                     |           |
fengb      | Pending...                     |           |

 (q)uit (r)efresh (hjkl) navigate

ui_selection: 0
```

You can see a stripped-down version of scraping in example.sh

# Usage

Currently you run this by `zig run main.zig`.

This is the zig rework.
The go version (on the `main` branch) has more features.

# Architecture

## Platforms

* Twitch - is done via the same GraphQL API with same ClientID that the main website uses. This is not publically documented, but see [this repo](https://github.com/SuperSonicHub1/twitch-graphql-api) that was last updated on Dec 28, 2021.
* YouTube - is done via webscrapping. I employ a similar method to CSS selectors.

## Dependencies

I have not planned out video playback, but it will likely be libmpv or ffmpeg.
I wanted to use EGL + Kitty for a TUI video player and [zigx](https://github.com/marler8997/zigx) + EGL for a GUI video player.

This will probably have to depend on youtube-dl/yt-dlp being installed.
There is a lot of work that goes into getting the actual video stream.

Other than that, I do not plan on using any dependencies.

# Features

* Offline
    * [x] Follow streams anonymously (local text config file of streams to follow)
    * [x] Unicode support (subject to your terminal's unicode support and the font you use)
    * [x] View latest
    * [x] View livestreams
    * [ ] View chat/comments on VODs and livestreams
    * [ ] [BTTV](https://betterttv.com/) emotes?
    * [ ] Support for chat emotes via Kitty protocol (see [bork](github.com/kristoff-it/bork))
    * [ ] Scroll chat history via keyboard
    * [ ] Sync streamlink and chat (might not be possible without taking control of mpv, maybe we just want to make syncing chat to current timestamp easy?)
    * [ ] UI to Scrub through video
    * [ ] Seamless rewind into VOD for live streams

* Livestream Tooling
    * [ ] Highlight a user message (good for streaming)
    * [ ] Open a tab that filter for messages by a group of users
    * [ ] Login
    * [ ] Event generation for when
    * [ ] View messages with temporal context
    * [ ] view mod notes

* Channel Exploration
    * [ ] View all videos from a channel
    * [ ] View shorts. I probably will not support this.
    * [ ] View clips. You probably want a more interactive, browser-like experience to view clips anyway 
    * [ ] Platform recommendations
    * [ ] Grayjay recommendations

# See Also

* [bork](github.com/kristoff-it/bork). This is live-streaming first
* NewPipe
* [twineo](https://codeberg.org/CloudyyUw/twineo) a privacy-first proxy/frontend
* GrayJay, FUTO's multiplatform feed agregrator and video player that has a [twitch](https://github.com/futo-org/grayjay-plugin-twitch/blob/master/TwitchScript.js) plugin
* [One of many examples](https://github.com/luukjp/twitch-live-status-checker) of using GraphQL.
* Social media [alternate-front-ends](https://github.com/mendel5/alternative-front-ends)
