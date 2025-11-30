package src

import (
	"time"
)

// Customise runtime behaviour
const IS_CLEAR = false // Remove old local files
const IS_LOCAL = false // Use local files on second run instead of issuing network requests

var CLIENT_ID = "ue6666qo983tsx6so1t0vnawi233wa" // old: kimne78kx3ncx6brgo4mv6wki5h1ko
var USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36"

const RING_QUEUE_SIZE int = 10000
const PAGE_SIZE = 20

const ANSI_FG_RED = "\x1b[31m"
const ANSI_FG_GREEN = "\x1b[32m"
const ANSI_FG_CYAN = "\x1b[36m"
const ANSI_RESET = "\x1b[0m"

type Video struct {
	Title         string
	Channel       string
	Description   string
	Thumbnail_URL []string
	Start_time    time.Time
	Duration      time.Duration
	Is_live       bool
	Url           string
}

func Sort_videos_by_latest(a, b Video) int {
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


//func Is_start_time_before(a, b Video) int {
//	if a.Start_time.Before(b.Start_time) {
//		return -1
//	} else {
//		return 0
//	}
//}
