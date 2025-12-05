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

type FollowPair struct {
	Live   src.Video
	Latest src.Video
}

type UIState struct {
	Height, Width int
	Screen int
	Channel_list []string

	Cache LRU
	Refresh_queue chan src.VideoPacket
	Log_queue chan []byte

	// Follow screen
	Follow_latest map[string]FollowPair
	Follow_selection uint16
	Follow_videos []src.Video

	// Channel screen
	Channel string
	Channel_selection uint16
	Channel_videos RingBuffer
	Channel_command []byte

	Message strings.Builder
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

	self.Refresh_queue = make(chan src.VideoPacket, 100)
	self.Log_queue = make(chan []byte, 100)

	self.Follow_videos = set_len(self.Follow_videos, count)

	self.Channel_list = list[:count]
	self.Channel_videos.Buffer = set_len(self.Channel_videos.Buffer, count)
	self.Channel_command = set_len(self.Channel_command, 100)

	self.Cache.Buffer = set_len(self.Cache.Buffer, src.RING_QUEUE_SIZE)
	// You typically want 70% fullness for hash maps
	// Although we wrap around (so we could add up to 2 * RING_QUEUE_SIZE)
	// before deleting elements, we typically typically are only adding vidoes
	// 10 at a time, so 1.3 * BUFFER_SIZE would be enough
	if self.Cache.Exists == nil {
		self.Cache.Exists = make(map[string]int, src.RING_QUEUE_SIZE * 2)
	}
	if self.Follow_latest == nil {
		self.Follow_latest = make(map[string]FollowPair, count * 2)
	}

	for i, channel := range list[:count] {
		blank := src.Video{
			Channel: channel,
		}
		self.Follow_videos[i] = blank

		// @VOLATILE: Add_and_update_follow depends on this
		self.Follow_latest[channel] = FollowPair{blank, blank}
	}
}

const PACKETS_PER_REFRESH = 2

func Refresh_channels(queue chan src.VideoPacket, channels ...string) {
	for _, channel := range channels {
		go func() {
			vods, live := src.Graph_vods(channel)
			if live.Is_live {
				src.L_DEBUG.Printf("%s is live", live.Channel)
			}
			queue <- vods
			queue <- src.VideoPacket{[]src.Video{live}, true, nil}
		}()
		//go func() { queue <- src.Scrape_vods(channel) }()
		//go func() { queue <- src.Scrape_live_status(channel) }()
	}
}

func Print_formatted_line(output io.Writer, gap string, video src.Video) {
	sizes := []int{10, 30, 9, 6}

	var s_ago, title, duration string
	t_ago := time.Now().Sub(video.Start_time)

	if video.Start_time == (time.Time{}) {
		title = "Pending..."
	} else {
		title = video.Title
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

	print_line(output, gap, sizes, []string{video.Channel, title, s_ago, duration})
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

// Live packets are only stored in self.Follow_latest, not in the Cache.
func (self *UIState) Add_and_update_follow(packet src.VideoPacket) {
	if packet.Live {
		src.Assert(len(packet.Vids) == 1)
		vid := packet.Vids[0]
		if las, ok := self.Follow_latest[vid.Channel]; ok {
			self.Follow_latest[vid.Channel] = FollowPair{vid, las.Latest}
		}
	} else {
		for _, vid := range packet.Vids {
			self.Cache.Push(vid)

			// If one of the channels we follow
			if pair, ok := self.Follow_latest[vid.Channel]; ok {
				las := pair.Latest
				vid_close_time := vid.Start_time.Add(vid.Duration)
				las_close_time := las.Start_time.Add(las.Duration)

				if src.Is_similar_time(vid.Start_time, las.Start_time) {
					vid.Is_live = las.Is_live
					pair.Latest = vid
				} else if vid_close_time.After(las_close_time) {
					pair.Latest = vid
				}
				self.Follow_latest[vid.Channel] = pair
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
}
func (self *RingBuffer) Push(video src.Video) {
	length := len(self.Buffer)

	self.Buffer[self.Close % length] = video
	self.Close += 1
	if self.Close > length {
		self.Close = (self.Close % length) + length
		self.Start = self.Close
	}
	self.Start %= length
}

func (self *RingBuffer) Clear() {
	self.Start = 0
	self.Close = 0
}

func (self *RingBuffer) As_slice() []src.Video {
	if self.Close <= len(self.Buffer) {
		return self.Buffer[:self.Close]
	} else {
		return self.Buffer
	}
}

// Like an LRU-cache
// Implemented on a ring buffer instead of a doubly-linked list
// Invalidated items are replaced with the default
type LRU struct {
	Exists map[string]int
	RingBuffer
}
func (self *LRU) Push(video src.Video) {
	length := len(self.Buffer)

	to_add := 0
	// self.Close is always greater than length once we wrap around once
	if i, ok := self.Exists[video.Url]; ok {
		self.Buffer[i] = src.Video{} // Invalidate
	}

	idx := self.Close % length
	if self.Close >= length {
		to_add = length
		past := self.Buffer[idx]
		delete(self.Exists, past.Url)
		self.Start += 1
		self.Start %= length
	}
	self.Exists[video.Url] = idx
	self.Buffer[idx] = video
	self.Close = ((self.Close + 1) % length) + to_add
}

func (self *LRU) As_slice() []src.Video {
	if self.Close <= len(self.Buffer) {
		return self.Buffer[:self.Close]
	} else {
		return self.Buffer
	}
}

