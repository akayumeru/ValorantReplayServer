package valorant

import (
	"strings"
	"time"
)

func NormalizeName(s string) string {
	i := strings.IndexByte(s, '#')
	if i < 0 {
		return s
	}
	if i > 0 && s[i-1] == ' ' {
		return s[:i-1] + s[i+1:]
	}
	return s[:i] + s[i+1:]
}

var AgentByInternal = map[string]string{
	"Clay":         "Raze",
	"Pandemic":     "Viper",
	"Wraith":       "Omen",
	"Hunter":       "Sova",
	"Thorne":       "Sage",
	"Phoenix":      "Phoenix",
	"Wushu":        "Jett",
	"Gumshoe":      "Cypher",
	"Sarge":        "Brimstone",
	"Breach":       "Breach",
	"Vampire":      "Reyna",
	"Killjoy":      "Killjoy",
	"Guide":        "Skye",
	"Stealth":      "Yoru",
	"Rift":         "Astra",
	"Grenadier":    "KAY/O",
	"Deadeye":      "Chamber",
	"Sprinter":     "Neon",
	"BountyHunter": "Fade",
	"Mage":         "Harbor",
	"AggroBot":     "Gekko",
	"Cable":        "Deadlock",
	"Sequoia":      "Iso",
	"Smonk":        "Clove",
	"Nox":          "Vyse",
	"Cashew":       "Tejo",
	"Terra":        "Waylay",
}

var PhaseDuration = map[string]time.Duration{
	"shopping": 30 * time.Second,
	"combat":   100 * time.Second,
	"end":      7 * time.Second,
	"game_end": 7 * time.Second,
}

func NormalizeAgent(internal string) string {
	if v, ok := AgentByInternal[internal]; ok {
		return v
	}
	return internal
}
