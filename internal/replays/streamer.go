package replays

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/akayumeru/valreplayserver/internal/obs"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

type Streamer struct {
	Store                *store.StateStore
	ObsController        *obs.Controller
	GameAudioStreamTitle string
	GameAudioStreamIndex int
	FFmpegBin            string
	FFprobeBin           string

	audioMu    sync.Mutex
	audioCache map[string]int // MediaPath -> audioIdx (a:<idx>)
}

func (s *Streamer) HandleStream(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("replay_id")
	if idStr == "" {
		http.Error(w, "missing replay_id", http.StatusBadRequest)
		return
	}
	id64, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid replay_id", http.StatusBadRequest)
		return
	}
	replayID := uint32(id64)

	st := s.Store.Get()
	replay, ok := st.ReplayState.Replays[replayID]
	if !ok {
		http.Error(w, "replay not found", http.StatusNotFound)
		return
	}

	highlights := replay.Highlights

	var streamDuration time.Duration
	maxDurStr := r.URL.Query().Get("max_duration")
	if maxDurStr != "" {
		maxDur, err := strconv.ParseUint(maxDurStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid max_duration", http.StatusBadRequest)
		}

		streamDuration = time.Duration(maxDur) * time.Second
	} else {
		streamDuration = valorant.PhaseDuration["shopping"]
	}

	var controlObs bool
	controlObsStr := r.URL.Query().Get("control_obs")
	if controlObsStr != "" {
		controlObs, err = strconv.ParseBool(controlObsStr)
		if err != nil {
			http.Error(w, "invalid control_obs", http.StatusBadRequest)
		}
	} else {
		controlObs = false
	}

	const fade = 350 * time.Millisecond

	clips, totalDur, err := BuildPlan(streamDuration, highlights, fade)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	audioIdx := s.resolveAudioIndices(clips)
	args := buildFFmpegArgsNVENC(clips, audioIdx, fade)

	w.Header().Set("Content-Type", "video/MP2T")
	w.Header().Set("Cache-Control", "no-store")

	flusher, _ := w.(http.Flusher)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, s.FFmpegBin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "stdout pipe error", http.StatusInternalServerError)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		http.Error(w, "stderr pipe error", http.StatusInternalServerError)
		return
	}

	go func() {
		sc := bufio.NewScanner(stderr)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 2*1024*1024)

		for sc.Scan() {
			log.Printf("[ffmpeg] %s", sc.Text())
		}
		if err := sc.Err(); err != nil {
			log.Printf("[ffmpeg] stderr scan error: %v", err)
		}
	}()

	var hookTimer *time.Timer
	if controlObs && totalDur > 1*time.Second {
		hookTimer = time.AfterFunc(totalDur-1*time.Second, func() {
			s.ObsController.StopReplay()
		})
	}

	if err := cmd.Start(); err != nil {
		if hookTimer != nil {
			hookTimer.Stop()
		}
		http.Error(w, "ffmpeg start error", http.StatusInternalServerError)
		log.Printf("ffmpeg start error: %v", err)
		return
	}

	defer func() {
		if hookTimer != nil {
			hookTimer.Stop()
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				cancel()
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func buildFFmpegArgsNVENC(clips []Clip, audioIdx []int, fade time.Duration) []string {
	args := make([]string, 0, 128)

	// Inputs
	for _, c := range clips {
		args = append(args,
			// Helps avoid blocking / stalls in complex graphs
			"-thread_queue_size", "1024",

			// Real-time pacing (important if you use wall-clock timer in Go)
			"-re",

			// Fast seek to approximate start
			"-ss", fmt.Sprintf("%.3f", c.StartSec),

			"-i", c.MediaPath,
		)
	}

	fadeSec := fade.Seconds()

	var b strings.Builder

	for i, c := range clips {
		// Video
		// Trim exact duration; setpts; fifo (buffering)
		fmt.Fprintf(&b, "[%d:v]trim=duration=%.3f,setpts=PTS-STARTPTS[v%d];", i, c.DurSec, i)

		ai := 0
		if i < len(audioIdx) && audioIdx[i] >= 0 {
			ai = audioIdx[i]
		}

		// Audio
		// atrim exact duration; asetpts; format normalize; async resample; afifo
		fmt.Fprintf(&b,
			"[%d:a:%d]atrim=duration=%.3f,asetpts=PTS-STARTPTS,"+
				"aformat=sample_rates=48000:channel_layouts=stereo,"+
				"aresample=async=1000:first_pts=0[a%d];",
			i, ai, c.DurSec, i,
		)
	}

	outV := "v0"
	outA := "a0"
	outLen := clips[0].DurSec

	for i := 1; i < len(clips); i++ {
		offset := outLen - fadeSec
		if offset < 0 {
			offset = 0
		}

		nextV := fmt.Sprintf("vxf%d", i)
		nextA := fmt.Sprintf("axf%d", i)

		fmt.Fprintf(&b,
			"[%s][v%d]xfade=transition=fade:duration=%.3f:offset=%.3f[%s];",
			outV, i, fadeSec, offset, nextV,
		)
		fmt.Fprintf(&b,
			"[%s][a%d]acrossfade=d=%.3f:c1=tri:c2=tri[%s];",
			outA, i, fadeSec, nextA,
		)

		outLen = outLen + clips[i].DurSec - fadeSec
		outV = nextV
		outA = nextA
	}

	args = append(args,
		"-hide_banner",
		"-loglevel", "warning",

		"-filter_complex", b.String(),
		"-map", fmt.Sprintf("[%s]", outV),
		"-map", fmt.Sprintf("[%s]", outA),

		// Encode with GPU (NVENC)
		"-c:v", "h264_nvenc",
		"-preset", "p2",
		"-tune", "ll",
		"-rc", "cbr",
		"-b:v", "25M",
		"-maxrate", "25M",
		"-bufsize", "4M",
		"-g", "120",

		// Audio AAC 128k
		"-c:a", "aac",
		"-b:a", "128k",

		// Output MPEG-TS to stdout (pipe:1)
		"-f", "mpegts",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"pipe:1",
	)

	return args
}

func (s *Streamer) resolveAudioIndices(clips []Clip) []int {
	out := make([]int, len(clips))

	title := s.GameAudioStreamTitle
	if title == "" {
		title = "Game only"
	}

	for i, c := range clips {
		if idx, ok := s.getAudioIdxCached(c.MediaPath); ok {
			out[i] = idx
			continue
		}

		idx, streamsCount, err := s.FindAudioStreamIndexByTitle(
			context.Background(),
			c.MediaPath,
			title,
		)

		if err != nil {
			if streamsCount >= s.GameAudioStreamIndex+1 {
				idx = s.GameAudioStreamIndex
			} else {
				idx = 0
			}
		}

		s.setAudioIdxCached(c.MediaPath, idx)
		out[i] = idx
	}

	return out
}

func (s *Streamer) getAudioIdxCached(mediaPath string) (int, bool) {
	s.audioMu.Lock()
	defer s.audioMu.Unlock()

	if s.audioCache == nil {
		s.audioCache = make(map[string]int)
	}
	v, ok := s.audioCache[mediaPath]
	return v, ok
}

func (s *Streamer) setAudioIdxCached(mediaPath string, idx int) {
	s.audioMu.Lock()
	defer s.audioMu.Unlock()

	if s.audioCache == nil {
		s.audioCache = make(map[string]int)
	}
	s.audioCache[mediaPath] = idx
}

type ffprobeStreamsResponse struct {
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	Index int               `json:"index"`
	Tags  map[string]string `json:"tags"`
}

func (streamer *Streamer) FindAudioStreamIndexByTitle(
	ctx context.Context,
	filePath string,
	wantTitle string,
) (int, int, error) {
	if strings.TrimSpace(filePath) == "" {
		return 0, 0, errors.New("filePath is empty")
	}
	if strings.TrimSpace(wantTitle) == "" {
		return 0, 0, errors.New("wantTitle is empty")
	}

	cmd := exec.CommandContext(
		ctx,
		streamer.FFprobeBin,
		"-v", "error",
		"-select_streams", "a",
		"-show_streams",
		"-print_format", "json",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var resp ffprobeStreamsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, 0, fmt.Errorf("ffprobe json parse failed: %w", err)
	}

	if len(resp.Streams) == 0 {
		return 0, 0, errors.New("no audio streams found")
	}

	normalize := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}
	want := normalize(wantTitle)

	for audioIdx, s := range resp.Streams {
		title := ""
		if s.Tags != nil {
			title = s.Tags["title"]
		}
		if normalize(title) == want {
			return audioIdx, len(resp.Streams), nil
		}
	}

	return 0, len(resp.Streams), fmt.Errorf("audio stream with title %q not found; fallback to a:0", wantTitle)
}
