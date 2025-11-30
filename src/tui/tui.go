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
	
	stdin_fd := int(os.Stdin.Fd())

	if w, h, err := xterm.GetSize(stdin_fd); err != nil {
		return
	} else {
		self.Width = w
		self.Height = h
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//events := make(chan term.Event, 1000)

	var old_state *xterm.State
	if st, err := xterm.MakeRaw(stdin_fd); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot enter term raw mode and thus cannot use the TUI-mode. Use this as a CLI. Type --help for more information.")
		return
	} else {
		old_state = st
	}
	defer func () {
		err := xterm.Restore(stdin_fd, old_state)
		_ = err
	}()
	_ = src.Must(writer.WriteString(term.Enter_alt_buffer + "\x1B[1;1H" + term.Hide_cursor))
	defer func() {
		_ = src.Must(writer.WriteString(term.Leave_alt_buffer +  term.Show_cursor))
		src.Must1(writer.Flush())
	}()

	//// Busy loop if set to non-blocking
	//if err := term.Sys_set_nonblock(stdin_fd, true); err != nil {
	//	return
	//}

	////////////////////////////////////////////////////////////////////////////
	// Setup inital screen

	render(writer, *self)
	src.Must1(writer.Flush())

	refresh_queue := make(chan bool, 100)
	self.Refresh_queue = make(chan src.Result[[]src.Video], 100)
	self.refresh_channels(self.Channel_list...)

	// Setup input loop
	// We do not want tob lock the main loop, so that we can have async updates
	// But we also cannot set the stdin to be non-blocking so we do not busy loop
	input_queue_size := 32
	input_queue := make(chan term.Event, input_queue_size)
	go func () {
		var buffer [32]byte
		for {
			var parser term.InputParser
			if n, err := term.Sys_read(stdin_fd, buffer[:]); err != nil || n < 1 {
				continue
			} else {
				parser = buffer[:n]
			}
			for {
				if evt := parser.Next(); evt == nil {
					break
				} else if evt == nil {
					break
				} else {
					input_queue <- *evt
				}
			}
		}
	}()

	////////////////////////////////////////////////////////////////////////////

	main_loop: for {
		select {
		case <-ctx.Done(): break main_loop
		case <-refresh_queue:

		case x := <-self.Refresh_queue:
			if videos, err := x.Val, x.Err; err != nil {
				self.Message = self.Message + "\r\n" + err.Error()
			} else {
				cache.Add(videos)
			}

			idx := 0
			for _, vid := range cache.Latest {
				self.Follow_videos[idx] = vid
				idx += 1
			}
			slices.SortFunc(self.Follow_videos, Sort_videos_by_latest)
		case event := <- input_queue:
			is_break := false
			switch (self.Screen) {
			case ScreenFollow: is_break = self.screen_follow_input(event, cancel)
			default: panic("DEV: Unsupport screen")
			}

			if is_break {
				break main_loop
			}
		}

		render(writer, *self)
	}
}
func (self *UIState) refresh_channels(channels ...string) {
	for _, channel := range self.Channel_list {
		go func() { self.Refresh_queue <- src.Graph_vods(channel) }()
		go func() { self.Refresh_queue <- src.Scrape_live_status(channel) }()
	}
}

func render(writer *bufio.Writer, ui UIState) {
	fmt.Fprint(writer, term.Clear + "\x1B[1;1H")
	switch ui.Screen {
	case ScreenFollow: ui.screen_follow_render(writer)
	default: panic("DEV: Unsupport screen")
	}
	src.Must1(writer.Flush())
}

func render_video_list(writer *bufio.Writer, selection uint16, videos []src.Video) {
	for i := uint16(0); int(i) < len(videos); i += 1 {
		fmt.Fprintf(writer, "\x1B[%d;1H", i + 2)
		if i == selection {
			fmt.Fprintf(writer, "\x1B[0;%s%s;%s%sm", term.Part_foreground, term.Part_white, term.Part_background, term.Part_black)
		}
		Print_formatted_line(writer, " | ", videos[i])
		if i == selection {
			fmt.Fprintf(writer, term.Reset_attributes)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// Follow screen

func (self *UIState) screen_follow_input(event term.Event, cancel context.CancelFunc) bool {
	self.Message = ""
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

		case 'r':
			self.refresh_channels(self.Channel_list...)
			
		case 'j':
			if int(self.Follow_selection) + 1 < len(self.Follow_videos) {
				self.Follow_selection += 1
			}
		case 'k':
			if int(self.Follow_selection) > 0 {
				self.Follow_selection -= 1
			}
		case 'l':
			if len(self.Follow_videos) > 0 {
				ctx, cancel := context.WithCancel(context.Background())
				vid := self.Follow_videos[self.Follow_selection]
				if vid.Is_live {
					go streamlink(ctx, "https://www.twitch.tv/" + vid.Channel)
				}
				_ = cancel
			}
		case '0','1','2','3','4','5','6','7','8','9':

		default:
			self.Message = fmt.Sprintf("%d %+v", event.Ty, event)
		}
	default:
		self.Message = fmt.Sprintf("%d %+v", event.Ty, event)
	}
	return false
}

func (self UIState) screen_follow_render(writer *bufio.Writer) {
	height_left := self.Height
	fmt.Fprint(writer, "Follow\n")
	height_left -= 1

	to_render := self.Follow_videos
	if len(to_render) < height_left - 2 {
		to_render = to_render[:len(to_render)]
	}
	render_video_list(writer, self.Follow_selection, to_render)

	fmt.Fprintf(writer, "\rui_selection: %d", self.Follow_selection)
	fmt.Fprintf(writer, "\n\r%s", self.Message)
}

