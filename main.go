// The entrypoint, this handles CLI parsing and the TUI
package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	//"time"
	"os"

	"github.com/yueleshia/streamsurf/src"
	"github.com/yueleshia/streamsurf/src/tui"
)

func help() {
		fmt.Println(`
Possible options:
USAGE: (Use first character or full word)

streamsurf follow                    - list online status of various channels
streamsurf open <channel> [<offset>] - see latest vods
streamsurf vods <channel> [<offset>] - see latest vods
`)
}

//go:embed channel_list.txt
var CHANNELS string

//run: go run % f

var UI = tui.UIState{}

// Plumbing (low-level) and porcelain (user-facing) are GIT developer terminology
func main() {
	pprof.StartCPUProfile(src.Must(os.Create("cpu.pprof")))
	defer pprof.StopCPUProfile()

	src.Set_log_level(os.Stderr, src.DEBUG)

	{
		list := make([]string, len(os.Args[1:]))
		for i, arg := range os.Args[1:] {
			list[i] = fmt.Sprintf("%q", arg)
		}
		src.L_DEBUG.Printf("Args: %s\n", strings.Join(list, " "))
	}

	var cmd string
	if len(os.Args[1:]) <= 0 {
		cmd = "interactive"
	} else {
		cmd = os.Args[1]
	}

	UI.Load_config(CHANNELS)

	switch cmd {
	case "interactive":
		src.Set_log_level(io.Discard, src.DEBUG)
		UI.Interactive(&UI.Cache)

	case "o": fallthrough
	case "open":
		// @TODO: test behaviour on VOD
		var channel string
		if len(os.Args) >= 3 {
			channel = os.Args[2]
		}

		if strings.ContainsAny(channel, "/") {
			fmt.Fprintf(os.Stderr, "Invalid channel name %q", channel)
			return
		}

		{
			jobs := []func (string) src.Result[[]src.Video]{
				src.Graph_vods,
				src.Scrape_live_status,
			}
			vid_chan := make(chan src.Result[[]src.Video], len(jobs))
			for _, job := range jobs {
				go func() { vid_chan <- job(channel) }()
			}
			for i := 0; i < len(jobs); i += 1 {
				x := <-vid_chan
				if videos, err := x.Val, x.Err; err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
				} else {
					UI.Cache.Add(videos)
				}
			}
		}

		cur := src.Video{}
		buffer_length := len(UI.Cache.Buffer)
		for i := UI.Cache.Start; i < UI.Cache.Close; i += 1 {
			vid := UI.Cache.Buffer[i % buffer_length]
			if vid.Start_time.After(cur.Start_time) {
				cur = vid
			}
		}

		play(cur)

	case "f": fallthrough
	case "follow":
		var wg sync.WaitGroup
		wg.Add(len(UI.Channel_list))

		{
			jobs := []func (string) src.Result[[]src.Video]{
				src.Graph_vods,
				src.Scrape_live_status,
			}
			job_count := len(jobs) * len(UI.Channel_list)

			vid_chan := make(chan src.Result[[]src.Video], job_count)
			for _, channel := range UI.Channel_list {
				for _, job := range jobs {
					go func() { vid_chan <- job(channel) }()
				}
			}
			for i := 0; i < job_count; i += 1 {
				x := <-vid_chan
				if videos, err := x.Val, x.Err; err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
				} else {
					UI.Cache.Add(videos)
				}
			}
		}

		idx := 0
		for _, vid := range UI.Cache.Latest {
			UI.Follow_videos[idx] = vid
			idx += 1
		}
		slices.SortFunc(UI.Follow_videos, tui.Sort_videos_by_latest)

		choice, err := basic_menu(
			"Follow list\n",
			len(UI.Follow_videos),
			"Enter a Video: ",
			func (out io.Writer, idx int) {
				tui.Print_formatted_line(out, " | ", UI.Follow_videos[idx])
			},
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return
		}
		play(UI.Follow_videos[choice])

	case "v": fallthrough
	case "vods":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Please specify a channel to query the VODs for")
			os.Exit(1)
		}
		channel := os.Args[2]
		{
			jobs := []func (string) src.Result[[]src.Video]{
				src.Graph_vods,
				src.Scrape_live_status,
			}
			vid_chan := make(chan src.Result[[]src.Video], len(jobs))
			for _, job := range jobs {
				go func() { vid_chan <- job(channel) }()
			}
			for i := 0; i < len(jobs); i += 1 {
				x := <-vid_chan
				if videos, err := x.Val, x.Err; err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
				} else {
					UI.Cache.Add(videos)
				}
			}
		}
		slices.SortFunc(UI.Cache.Buffer[UI.Cache.Start:UI.Cache.Close], tui.Sort_videos_by_latest)

		buffer_length := len(UI.Cache.Buffer)
		choice, err := basic_menu(
			fmt.Sprintf("VODs for %s\n", channel),
			UI.Cache.Close - UI.Cache.Start,
			"Enter a Video: ",
			func (out io.Writer, idx int) {
				vid := UI.Cache.Buffer[(UI.Cache.Close - idx - 1) % buffer_length]
				tui.Print_formatted_line(out, " | ", vid)
			},
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return
		}

		vid := UI.Cache.Buffer[(UI.Cache.Close - choice - 1) % buffer_length]
		play(vid)

		//var list strings.Builder
		//buffer_length := len(UI.Cache.Buffer)
		//for i := UI.Cache.Start; i < UI.Cache.Close; i += 1 {
		//	vid := UI.Cache.Buffer[i % buffer_length]
		//	tui.Print_formatted_line(&list, " | ", vid)
		//}

	default:
		fmt.Fprintf(os.Stderr, "Unsupported command %q\n", cmd)
	}
}

