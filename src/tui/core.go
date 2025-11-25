package tui

import (
	"io"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/uniseg"

	"github.com/yueleshia/streamsurf/src"
)

const (
	ScreenFollow int = iota
)

type UIState struct {
	Height, Width int
	Screen int
	Channel_list []string

	// Follow
	Follow_selection int
	Follow_videos []src.Video

	Message string
}

// @TODO: Cater for reloading a channel list when there are lines in CACHE.Latest
func (self *UIState) Load_follow_list(config string, cache *src.RingBuffer) {
	list := strings.Split(config, "\n")
	idx := 0
	// filter out empty string and trim "\r"
	for _, x := range list {
		s, _ := strings.CutSuffix(x, "\r")
		if "" != s {
			list[idx] = s
			idx += 1
		}
	}

	// @TODO: Refactor this to work even when we run out of cache
	//        Maybe this is resolved RingBuffer.Latest
	src.Assert(idx * src.PAGE_SIZE <= src.RING_QUEUE_SIZE)

	self.Channel_list = list[:idx]
	self.Follow_videos = make([]src.Video, idx)
	for i, channel := range list[:idx] {
		blank := src.Video{
			Channel: channel,
			Title: "Pending...",
		}
		self.Follow_videos[i] = blank
		cache.Latest[channel] = blank
	}
}

func Sort_videos_by_latest(a, b src.Video) int {
	less_than := false
	if a.Is_live && b.Is_live {
		// Shortest live first
		less_than = a.Duration < b.Duration
	} else if a.Is_live || b.Is_live {
		less_than = a.Is_live // Live first, vod after
	} else {
		a_close := a.Start_time.Add(a.Duration)
		b_close := b.Start_time.Add(b.Duration)
		// Latest close time first
		less_than = a_close.After(b_close)
	}
	if less_than {
		return -1
	} else {
		return 0
	}
}

func Print_formatted_line(output io.Writer, gap string, video src.Video) {
	sizes := []int{10, 30, 9, 6}

	var s_ago, duration string
	t_ago := time.Now().Sub(video.Start_time)

	if video.Start_time == (time.Time{}) {
	} else {
		if video.Is_live {
			s_ago = "○"
			duration = fmt.Sprintf("%dh%02dm", int(t_ago.Hours()), int(t_ago.Minutes()) % 60)
		} else {
			// @NOTE twitch streams are capped at 48 hours
			if int(t_ago.Minutes()) < 100 {
				s_ago = fmt.Sprintf("%d min ago", int(t_ago.Minutes()))
			} else if int(t_ago.Hours()) < 72 {
				s_ago = fmt.Sprintf("%d hr ago", int(t_ago.Hours()))
			} else {
				s_ago = fmt.Sprintf("%d d ago", int(t_ago.Hours() / 24))
			}

			duration = fmt.Sprintf("%dh%02dm", int(video.Duration.Hours()), int(video.Duration.Minutes()) % 60)
		}
	}

	print_line(output, gap, sizes, []string{video.Channel, video.Title, s_ago, duration})
}
func print_line(output io.Writer, gap string, sizes []int, cols []string) error {
	if len(sizes) != len(cols) {
		src.L_ERROR.Fatalf("Incorrect number of arguments")
	}

	for i, width := range sizes {
		str := cols[i]

		if (i != 0) {
			if _, err := io.Copy(output, strings.NewReader(gap)); err != nil {
				return err
			}
		}

		var to_print, extra string
		w := 0
		if is_ASCII(str) {
			w = len(str)
			if width < 3 {
				min_width := w
				if width < min_width {
					min_width = width
				}
				to_print = str[0:min_width]
				extra = "   "[0:3 - min_width]
			} else if w > width {
				to_print = str[0:width - 3]
				extra = "..."
			} else {
				to_print = str
			}
		} else {
			str_len := uniseg.StringWidth(str)

			if width < 3 {
				to_print, w = break_unicode_before(3, str)

			} else if str_len > width {
				to_print, w = break_unicode_before(width - 3, str)
				extra = "..."
			} else {
				to_print = str
				w = str_len
			}
		}

		if _, err := io.Copy(output, strings.NewReader(to_print)); err != nil {
			return err
		}
		if _, err := io.Copy(output, strings.NewReader(extra)); err != nil {
			return err
		}
		for i := w + len(extra); i < width; i += 1 {
			n, err := output.Write([]byte{' '})
			if n != 1 {
				return fmt.Errorf("Could not print")
			}
			if err != nil {
				return err
			}
		} 
	}
	n, err := output.Write([]byte{'\n'})
	if n != 1 {
		return fmt.Errorf("Could not print")
	}
	if err != nil {
		return err
	}
	return nil
}

func break_unicode_before(max int, str string) (string, int) {
	state := -1
	width := 0
	c := ""
	rest := str
	for len(rest) > 0 {
		var boundaries int
		c, rest, boundaries, state = uniseg.StepString(rest, state)
		to_add := boundaries >> uniseg.ShiftWidth

		if width + to_add > max {
			return str[:len(str) - len(rest) - len(c)], width
		}

		width += to_add
	}
	fmt.Print(width)
	return str[0:1], width
}

func is_ASCII(s string) bool {
	for i := 0; i < len(s); i++ {
	if s[i] > 128 {
		return false
		}
	}
	return true
}

