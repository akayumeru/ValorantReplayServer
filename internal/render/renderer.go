package render

import (
	"bytes"
	"html/template"
	"strconv"

	"github.com/akayumeru/valreplayserver/internal/domain"
)

type Renderer struct {
	playerPicksPage  *template.Template
	prematchPage     *template.Template
	matchResultsPage *template.Template
}

func NewRenderer() (*Renderer, error) {
	pp, err := template.ParseFiles("web/templates/screens/player_picks.html")
	if err != nil {
		return nil, err
	}
	pm, err := template.ParseFiles("web/templates/screens/prematch.html")
	if err != nil {
		return nil, err
	}
	mr, err := template.ParseFiles("web/templates/screens/match_results.html")
	if err != nil {
		return nil, err
	}

	return &Renderer{
		playerPicksPage:  pp,
		prematchPage:     pm,
		matchResultsPage: mr,
	}, nil
}

func (r *Renderer) RenderPlayerPicksPage(st domain.State) ([]byte, error) {
	return execute(r.playerPicksPage, st)
}

func (r *Renderer) RenderPrematchPage(st domain.State) ([]byte, error) {
	return execute(r.prematchPage, st)
}

func (r *Renderer) RenderMatchResultsPage(st domain.State) ([]byte, error) {
	return execute(r.matchResultsPage, st)
}

func (r *Renderer) RenderPlayerPicksFragment(st domain.State) []byte {
	var b bytes.Buffer

	b.WriteString(`<div id="content">`)
	b.WriteString(`<h1>Player picks</h1>`)
	b.WriteString(`<ul>`)
	for _, p := range st.Picks {
		b.WriteString(`<li>`)
		b.WriteString(template.HTMLEscapeString(p.PlayerName))
		b.WriteString(` — `)
		b.WriteString(template.HTMLEscapeString(p.Agent))
		if p.Locked {
			b.WriteString(` (locked)`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
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

func intToString(v int) string {
	return template.HTMLEscapeString(strconv.Itoa(v))
}
