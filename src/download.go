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
			} else {
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

	is_found := false
	if live.Is_live { // Not live is marked by Video{}, which is defualt false
		for i := len(vods) - 1; i >= 0; i -= 1 {
			delta := vods[i].Start_time.Sub(live.Start_time)
			if -5 * time.Minute < delta &&  delta < 5 * time.Minute {
				vods[i].Is_live = true
				is_found = true
				break
			}
		}
	}

	cache.Mutex.Lock()
	cache.Add(vods)
	if !is_found && live.Is_live {
		cache.Add([]Video{live})
	}
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

	if r.Close >= length {
		for i := 0; i < len(items); i += 1 {
			delete(r.Latest, r.Buffer[(r.Close + i) % length].Url)
		}
	}
	for _, vid := range items {
		if _, ok := r.Latest[vid.Url]; ok {
			continue
		} else {
			r.Latest[vid.Url] = vid
		}
		r.Buffer[r.Close % length] = vid
		r.Close += 1
	}
	if r.Close > length {
		r.Close = (r.Close % length) + length
		r.Start = r.Close
	}
	r.Start %= length
}

