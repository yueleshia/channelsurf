package src

import (
	"time"
)

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
