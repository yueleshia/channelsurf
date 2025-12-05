package tui

import (
	"bufio"
	"context"
	"fmt"
	"slices"
	"strings"
	"os"

	"io"
	"os/exec"

	xterm "golang.org/x/term"

	"github.com/yueleshia/streamsurf/src"
	"github.com/yueleshia/streamsurf/src/term"
)

//run: go run ../../main.go

func streamlink(ctx context.Context, output chan []byte, args ...string) error {
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
	}

	go stream_input(output, stdout)
	go stream_input(output, stderr)
	return cmd.Wait()
}

func (self *UIState) Interactive() {
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
	self.Refresh_queue = make(chan src.VideoPacket, 100)
	Refresh_channels(self.Refresh_queue, self.Channel_list...)

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

		case message := <-self.Log_queue:
			fmt.Println("hello")
			_, _ = self.Message.Write(message)

		case packet := <-self.Refresh_queue:
			if packet.Err != nil {
				_, _ = self.Message.WriteString(packet.Err.Error())
				_ = self.Message.WriteByte('\n')
			} else {
				self.Add_and_update_follow(packet)
				if self.Message.String() != "Refreshed\n" {
					_, _  = self.Message.WriteString("Refreshed\n")
				}
			}
			slices.SortFunc(self.Follow_videos, src.Sort_videos_by_latest)

			switch (self.Screen) {
			case ScreenFollow: self.follow_swap()
			case ScreenChannel: self.channel_swap(self.Channel)
			default: panic("DEV: Unsupport screen")
			}

		case event := <- input_queue:
			is_break := false
			switch (self.Screen) {
			case ScreenFollow: is_break = self.follow_input(event, cancel)
			case ScreenChannel: is_break = self.channel_input(event, cancel)
			default: panic("DEV: Unsupport screen")
			}

			if is_break {
				break main_loop
			}
		}

		render(writer, *self)
	}
}

