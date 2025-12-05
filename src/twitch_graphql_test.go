package src

import (
	"testing"
)

//run: go test -v

const DEV_TWITCH = true

func TestAdd(t *testing.T) {
	if DEV_TWITCH {
		result := Scrape_vods("limealicious")
		if result.Err != nil {
			t.Logf("ERROR: %s", result.Err)
		}
		t.Logf("%+v", result.Val)
	}
}


