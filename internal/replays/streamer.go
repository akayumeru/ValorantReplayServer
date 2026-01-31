package replays

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

type Streamer struct {
	Store     *store.StateStore
	FFmpegBin string
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

	window := valorant.PhaseDuration["shopping"]

	const fade = 350 * time.Millisecond

	clips, totalDur, err := BuildPlan(window, highlights, fade)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	args := buildFFmpegArgsNVENC(clips, fade)

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
		for sc.Scan() {
			_ = sc.Text()
		}
	}()

	var hookTimer *time.Timer
	if totalDur > 2*time.Second {
		hookTimer = time.AfterFunc(totalDur-2*time.Second, func() {
			// TODO: OBS WebSocket signal here
		})
	}

	if err := cmd.Start(); err != nil {
		if hookTimer != nil {
			hookTimer.Stop()
		}
		http.Error(w, "ffmpeg start error", http.StatusInternalServerError)
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

func buildFFmpegArgsNVENC(clips []Clip, fade time.Duration) []string {
	args := make([]string, 0, 64)

	for _, c := range clips {
		args = append(args,
			"-re",
			"-ss", fmt.Sprintf("%.3f", c.StartSec),
			"-t", fmt.Sprintf("%.3f", c.DurSec),
			"-i", c.MediaPath,
		)
	}

	// filter_complex: xfade + acrossfade
	fadeSec := fade.Seconds()

	fc := ""
	for i := range clips {
		fc += fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS[v%d];", i, i)
		fc += fmt.Sprintf("[%d:a]asetpts=PTS-STARTPTS[a%d];", i, i)
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

		fc += fmt.Sprintf("[%s][v%d]xfade=transition=fade:duration=%.3f:offset=%.3f[%s];",
			outV, i, fadeSec, offset, nextV,
		)
		fc += fmt.Sprintf("[%s][a%d]acrossfade=d=%.3f[%s];",
			outA, i, fadeSec, nextA,
		)

		outLen = outLen + clips[i].DurSec - fadeSec
		outV = nextV
		outA = nextA
	}

	args = append(args,
		"-filter_complex", fc,
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
		"-c:a", "aac",
		"-b:a", "192k",

		// MPEG-TS into stdout (pipe:1)
		"-f", "mpegts",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"pipe:1",
	)

	return args
}
