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

type interval struct {
	mediaPath string
	startSec  float64
	endSec    float64
	sortKeyMs uint64
}

func BuildPlan(window time.Duration, highlights []*domain.Highlight, fade time.Duration) ([]Clip, time.Duration, error) {
	if window <= 0 {
		return nil, 0, errors.New("window must be > 0")
	}

	totalEvents := 0
	for _, h := range highlights {
		if h != nil {
			totalEvents += len(h.EventsTimestamps)
		}
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

	raw := make([]interval, 0, totalEvents)

	for _, h := range highlights {
		if h == nil {
			continue
		}

		highlightDurSec := float64(h.Duration) / 1000.0
		if highlightDurSec <= 0 {
			continue
		}

		for _, evOffset := range h.EventsTimestamps {
			evOffsetSec := float64(evOffset) / 1000.0

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

			end := start + dur
			if end > highlightDurSec {
				end = highlightDurSec
				start = math.Max(0, end-dur)
			}

			raw = append(raw, interval{
				mediaPath: h.MediaPath,
				startSec:  start,
				endSec:    end,
				sortKeyMs: h.StartTime + evOffset,
			})
		}
	}

	if len(raw) == 0 {
		return nil, 0, errors.New("no clips built")
	}

	byPath := make(map[string][]interval, len(highlights))
	for _, it := range raw {
		byPath[it.mediaPath] = append(byPath[it.mediaPath], it)
	}

	mergeGapSec := math.Max(fadeSec*1.75, 0.25)

	merged := make([]interval, 0, len(raw))
	for path, items := range byPath {
		sort.Slice(items, func(i, j int) bool {
			if items[i].startSec == items[j].startSec {
				return items[i].sortKeyMs < items[j].sortKeyMs
			}
			return items[i].startSec < items[j].startSec
		})

		cur := items[0]
		cur.mediaPath = path

		for i := 1; i < len(items); i++ {
			nxt := items[i]

			// when intersecting or so close to intersect
			if nxt.startSec <= cur.endSec+mergeGapSec {
				if nxt.endSec > cur.endSec {
					cur.endSec = nxt.endSec
				}
				if nxt.sortKeyMs < cur.sortKeyMs {
					cur.sortKeyMs = nxt.sortKeyMs
				}
				continue
			}

			merged = append(merged, cur)
			cur = nxt
			cur.mediaPath = path
		}

		merged = append(merged, cur)
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].sortKeyMs < merged[j].sortKeyMs })

	clips := make([]Clip, 0, len(merged))
	for _, it := range merged {
		d := it.endSec - it.startSec
		if d <= 0.05 {
			continue
		}
		clips = append(clips, Clip{
			MediaPath: it.mediaPath,
			StartSec:  it.startSec,
			DurSec:    d,
			SortKeyMs: it.sortKeyMs,
		})
	}

	if len(clips) == 0 {
		return nil, 0, errors.New("no clips after merge")
	}

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
