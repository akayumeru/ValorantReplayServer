package valorant

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
)

type Topics struct {
	PlayerPicks bool
	MatchInfo   bool
}

func (t Topics) List() []string {
	out := make([]string, 0, 3)
	if t.PlayerPicks {
		out = append(out, "player_picks")
	}
	if t.MatchInfo {
		out = append(out, "match_info")
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

		log.Printf("Got events: %#v\n", env.Events)

		return cur, touched, nil
	}

	// match_info / game_info
	if len(env.MatchInfo) > 0 || root["match_info"] != nil {
		if len(env.MatchInfo) == 0 && root["match_info"] != nil {
			_ = json.Unmarshal(root["match_info"], &env.MatchInfo)
		}
		cur, touched = applyMatchInfo(cur, env.MatchInfo, touched)

		log.Printf("Got match info update: %#v\n", env.MatchInfo)
	}

	if len(env.GameInfo) > 0 || root["game_info"] != nil {
		if len(env.GameInfo) == 0 && root["game_info"] != nil {
			_ = json.Unmarshal(root["game_info"], &env.GameInfo)
		}
		cur, touched = applyGameInfo(cur, env.GameInfo, touched)

		log.Printf("Got game info update: %#v\n", env.GameInfo)
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
					cur.MatchInfo.RoundNumber = n
					touched.MatchInfo = true
				}
			}

		case k == "round_phase":
			var s string
			if json.Unmarshal(v, &s) == nil {
				cur.MatchInfo.RoundPhase = s
				touched.MatchInfo = true
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
		touched.MatchInfo = true

	case "kill_feed":
		var s string
		if json.Unmarshal(e.Data, &s) == nil && s != "" {
			var k domain.KillFeedEntry
			if json.Unmarshal([]byte(s), &k) == nil {
				k.Attacker = NormalizeName(k.Attacker)
				k.Victim = NormalizeName(k.Victim)

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
