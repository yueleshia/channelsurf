package src

import (
	"time"
	"sync"
)

func (cache *RingBuffer) Query_channel(channel string) {
	var vods []Video
	var live Video

	jobs := [] func() {
		func () {
			if vids, err := Graph_vods(channel); err != nil {
				//fmt.Println(err)
				L_DEBUG.Println(err)
				// @TODO: display error
			} else {
				vods = vids
			}
		},
		func () {
			if x, err := Scrape_live_status(channel); err != nil {
				//fmt.Println(err)
				L_DEBUG.Println(err)
				// @TODO: display error
				live = Video {
					Channel: channel,
				}
			} else {
				L_DEBUG.Printf("%q is live\n", channel)
				live = x
			}
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(jobs))
	for _, x := range jobs {
		go func () {
			x()
			wg.Done()
		}()
	}
	wg.Wait()

	latest_idx := -1
	latest := live // Video{} or an actual video
	for i, vod := range vods {
		vod_close_time := vod.Start_time.Add(vod.Duration)
		lat_close_time := latest.Start_time.Add(latest.Duration)
		if vod_close_time.After(lat_close_time) {
			latest = vod
			latest_idx = i
		}
	}

	cache.Mutex.Lock()
	if _, ok := cache.Latest[channel]; ok {
		cache.Latest[channel] = latest
	}
	if live.Is_live && latest_idx >= 0 { // Not live is marked by zero value Video{} (default false)
		delta := vods[latest_idx].Start_time.Sub(live.Start_time)
		if -5 * time.Minute < delta && delta < 5 * time.Minute {
			vods[latest_idx].Is_live = true
		} else {
			cache.Add([]Video{live})
		}
	}
	cache.Add(vods)
	cache.Mutex.Unlock()
}



////////////////////////////////////////////////////////////////////////////////

type RingBuffer struct {
	Latest map[string]Video
	Buffer []Video
	Start int
	Close int
	Mutex sync.Mutex
	Wrapped bool
}
func (r *RingBuffer) Add(items []Video) {
	length := len(r.Buffer)

	//if r.Close >= length {
	//	for i := 0; i < len(items); i += 1 {
	//		delete(r.Latest, r.Buffer[(r.Close + i) % length].Url)
	//	}
	//}
	for _, vid := range items {
		//if _, ok := r.Latest[vid.Url]; ok {
		//	continue
		//} else {
		//	r.Latest[vid.Url] = vid
		//}
		r.Buffer[r.Close % length] = vid
		r.Close += 1
	}
	if r.Close > length {
		r.Close = (r.Close % length) + length
		r.Start = r.Close
	}
	r.Start %= length
}

