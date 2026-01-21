package replays

import (
	"errors"
	"math"
	"sort"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
)

type Clip struct {
	MediaPath string
	StartSec  float64
	DurSec    float64
	SortKeyMs uint64
}

func BuildPlan(window time.Duration, highlights []domain.Highlight, fade time.Duration) ([]Clip, time.Duration, error) {
	if window <= 0 {
		return nil, 0, errors.New("window must be > 0")
	}

	totalEvents := 0
	for _, h := range highlights {
		totalEvents += len(h.EventsTimestamps)
	}
	if totalEvents == 0 {
		return nil, 0, errors.New("no EventsTimestamps")
	}

	windowSec := window.Seconds()
	slotSec := windowSec / float64(totalEvents)

	if fade < 0 {
		fade = 0
	}
	fadeSec := math.Min(fade.Seconds(), slotSec*0.2)

	// transition overlap compensation
	clipSec := math.Max((windowSec+float64(totalEvents-1)*fadeSec)/float64(totalEvents), 7.5)

	var clips []Clip
	for _, h := range highlights {
		highlightDurSec := float64(h.Duration) / 1000.0
		if highlightDurSec <= 0 {
			continue
		}

		for _, evOffset := range h.EventsTimestamps {
			// offset of highlight
			evOffsetSec := float64(evOffset) / 1000.0

			// duration per highlight
			dur := clipSec
			if dur > highlightDurSec {
				dur = highlightDurSec
			}

			start := evOffsetSec - dur/2.0
			if start < 0 {
				start = 0
			}

			maxStart := highlightDurSec - dur
			if maxStart < 0 {
				maxStart = 0
			}
			if start > maxStart {
				start = maxStart
			}

			clips = append(clips, Clip{
				MediaPath: h.MediaPath,
				StartSec:  start,
				DurSec:    dur,
				SortKeyMs: h.StartTime + evOffset,
			})
		}
	}

	sort.Slice(clips, func(i, j int) bool { return clips[i].SortKeyMs < clips[j].SortKeyMs })

	if len(clips) == 0 {
		return nil, 0, errors.New("no clips built")
	}

	// total duration after xfade overlap
	total := 0.0
	for _, c := range clips {
		total += c.DurSec
	}
	if len(clips) > 1 {
		total -= float64(len(clips)-1) * fadeSec
	}
	if total < 0 {
		total = 0
	}

	return clips, time.Duration(total * float64(time.Second)), nil
}
