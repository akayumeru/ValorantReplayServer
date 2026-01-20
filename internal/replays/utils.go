package replays

import (
	"errors"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

func ReplayWindow(mi domain.MatchInfo) (time.Duration, error) {
	if mi.RoundPhase == "combat" {
		return 0, errors.New("replay is disabled during combat phase")
	}

	dur, ok := valorant.PhaseDuration[mi.RoundPhase]
	if !ok {
		return 1 * time.Minute, nil
	}
	if mi.RoundPhaseStartedAt.IsZero() {
		return 1 * time.Minute, nil
	}

	endsAt := mi.RoundPhaseStartedAt.Add(dur)
	rem := time.Until(endsAt)
	if rem <= 0 {
		return 0, errors.New("phase already ended")
	}
	return rem, nil
}
