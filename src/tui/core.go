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
	ScreenChannel
)

type UIState struct {
	Height, Width int
	Screen int
	Channel_list []string

	Cache RingBuffer
	Refresh_queue chan src.Result[[]src.Video]

	// Follow screen
	Follow_latest map[string]src.Video
	Follow_selection uint16
	Follow_videos []src.Video

	// Channel screen
	Channel string
	Channel_selection uint16
	Channel_videos RingBuffer

	Message string
}

func set_len[T any](slice []T, target_len int) []T {
	if len(slice) == target_len {
		return slice
	}

	if target_len <= cap(slice) {
		var uinit T
		for i := len(slice); i < target_len; i += 1 {
			slice = append(slice, uinit)
		}
		return slice[:target_len]
	} else {
		ret := make([]T, target_len)
		copy(ret, slice)
		return ret
	}
}

// @TODO: Cater for reloading a channel list when there are lines in CACHE.Latest
func (self *UIState) Load_config(config string) {
	list := strings.Split(config, "\n")
	count := 0
	// filter out empty string and trim "\r"
	for _, x := range list {
		s, _ := strings.CutSuffix(x, "\r")
		if "" != s {
			list[count] = s
			count += 1
		}
	}

	// @TODO: Refactor this to work even when we run out of cache
	//        Maybe this is resolved RingBuffer.Latest
	src.Assert(count * src.PAGE_SIZE <= src.RING_QUEUE_SIZE)

	self.Channel_list = list[:count]
	self.Follow_videos = set_len(self.Follow_videos, count)
	self.Channel_videos.Buffer = set_len(self.Channel_videos.Buffer, count)

	self.Cache.Buffer = set_len(self.Cache.Buffer, src.RING_QUEUE_SIZE)
	if self.Follow_latest == nil {
		// You typically want 70% fullness for hash maps
		// Although we wrap around (so we could add up to 2 * RING_QUEUE_SIZE)
		// before deleting elements, we typically typically are only adding vidoes
		// 10 at a time, so 1.3 * BUFFER_SIZE would be enough
		self.Follow_latest = make(map[string]src.Video, src.RING_QUEUE_SIZE * 2)
	}

	for i, channel := range list[:count] {
		blank := src.Video{
			Channel: channel,
			Title: "Pending...",
		}
		self.Follow_videos[i] = blank

		// @VOLATILE: Add_and_update_follow depends on this
		self.Follow_latest[channel] = blank
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

//run: go run ../../main.go

////////////////////////////////////////////////////////////////////////////////

func (self *UIState) Add_and_update_follow(videos []src.Video) {
	self.Cache.Add(videos)
	// @VOLATILE: Depends on Update_config seeding Follow_latest with the channel names
	for _, vid := range videos {
		// If one of the channels we follow
		if las, ok := self.Follow_latest[vid.Channel]; ok {
			vid_close_time := vid.Start_time.Add(vid.Duration)
			las_close_time := las.Start_time.Add(las.Duration)

			if vid.Is_live {
				self.Follow_latest[vid.Channel] = vid
			} else if src.Is_similar_time(vid.Start_time, las.Start_time) {
				vid.Is_live = las.Is_live
				self.Follow_latest[vid.Channel] = vid
			} else if vid_close_time.After(las_close_time) {
				self.Follow_latest[vid.Channel] = vid
			}
		}
	}
}


// @TODO: Check if using a BTreeMap would be faster than a ring buffer
//        This is relevant for the channel list in interactive

// Our basic algorithm is start..close are valid index
// Once close has passed len(Buffer), it will always be len + idx, thus close is
// always a > start. It is on the usage code to do a modulus.
type RingBuffer struct {
	Buffer []src.Video
	Start int
	Close int
	Wrapped bool
}
func (self *RingBuffer) Add(videos []src.Video) {
	length := len(self.Buffer)

	//if self.Close >= length {
	//	for i := 0; i < len(videos); i += 1 {
	//		delete(self.Latest, self.Buffer[(self.Close + i) % length].Url)
	//	}
	//}
	for _, vid := range videos {
		// @VOLATILE: Channel_videos requires added batches to be internally sorted. This is sorted at network ingress
		src.Assert(past.Start_time == vid.Start_time || past.Start_time.Before(vid.Start_time))

		//if _, ok := self.Latest[vid.Url]; ok {
		//	continue
		//} else {
		//	self.Latest[vid.Url] = vid
		//}
		self.Buffer[self.Close % length] = vid
		self.Close += 1
	}
	if self.Close > length {
		self.Close = (self.Close % length) + length
		self.Start = self.Close
	}
	self.Start %= length
}

func (self *RingBuffer) As_slice() []src.Video {
	if self.Close <= len(self.Buffer) {
		return self.Buffer[:self.Close]
	} else {
		return self.Buffer
	}
}
