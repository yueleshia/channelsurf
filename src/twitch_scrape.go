package src

//run: go run ../main.go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Most frontend frameworks will serialise page-specific data into a packet
// For twitch this happens to be:
//   pup 'head script[type="application/ld+json"] text{}'
func read_twitch_frontend_packet(input io.ReadCloser) ([]byte, error) {
	z := html.NewTokenizer(input)
	const (
		START uint = iota
		HEAD
		PACKET
		DONE
	)
	state := START

	outer_loop: for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return nil, z.Err()
		case html.TextToken:
			if state == PACKET {
				return z.Text(), nil
			}
		case html.StartTagToken:
			tag_name, has_attrs := z.TagName()
			//fmt.Println(is_in_head, string(tag_name))
			if state == START && bytes.Equal(tag_name, []byte("head")) {
				state = HEAD
			} else if state == HEAD && has_attrs && bytes.Equal(tag_name, []byte("script")) {
				for {
					key, val, has_more_attr := z.TagAttr()
					if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("application/ld+json")) {
						state = PACKET
					}
					if !has_more_attr {
						break
					}
				}
			}
		case html.EndTagToken:
			tag_name, _ := z.TagName()
			if bytes.Equal(tag_name, []byte("head")) {
				state = DONE
			} else if bytes.Equal(tag_name, []byte("html")) {
				break outer_loop
			}
		}
	}
	return nil, ErrMissing { message: "Packet not found" }
}


// You probably have to run this several times since it stochastically curls nothing
func Scrape_vods(channel string) ([]Video, error) {
	Assert(strings.Index(channel, "/") == -1)
	videos := [10]Video{}

	body, err := Request(context.TODO(), "GET", nil, nil, "https://twitch.tv/" + channel + "/videos", fmt.Sprintf("scrape-%s-videos", channel))
	
	if err != nil {
		return videos[:0], err
	}
	ret, err := func () ([]Video, error) {
		var live_data []byte
		if x, err := read_twitch_frontend_packet(body); err != nil {
			return videos[:0], err

		} else {
			live_data = x
			//fmt.Println(string(live_data))
		}

		type GraphQL struct {
			Context string            `json:"@context"`
			Graph   [2]json.RawMessage `json:"@graph"`
		}

		type VideoObject struct {
			Type          string          `json:"@type"`
			Name          string          `json:"name"`
			Url           string          `json:"url"`
			Description   string          `json:"description"`
			Thumbnail_URL []string        `json:"thumbnailUrl"`
			Upload_date   string          `json:"uploadDate"`
			Duration      string          `json:"duration"`
			Position      uint            `json:"position"`
			Stats         json.RawMessage `json:"interactionStatistic"`
			Embed_url     string          `json:"embedUrl"`
		}
		type ItemList struct {
			Type              string        `json:"@type"`
			Item_list_element []VideoObject `json:"itemListElement"`
		}

		idx := 0
		var x1 GraphQL
		{
			dec := json.NewDecoder(bytes.NewBuffer(live_data))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&x1); err != nil {
				return videos[:0], err
			}
		}

		var x2 ItemList
		{
			dec := json.NewDecoder(bytes.NewBuffer(x1.Graph[1]))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&x2); err != nil {
				return videos[:0], err
			}
		}
		// Reverse so that our QUEUE overwrites older VODs
		for i := len(x2.Item_list_element) - 1; i >= 0; i -= 1 {
			x := x2.Item_list_element[i]
			var start, close time.Time

			if parsed_url, err := url.Parse(x.Url); err != nil {
				L_DEBUG.Printf("Failed to parse url: %q", x.Url)
				continue
			} else if _, ok := strings.CutPrefix(parsed_url.Path, "/videos"); ok {
			} else {
				L_TRACE.Printf("Skipping non-VOD: %q", x.Url)
				continue
			}

			if x, err := time.Parse(time.RFC3339, x.Upload_date); err != nil {
				return videos[:0], err
			} else {
				start = x
			}

			if post_pt, ok := strings.CutPrefix(x.Duration, "PT"); !ok {
				L_DEBUG.Printf("Failed to parse start time for %q: %s", x.Url, x.Upload_date)
				continue
			} else {
				if dur, err := time.ParseDuration(strings.ToLower(post_pt)); err != nil {
					L_DEBUG.Printf("Failed to parse duration for %q: %s", x.Url, x.Duration)
				} else {
					close = start.Add(dur)
				}
			}

			videos[idx] = Video {
				Title: x.Name,
				Channel: channel,
				Description: x.Description,
				Thumbnail_URL: x.Thumbnail_URL,
				Start_time: start,
				Duration: close.Sub(start),
				Is_live: false,
				Url: x.Url,
			}
			idx += 1
			if idx >= 10 {
				break
			}
		}
		return videos[:idx], nil
	}()
	if err != nil {
		return videos[:0], err
	}
	return ret, body.Close()
}



func Scrape_live_status(channel string) (Video, error) {
	Assert(strings.Index(channel, "/") == -1)
	var undef Video

	channel_url := "https://twitch.tv/" + channel
	body, err := Request(context.TODO(), "GET", nil, nil, channel_url, fmt.Sprintf("scrape-%s", channel))
	if err != nil {
		return undef, err
	}
	ret, err := func () (Video, error) {
		var live_data []byte
		if x, err := read_twitch_frontend_packet(body); err != nil {
			if _, ok := err.(ErrMissing); ok {
				return undef, ErrMissing { message: channel + " is not live" }
			} else {
				return undef, err
			}
		} else {
			live_data = x
			//fmt.Println(string(live_data))
		}

		type Publication struct {
			Type              string `json:"@type"`
			End_date          string `json:"endDate"`
			Start_date        string `json:"startDate"`
			Is_live_broadcast bool   `json:"isLiveBroadcast"`
		}

		type Obj struct {
			Type string `json:"@type"`
			Description string `json:"description"` 
			Embed_url string `json:"embedUrl"`
			Name string `json:"name"`
			Thumbnail_URL []string `json:"thumbnailUrl"`
			UploadDate string`json:"uploadDate"`
			Publication Publication `json:"publication"`
		}
		type GraphQL struct {
			Context string `json:"@context"`
			Graph   [1]Obj `json:"@graph"`
		}

		var x GraphQL
		dec := json.NewDecoder(bytes.NewBuffer(live_data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&x); err != nil {
			return undef, err
		}
		//fmt.Println(x)

		// @TODO: error check around len
		node := x.Graph[0]
		var start, close time.Time
		//"2025-11-12T15:06:12Z"
		if x, err := time.Parse(time.RFC3339, node.Publication.Start_date); err != nil {
			return undef, err
		} else {
			start = x
		}
		if x, err := time.Parse(time.RFC3339, node.Publication.End_date); err != nil {
			return undef, err
		} else {
			close = x
		}
		return Video{
			Title: node.Description,
			Channel: channel,
			Description: node.Name,
			Thumbnail_URL: node.Thumbnail_URL,
			Start_time: start,
			Duration: close.Sub(start),
			Is_live: node.Publication.Is_live_broadcast,
			Url: channel_url,
		}, nil
	}()
	if err != nil {
		return Video{}, err
	}
	return ret, body.Close()
}
