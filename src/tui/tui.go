package tui

import (
	"bufio"
	"context"
	"fmt"
	"slices"
	"os"

	"bytes"
	"io"
	"os/exec"

	xterm "golang.org/x/term"

	"github.com/yueleshia/streamsurf/src"
	"github.com/yueleshia/streamsurf/src/term"
)

//run: go run ../../main.go

var STDIN_FD int

func streamlink(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "streamlink", args...)

	var stdout, stderr io.ReadCloser
	if pipe, err := cmd.StdoutPipe(); err != nil {
		return err
	} else {
		stdout = pipe
	}
	if pipe, err := cmd.StderrPipe(); err != nil {
		return err
	} else {
		stderr = pipe
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stream_input := func (channel chan []byte, pipe io.ReadCloser) {
		buffer := make([]byte, 1024)
		for {
			bytes_read, err := pipe.Read(buffer)
			if bytes_read > 0 {
				s := make([]byte, bytes_read)
				copy(s, buffer[:bytes_read])
				channel <- s
			}
			if err != nil {
				break
			}
		}
		channel <- nil
	}

	messages := make(chan []byte)
	go stream_input(messages, stdout)
	go stream_input(messages, stderr)

	count := 2
	var output = make([]byte, 0, 4096) // Streamlink with no errors is prety short
	for {
		msg := <-messages
		if msg == nil {
			count -= 1
			if count == 0 {
				break
			}
		}
		output = append(output, msg...)
		idx := 0
		for j := 0; j > 0; j = bytes.IndexByte(output[idx:], '\n') {
			fmt.Fprint(os.Stderr, string(output[idx:][:j]))
			fmt.Fprint(os.Stderr, "\r")
			idx += j
		}
		fmt.Fprint(os.Stderr, string(output[idx:]))
	}

	return cmd.Wait()
}

func (self *UIState) Interactive(cache *src.RingBuffer) {
	////////////////////////////////////////////////////////////////////////////
	// Setup
	writer := bufio.NewWriter(os.Stdout)
	
	if w, h, err := xterm.GetSize(STDIN_FD); err != nil {
		return
	} else {
		self.Width = w
		self.Height = h
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//events := make(chan term.Event, 1000)

	var old_state *xterm.State
	if st, err := xterm.MakeRaw(STDIN_FD); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot enter term raw mode and thus cannot use the TUI-mode. Use this as a CLI. Type --help for more information.")
		return
	} else {
		old_state = st
	}
	defer func () {
		err := xterm.Restore(STDIN_FD, old_state)
		_ = err
	}()
	_ = src.Must(writer.WriteString(term.Enter_alt_buffer + "\x1B[1;1H" + term.Hide_cursor))
	defer func() {
		_ = src.Must(writer.WriteString(term.Leave_alt_buffer +  term.Show_cursor))
		src.Must1(writer.Flush())
	}()

	if err := term.Sys_set_nonblock(STDIN_FD, true); err != nil {
		return
	}

	////////////////////////////////////////////////////////////////////////////
	// Setup inital screen

	render(writer, *self)
	refresh_queue := make(chan bool, 100)
	for _, channel := range self.Channel_list {
		go func() {
			cache.Query_channel(channel)
			refresh_queue <- true
		}()
	}

	////////////////////////////////////////////////////////////////////////////
	// Main loop
	input_buffer := make([]byte, 32)
	src.Must1(writer.Flush())
	outer: for {
		select {
		case <-ctx.Done(): break outer
		case <-refresh_queue:
			idx := 0
			for _, vid := range cache.Latest {
				self.Follow_videos[idx] = vid
				idx += 1
			}
			slices.SortFunc(self.Follow_videos, Sort_videos_by_latest)

			self.Message = "Refreshed"
		default:
			var buf []byte
			if n, err := term.Sys_read(STDIN_FD, input_buffer); err != nil || n < 1 {
				continue
			} else {
				buf = input_buffer[:n]
			}


			var parser term.InputParser = buf
			var event term.Event
			for {
				if evt := parser.Next(); evt == nil {
					break
				} else {
					event = *evt
				}

				switch (self.Screen) {
				case ScreenFollow:
					if self.screen_follow_input(event, cancel) {
						break
					}
				default: panic("DEV: Unsupport screen")
				}
			}
		}

		render(writer, *self)
	}
}

func (ui *UIState) screen_follow_input(event term.Event, cancel context.CancelFunc) bool {
	ui.Message = ""
	switch event.Ty {
	case term.TyCodepoint:
		switch event.X {
		case 'c':
			if event.Mod_ctrl {
				cancel()
				return true
			}
		case 'q':
			cancel()
			return true
		case 'j':
			if ui.Follow_selection < len(ui.Follow_videos) {
				ui.Follow_selection += 1
			}
		case 'k':
			if ui.Follow_selection > 0 {
				ui.Follow_selection -= 1
			}
		case 'l':
			if len(ui.Follow_videos) > 0 {
				ctx, cancel := context.WithCancel(context.Background())
				vid := ui.Follow_videos[ui.Follow_selection]
				if vid.Is_live {
					go streamlink(ctx, "https://www.twitch.tv/" + vid.Channel)
				}
				_ = cancel
			}

		default:
			ui.Message = fmt.Sprintf("%d %+v", event.Ty, event)
		}
	default:
		ui.Message = fmt.Sprintf("%d %+v", event.Ty, event)
	}
	return false
}

func (ui UIState) screen_follow_render(writer *bufio.Writer) {
	height_left := ui.Height
	fmt.Fprint(writer, "Follow\n")
	height_left -= 1

	max_count := height_left
	if len(ui.Follow_videos) < max_count {
		max_count = len(ui.Follow_videos)
	}
	for i := 0; i < max_count; i += 1 {
		fmt.Fprintf(writer, "\x1B[%d;1H", i + 2)
		if i == ui.Follow_selection {
			fmt.Fprintf(writer, "\x1B[0;%s%s;%s%sm", term.Part_foreground, term.Part_white, term.Part_background, term.Part_black)
		}
		Print_formatted_line(writer, " | ", ui.Follow_videos[i])
		if i == ui.Follow_selection {
			fmt.Fprintf(writer, term.Reset_attributes)
		}
		height_left -= 1
	}
	fmt.Fprintf(writer, "\rui_selection: %d", ui.Follow_selection)
	fmt.Fprintf(writer, "\n\r%s", ui.Message)
}


func render(writer *bufio.Writer, ui UIState) {
	fmt.Fprint(writer, term.Clear + "\x1B[1;1H")
	switch ui.Screen {
	case ScreenFollow:
		ui.screen_follow_render(writer)
	}
	src.Must1(writer.Flush())
}