func render(writer *bufio.Writer, ui UIState) {
	fmt.Fprint(writer, term.Clear + "\x1B[1;1H")
	switch ui.Screen {
	case ScreenFollow: ui.follow_render(writer)
	case ScreenChannel: ui.channel_render(writer)
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

func render_message(writer *bufio.Writer, message string) {
	for part := range strings.SplitSeq(message, "\n") {
		fmt.Fprintf(writer, "%s\r\n", part)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Follow screen

func (self *UIState) follow_swap() {
	self.Screen = ScreenFollow

	// @VOLATILE: Load_config seeds the keys
	var idx uint = 0
	for _, pair := range self.Follow_latest {
		if pair.Live.Duration > 0 {
			self.Follow_videos[idx] = pair.Live
		} else {
			self.Follow_videos[idx] = pair.Latest
		}
		idx += 1
	}
	slices.SortFunc(self.Follow_videos, src.Sort_videos_by_latest)
}

func (self *UIState) follow_input(event term.Event, cancel context.CancelFunc) bool {
	self.Message.Reset()
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
			Refresh_channels(self.Refresh_queue, self.Channel_list...)
			
		case 'j':
			if int(self.Follow_selection) + 1 < len(self.Follow_videos) {
				self.Follow_selection += 1
			}
		case 'k':
			if int(self.Follow_selection) > 0 {
				self.Follow_selection -= 1
			}
		case 'l':
			vid := self.Follow_videos[self.Follow_selection]
			self.channel_swap(vid.Channel)

		default:
			self.Message.WriteString(fmt.Sprintf("%d %+v\n", event.Ty, event))
		}
	default:
		self.Message.WriteString(fmt.Sprintf("%d %+v\n", event.Ty, event))
	}
	return false
}

func (self UIState) follow_render(writer *bufio.Writer) {
	height_left := self.Height
	fmt.Fprint(writer, "Follow\n")
	height_left -= 1

	to_render := self.Follow_videos
	if len(to_render) < height_left - 2 {
		to_render = to_render[:len(to_render)]
	}
	render_video_list(writer, self.Follow_selection, to_render)

	fmt.Fprintf(writer, "\r\n (q)uit (r)efresh (hjkl) navigate")
	fmt.Fprintf(writer, "\r\n")
	fmt.Fprintf(writer, "\r\nui_selection: %d\r\n", self.Follow_selection)
	fmt.Fprintf(writer, "\r\n")
	render_message(writer, self.Message.String())
}

////////////////////////////////////////////////////////////////////////////////
// Channel screen

func (self *UIState) channel_swap(channel string) {
	self.Screen = ScreenChannel
	self.Channel = channel
	self.Channel_command = self.Channel_command[:0]

	self.Channel_videos.Clear()
	if pair, ok := self.Follow_latest[channel]; ok && pair.Live.Duration > 0 {
		self.Channel_videos.Push(pair.Live)
	}
	for _, vid := range self.Cache.As_slice() {
		if vid.Channel == self.Channel {
			self.Channel_videos.Push(vid)
		}
	}
	slices.SortFunc(self.Channel_videos.As_slice(), src.Sort_videos_by_latest)
}

func (self *UIState) channel_input(event term.Event, cancel context.CancelFunc) bool {
	self.Message.Reset()
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
			Refresh_channels(self.Refresh_queue, self.Channel)
			
		case 'h':
			for i, vid := range self.Follow_videos {
				if vid.Channel == self.Channel {
					self.Follow_selection = uint16(i)
					break
				}
			}
			self.Screen = ScreenFollow

		case 'j':
			if int(self.Channel_selection) + 1 < len(self.Channel_videos.Buffer) {
				self.Channel_command = self.Channel_command[:0] // Clear time selection
				self.Channel_selection += 1
			}
		case 'k':
			if self.Channel_selection > 0 {
				self.Channel_command = self.Channel_command[:0] // Clear time selection
				self.Channel_selection -= 1
			}
		case 'l':
			if len(self.Channel_videos.Buffer) > 0 {
				ctx, cancel := context.WithCancel(context.Background())
				vid := self.Channel_videos.Buffer[self.Channel_selection]

				if vid.Is_live || len(self.Channel_command) == 0 {
					_, _ = self.Message.WriteString(fmt.Sprintf("Playing %s\n", vid.Url))
					go streamlink(ctx, self.Log_queue, vid.Url)
				} else {
					_, _ = self.Message.WriteString(fmt.Sprintf("Playing %s at %s\n", vid.Url, self.Channel_command))
					go streamlink(ctx, self.Log_queue, vid.Url, "--hls-start-offset", string(self.Channel_command))
				}
				// @TODO: Track if video is currently playing, and close it if we reopen. Maybe this is undesired behaviour?
				_ = cancel
			}
		case '0','1','2','3','4','5','6','7','8','9', ':':
			vid := self.Channel_videos.Buffer[self.Channel_selection]

			if !vid.Is_live {
				self.Channel_command = append(self.Channel_command, byte(event.X))
			}

		case 127:
			length := len(self.Channel_command)
			if length > 0 {
				self.Channel_command = self.Channel_command[:length - 1]
			}

		default:
		}
	default:
		self.Message.WriteString(fmt.Sprintf("%d %+v\n", event.Ty, event))
		self.Message.WriteString(self.Channel_videos.Buffer[self.Channel_selection].Url)
		_ = self.Message.WriteByte('\n')
	}
	return false
}

func (self UIState) channel_render(writer *bufio.Writer) {
	height_left := self.Height
	fmt.Fprintf(writer, "Channel %s\n", self.Channel)
	height_left -= 1

	to_render := self.Channel_videos.As_slice()
	if len(to_render) < height_left - 2 {
		to_render = to_render[:len(to_render)]
	}
	render_video_list(writer, self.Channel_selection, to_render)

	// Display play time
	vid := self.Channel_videos.Buffer[self.Channel_selection]
	// On live videos hide this selection, because you will be on live
	if !vid.Is_live && len(self.Channel_command) > 0 {
		fmt.Fprintf(writer, "\r\n Length (hh:mm:ss): %s\r\n", string(self.Channel_command))
	}

	fmt.Fprintf(writer, "\r\n (q)uit (r)efresh (hjkl) navigate")
	fmt.Fprintf(writer, "\r\n")
	fmt.Fprintf(writer, "\r\n%s", vid.Url)
	fmt.Fprintf(writer, "\r\n%s", vid.Title)
	fmt.Fprintf(writer, "\r\nChapters: ")
	for i, chapter := range vid.Chapters {
		if i != 0 {
			fmt.Fprintf(writer, "%s", " | ")
		}
		fmt.Fprintf(writer, "%s", chapter.Name)
	}
	fmt.Fprintf(writer, "\r\n")
	render_message(writer, self.Message.String())
}