func play(vid src.Video) {
	tui.Print_formatted_line(os.Stderr, " | ", vid)
	stdin := bufio.NewReader(os.Stdin)
	if vid.Is_live {
		fmt.Fprint(os.Stderr, "Start time (e.g. 1:00:00) (leave blank for live): ")
	} else {
		fmt.Fprint(os.Stderr, "Start time (e.g. 1:00:00): ")
	}

	//src.Must1(src.Run(nil, os.Stdout, "streamlink", "https://www.twitch.tv/" + vid.Channel))
	var start_time string
	if input, err := stdin.ReadString('\n'); err != nil {
		fmt.Fprint(os.Stderr, "%s\n", err)
	} else {
		start_time = input[:len(input) - len("\n")]
	}

	if start_time == "" {
		if vid.Is_live {
			src.Must1(src.Run(nil, os.Stdout, "streamlink", "https://www.twitch.tv/" + vid.Channel))
		} else {
			src.Must1(src.Run(nil, os.Stdout, "streamlink", vid.Url))
		}
	} else {
		src.Must1(src.Run(nil, os.Stdout, "streamlink", "--hls-start-offset", start_time, vid.Url))
	}
}


func basic_menu(header string, option_count int, prompt string, print_option func (io.Writer, int)) (int, error) {
	if option_count == 0 {
		return -1, fmt.Errorf("No options available")
	} else if option_count == 1 {
		return 0, nil
	}

	max_index_digit_count := strconv.Itoa(len(strconv.Itoa(option_count)))
	for {
		fmt.Fprintf(os.Stderr, "%s", header)
		for i := 0; i < option_count; i += 1 {
			fmt.Fprintf(os.Stderr, "%" + max_index_digit_count + "d | ", i + 1)
			print_option(os.Stderr, i)
		}

		fmt.Fprintf(os.Stderr, "(Press Ctrl-c to quit)\n%s", prompt)
		stdin := bufio.NewReader(os.Stdin)

		var choice_as_str string
		if input, err := stdin.ReadString('\n'); err != nil {
			return -1, err
		} else {
			choice_as_str = input[:len(input) - len("\n")]
		}

		if choice, err := strconv.Atoi(choice_as_str); err != nil {
			fmt.Fprintf(os.Stderr, "%s%q is not a number%s\n", src.ANSI_FG_RED, choice_as_str, src.ANSI_RESET)
			continue
		} else if choice < 1 || choice > option_count {
			fmt.Fprintf(os.Stderr, "%s%q is not one of the options%s\n", src.ANSI_FG_RED, choice_as_str, src.ANSI_RESET)
		} else {
			return choice - 1, nil
		}
	}
}
