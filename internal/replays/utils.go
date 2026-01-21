package replays

import (
	"errors"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

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
