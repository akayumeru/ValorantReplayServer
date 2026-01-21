package valorant

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/utils"
)

type Topics struct {
	PlayerPicks bool
	MatchInfo   bool
	Replays     bool
}

func (t Topics) List() []string {
	out := make([]string, 0, 3)
	if t.PlayerPicks {
		out = append(out, "player_picks")
	}
	if t.MatchInfo {
		out = append(out, "match_info")
	}
	if t.Replays {
		out = append(out, "replays")
	}
	return out
}

func ApplyPayload(cur domain.State, payload []byte) (domain.State, Topics, error) {
	env, root, err := ParseEnvelope(payload)
	if err != nil {
		return cur, Topics{}, err
	}

	if cur.MatchInfo.Roster == nil {
		cur.MatchInfo.Roster = make(map[string]domain.RosterPlayer)
	}

	var touched Topics

	// events
	if len(env.Events) > 0 || root["events"] != nil {
		if len(env.Events) == 0 && root["events"] != nil {
			_ = json.Unmarshal(root["events"], &env.Events)
		}
		for _, e := range env.Events {
			cur, touched = applyEvent(cur, e, touched)
		}
		cur.UpdatedAt = time.Now().UTC()

		utils.DebugLog("Got events", env.Events)

		return cur, touched, nil
	}

	// match_info / game_info
	if len(env.MatchInfo) > 0 || root["match_info"] != nil {
		if len(env.MatchInfo) == 0 && root["match_info"] != nil {
			_ = json.Unmarshal(root["match_info"], &env.MatchInfo)
		}
		cur, touched = applyMatchInfo(cur, env.MatchInfo, touched)

		utils.DebugLog("Got match info update", env.MatchInfo)
	}

	if len(env.GameInfo) > 0 || root["game_info"] != nil {
		if len(env.GameInfo) == 0 && root["game_info"] != nil {
			_ = json.Unmarshal(root["game_info"], &env.GameInfo)
		}
		cur, touched = applyGameInfo(cur, env.GameInfo, touched)

		utils.DebugLog("Got game info update", env.GameInfo)
	}

	if len(env.PlayerInfo) > 0 || root["me"] != nil {
		if len(env.PlayerInfo) == 0 && root["me"] != nil {
			_ = json.Unmarshal(root["me"], &env.PlayerInfo)
		}
		cur = applyPlayerInfo(cur, env.PlayerInfo)

		utils.DebugLog("Got player info update", env.PlayerInfo)
	}

	cur.UpdatedAt = time.Now().UTC()
	return cur, touched, nil
}

func applyGameInfo(cur domain.State, gi map[string]json.RawMessage, touched Topics) (domain.State, Topics) {
	if v, ok := gi["scene"]; ok {
		var scene string
		if json.Unmarshal(v, &scene) == nil && scene != "" {
			cur.GameInfo.Scene = scene
			touched.PlayerPicks = true
			touched.MatchInfo = true
		}
	}

	if v, ok := gi["state"]; ok {
		var st string
		if json.Unmarshal(v, &st) == nil && st != "" {
			cur.GameInfo.State = st
		}
	}

	return cur, touched
}

func applyPlayerInfo(cur domain.State, pi map[string]json.RawMessage) domain.State {
	if v, ok := pi["player_name"]; ok {
		var pn string
		if json.Unmarshal(v, &pn) == nil && pn != "" {
			cur.PlayerInfo.Name = pn
		}
	}

	if v, ok := pi["player_id"]; ok {
		var pid string
		if json.Unmarshal(v, &pid) == nil && pid != "" {
			cur.PlayerInfo.ID = pid
		}
	}

	return cur
}

func applyMatchInfo(cur domain.State, mi map[string]json.RawMessage, touched Topics) (domain.State, Topics) {
	for k, v := range mi {
		switch {
		case k == "pseudo_match_id":
			var s string
			if json.Unmarshal(v, &s) == nil {
				cur.MatchInfo.PseudoMatchID = s
				touched.MatchInfo = true
			}

		case k == "match_id":
			var s string
			if json.Unmarshal(v, &s) == nil {
				cur.MatchInfo.MatchID = s
				touched.MatchInfo = true
			}

		case k == "map":
			var s string
			if json.Unmarshal(v, &s) == nil {
				cur.MatchInfo.Map = s
				touched.MatchInfo = true
			}

		case k == "round_number":
			var s string
			if json.Unmarshal(v, &s) == nil {
				if n, err := strconv.Atoi(s); err == nil {
					if cur.MatchInfo.CurrentRound == nil || cur.MatchInfo.CurrentRound.Number != n {
						newRound := &domain.Round{
							Number:          n,
							StartedAt:       time.Now().UTC(),
							EndedAt:         time.Now().UTC().Add(PhaseDuration["shopping"] + PhaseDuration["combat"] + PhaseDuration["end"] + 1*time.Second),
							LastPhase:       "shopping",
							PhaseStartedAt:  time.Now().UTC(),
							HighlightsCount: 0,
						}
						if cur.MatchInfo.Rounds == nil {
							cur.MatchInfo.Rounds = make(map[int]*domain.Round)
						}
						cur.MatchInfo.Rounds[newRound.Number] = newRound
						if cur.MatchInfo.CurrentRound != nil {
							cur.MatchInfo.CurrentRound.EndedAt = newRound.StartedAt.Add(-1 * time.Millisecond)
						}
						cur.MatchInfo.CurrentRound = newRound

						touched.Replays = true
					}
					touched.MatchInfo = true
				}
			}

		case k == "round_phase":
			var s string
			if json.Unmarshal(v, &s) == nil {
				if cur.MatchInfo.CurrentRound != nil {
					cur.MatchInfo.CurrentRound.LastPhase = s
					cur.MatchInfo.CurrentRound.PhaseStartedAt = time.Now().UTC()
					touched.MatchInfo = true
				}
			}

		default:
			if strings.HasPrefix(k, "roster_") {
				var s string
				if json.Unmarshal(v, &s) != nil || s == "" {
					continue
				}
				var p domain.RosterPlayer
				if json.Unmarshal([]byte(s), &p) != nil {
					continue
				}

				p.Name = NormalizeName(p.Name)
				p.Character = NormalizeAgent(p.Character)

				if p.PlayerID != "" {
					cur.MatchInfo.Roster[p.PlayerID] = p
					touched.PlayerPicks = true
					touched.MatchInfo = true
				}
			}
		}
	}
	return cur, touched
}

func applyEvent(cur domain.State, e RawEvent, touched Topics) (domain.State, Topics) {
	switch e.Name {
	case "match_start":
		touched.MatchInfo = true

	case "match_end":
		cur.MatchInfo.Rounds = nil
		cur.MatchInfo.KillFeed = nil
		cur.MatchInfo.CurrentRound = nil
		touched.MatchInfo = true

	case "kill_feed":
		var s string
		if json.Unmarshal(e.Data, &s) == nil && s != "" {
			var k domain.KillFeedEntry
			if json.Unmarshal([]byte(s), &k) == nil {
				k.Attacker = NormalizeName(k.Attacker)
				k.Victim = NormalizeName(k.Victim)

				if k.Attacker == cur.PlayerInfo.Name {
					cur.MatchInfo.CurrentRound.HighlightsCount++
				}

				cur.MatchInfo.KillFeed = append(cur.MatchInfo.KillFeed, k)
				if len(cur.MatchInfo.KillFeed) > 20 {
					cur.MatchInfo.KillFeed = cur.MatchInfo.KillFeed[len(cur.MatchInfo.KillFeed)-20:]
				}

				touched.MatchInfo = true
			}
		}
	}
	return cur, touched
}
