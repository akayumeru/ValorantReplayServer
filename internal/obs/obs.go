package obs

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/valorant"
	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/scenes"
)

type Controller struct {
	StateStore      *store.StateStore
	ReplaySceneName string
	VlcInputName    string
	Obs             *goobs.Client
	BaseURL         *url.URL

	isPlaying     bool
	previousScene string
}

func (c *Controller) StartReplay(replayID uint32) error {
	if c.Obs == nil {
		return errors.New("obs client is nil")
	}

	if c.ReplaySceneName == "" {
		c.ReplaySceneName = "Replay"
	}
	if c.VlcInputName == "" {
		c.VlcInputName = "Replay Source"
	}

	replayURL := c.buildReplayURL(replayID)
	if replayURL == "" {
		return errors.New("replay url is empty")
	}

	playlist := []map[string]any{
		{"value": replayURL},
	}

	_, err := c.Obs.Inputs.SetInputSettings(
		inputs.NewSetInputSettingsParams().
			WithInputName(c.VlcInputName).
			WithOverlay(true).
			WithInputSettings(map[string]any{
				"playlist": playlist,
			}),
	)
	if err != nil {
		return fmt.Errorf("SetInputSettings(%s): %w", c.VlcInputName, err)
	}

	cur, err := c.Obs.Scenes.GetCurrentProgramScene(&scenes.GetCurrentProgramSceneParams{})
	if err != nil {
		return fmt.Errorf("GetCurrentProgramScene: %w", err)
	}

	c.previousScene = cur.SceneName
	c.isPlaying = true

	c.Obs.Outputs.StopReplayBuffer()

	_, err = c.Obs.Scenes.SetCurrentProgramScene(
		scenes.NewSetCurrentProgramSceneParams().WithSceneName(c.ReplaySceneName),
	)
	if err != nil {
		c.isPlaying = false
		return fmt.Errorf("SetCurrentProgramScene(%s): %w", c.ReplaySceneName, err)
	}

	return nil
}

func (c *Controller) StartReplayBuffer() error {
	status, err := c.Obs.Outputs.GetReplayBufferStatus()

	if err == nil && !status.OutputActive {
		_, err = c.Obs.Outputs.StartReplayBuffer()
		return err
	}

	return err
}

func (c *Controller) StopReplay() error {
	if c.Obs == nil {
		return errors.New("obs client is nil")
	}
	if !c.isPlaying {
		return nil
	}
	if c.previousScene == "" {
		c.isPlaying = false
		return nil
	}

	cur, err := c.Obs.Scenes.GetCurrentProgramScene(&scenes.GetCurrentProgramSceneParams{})
	if err != nil {
		return fmt.Errorf("GetCurrentProgramScene: %w", err)
	}
	if cur.SceneName != c.ReplaySceneName {
		c.isPlaying = false
		return nil
	}

	_, err = c.Obs.Scenes.SetCurrentProgramScene(
		scenes.NewSetCurrentProgramSceneParams().WithSceneName(c.previousScene),
	)
	if err != nil {
		return fmt.Errorf("SetCurrentProgramScene(%s): %w", c.previousScene, err)
	}

	c.isPlaying = false
	c.previousScene = ""

	return nil
}

func (c *Controller) buildReplayURL(replayID uint32) string {
	u := *c.BaseURL
	u.Path = "/replay.ts"
	q := u.Query()
	q.Set("replay_id", fmt.Sprintf("%d", replayID))
	q.Set("control_obs", "true")

	replayWindow, _ := ReplayWindow(c.StateStore.Get().MatchInfo)

	if replayWindow-2*time.Second <= 0*time.Second {
		replayWindow = 15 * time.Second
	}

	q.Set("max_duration", fmt.Sprintf("%d", uint32(math.Round(replayWindow.Seconds()))))
	u.RawQuery = q.Encode()

	return u.String()
}

func ReplayWindow(mi domain.MatchInfo) (time.Duration, error) {
	if mi.CurrentRound == nil {
		return 30 * time.Second, nil
	}

	if mi.CurrentRound.LastPhase == "combat" {
		return 0, errors.New("replay is disabled during combat phase")
	}

	dur, ok := valorant.PhaseDuration[mi.CurrentRound.LastPhase]
	if !ok {
		return 30 * time.Second, nil
	}
	if mi.CurrentRound.PhaseStartedAt.IsZero() {
		return 30 * time.Second, nil
	}

	endsAt := mi.CurrentRound.PhaseStartedAt.Add(dur)
	rem := time.Until(endsAt)
	if rem <= 0 {
		return 0, errors.New("phase already ended")
	}
	return rem, nil
}
