This is a local-first Terminal User Interface for Twitch.
Maybe YouTube as well, but let's not get our hopes up.

This works based off of streamlink.
I will most likely rewrite this to zig once the Async rework has landed.

You can see a stripped-down version of scraping in example.sh

# Usage

I have not set up compiling into a binary yet, so `go run main.go` is the way to use this.
Create a text file called `channel_list.txt` and put channel names separated by newlines.


# Architecture

When trying to shim Twitch, there are essentially three approaches you could take.

* Scrapping
    * Selenium, i.e. full javascript and dom emulation
    * Curl, and parse the frontend data packet. Curling sometimes pre-loaded state, and the VOD screen is often not sufficient on first CURL. So JavaScript machinery would be necessary.

* Use Twitch APIs
    * GraphQL - This is what the official web front end uses, but it is not publically documented. See [this repo](https://github.com/SuperSonicHub1/twitch-graphql-api) that was lsat updated on Dec 28, 2021.
    * Register as an app and use the REST API
    * Login as a user and use the REST API

You can use GraphQL as an anonymous user.

# Features

* Basic Features
    * [x] Follow streams anonymously (local text config file of streams to follow)
    * [x] Unicode support (subject to your terminal's unicode support and the font you use)
    * [ ] View chat
    * [ ] Login to twitch

* Exploration
    * [ ] ~Search for streams~ (too much work)
    * [ ] ~View recommended streams~ (too much work)

* Video features
    * [ ] UI to Scrub through video (WIP)
    * [ ] Sync scrubbing with live chat
    * [ ] Seemless rewind into vod for live streams

* Chat features
    * [ ] Sync streamlink and chat (might not be possible without taking control of mpv, maybe we just want to make syncing chat to current timestamp easy?)
    * [ ] Scroll chat history via keyoard
    * [ ] Highlight a user message (good for streaming)
    * [ ] Search users and messages (in context window?)
    * [ ] Support for chat emotes via Kitty protocol (see [bork](github.com/kristoff-it/bork))
    * [ ] BTTV emotes?

* Mod tools
    * [ ] enter to view message with chat context
    * [ ] view stream context?
    * [ ] view mod notes

* [x] ~Clips~: Likely will not support this. You want to be more interactive, browser-like experience to view clips anyway 

# See Also

* NewPipe
* [twineo](https://codeberg.org/CloudyyUw/twineo) a privacy-first proxy/frontend
* GrayJay, FUTO's multiplatform feed agregrator and video player that has a [twitch](https://github.com/futo-org/grayjay-plugin-twitch/blob/master/TwitchScript.js) plugin
* [One of many examples](https://github.com/luukjp/twitch-live-status-checker) of using GraphQL.
* Social media [alternate-front-ends](https://github.com/mendel5/alternative-front-ends)
