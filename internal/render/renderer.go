package render

import (
	"bytes"
	"html/template"
	"strconv"

	"github.com/akayumeru/valreplayserver/internal/domain"
)

type Renderer struct {
	playerPicksPage  *template.Template
	matchInfoPage    *template.Template
	matchResultsPage *template.Template
}

func NewRenderer() (*Renderer, error) {
	pp, err := template.ParseFiles("web/templates/screens/player_picks.html")
	if err != nil {
		return nil, err
	}

	mi, err := template.ParseFiles("web/templates/screens/match_info.html")
	if err != nil {
		return nil, err
	}

	return &Renderer{
		playerPicksPage: pp,
		matchInfoPage:   mi,
	}, nil
}

func (r *Renderer) RenderPlayerPicksPage(st domain.State) ([]byte, error) {
	return execute(r.playerPicksPage, st)
}

func (r *Renderer) RenderMatchInfoPage(st domain.State) ([]byte, error) {
	return execute(r.matchInfoPage, st)
}

func (r *Renderer) RenderPlayerPicksFragment(st domain.State) []byte {
	var b bytes.Buffer

	b.WriteString(`<div id="content">`)
	b.WriteString(`<h1>Player picks</h1>`)
	b.WriteString(`</div>`)

	return b.Bytes()
}

func (r *Renderer) RenderMatchInfoFragment(st domain.State) []byte {
	var b bytes.Buffer

	b.WriteString(`<div id="content">`)
	b.WriteString(`<h1>Match Info</h1>`)
	b.WriteString(`<p>Map: `)
	b.WriteString(template.HTMLEscapeString(st.MatchInfo.Map))
	b.WriteString(`</p>`)
	if st.MatchInfo.CurrentRound != nil {
		b.WriteString(`<p>Round: `)
		b.WriteString(template.HTMLEscapeString(strconv.Itoa(int(st.MatchInfo.CurrentRound.Number))))
		b.WriteString(` (`)
		b.WriteString(template.HTMLEscapeString(string(st.MatchInfo.CurrentRound.LastPhase)))
		b.WriteString(`)`)
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)

	return b.Bytes()
}

func execute(t *template.Template, st domain.State) ([]byte, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, st); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
