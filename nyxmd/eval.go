package nyxmd

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Options règle l'évaluation d'une requête compilée.
type Options struct {
	// FoldAccents ignore les accents des lettres latines (archive = archivé), dans la
	// requête comme dans les documents. Désactivé par défaut (règle NyxBoolean).
	FoldAccents bool `json:"fold_accents"`
}

// Span est une plage [Start, End) en caractères Unicode (runes) dans Document.Text.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Query est une requête compilée une fois, évaluable sur de nombreux documents.
// Une Query est immuable : elle peut être partagée entre goroutines.
type Query struct {
	root *Node
	opts Options
}

// Compile accepte une requête vide ou blanche, qui correspond à tous les documents.
func Compile(query string) (*Query, error) { return CompileWith(query, Options{}) }

// CompileWith compile la requête avec des options.
func CompileWith(query string, opts Options) (*Query, error) {
	if strings.TrimSpace(query) == "" {
		return &Query{opts: opts}, nil
	}
	n, err := Parse(query)
	if err != nil {
		return nil, err
	}
	if opts.FoldAccents {
		compile(n, opts) // Parse a compilé sans options
	}
	return &Query{root: n, opts: opts}, nil
}

// Root renvoie l'AST (nil pour une requête vide).
func (q *Query) Root() *Node { return q.root }

// Options renvoie les options de compilation.
func (q *Query) Options() Options { return q.opts }

// Match évalue la requête ; aucune donnée externe n'est lue ni modifiée.
func (q *Query) Match(d Document) bool {
	if q == nil {
		return false
	}
	if q.root == nil {
		return true
	}
	return q.root.Match(d)
}

// Find évalue la requête et renvoie, si elle correspond, les plages de texte trouvées
// par les termes, expressions et NEAR non niés (triées, sans chevauchement).
// Les métadonnées (#tag, @entité, note:, couleur:) ne produisent pas de plage.
func (q *Query) Find(d Document) (bool, []Span) {
	if !q.Match(d) || q.root == nil {
		return q != nil && q.root == nil, nil
	}
	ws := tokenize(d.Text, q.opts.FoldAccents)
	var spans []Span
	var walk func(n *Node, negated bool)
	walk = func(n *Node, negated bool) {
		if n == nil {
			return
		}
		switch n.Op {
		case "NOT":
			walk(n.Left, !negated)
		case "term", "phrase", "NEAR":
			if !negated {
				for _, s := range n.spans(ws) {
					spans = append(spans, Span{ws[s.start].start, ws[s.end].end})
				}
			}
		case "AND", "OR":
			walk(n.Left, negated)
			walk(n.Right, negated)
		}
	}
	walk(q.root, false)
	return true, mergeSpans(spans)
}

// Terms liste les termes et expressions de texte non niés, pour l'affichage.
func (q *Query) Terms() []string {
	var out []string
	var walk func(n *Node, negated bool)
	walk = func(n *Node, negated bool) {
		if n == nil {
			return
		}
		switch n.Op {
		case "NOT":
			walk(n.Left, !negated)
		case "term", "phrase":
			if !negated {
				out = append(out, n.Value)
			}
		default:
			walk(n.Left, negated)
			walk(n.Right, negated)
		}
	}
	if q != nil {
		walk(q.root, false)
	}
	return out
}

func mergeSpans(in []Span) []Span {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Start < in[j].Start })
	out := []Span{in[0]}
	for _, s := range in[1:] {
		last := &out[len(out)-1]
		if s.Start <= last.End {
			if s.End > last.End {
				last.End = s.End
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// compile précalcule les motifs normalisés (une seule fois par requête).
func compile(n *Node, opts Options) {
	if n == nil {
		return
	}
	n.fold = opts.FoldAccents
	switch n.Op {
	case "term", "tag", "entity", "color":
		n.pattern = []rune(normalize(n.Value, opts.FoldAccents))
		n.wildcard = strings.ContainsAny(n.Value, "*?")
	case "note":
		// Un mot (jokers permis) ou une expression entre guillemets.
		n.pattern = []rune(normalize(n.Value, opts.FoldAccents))
		n.wildcard = strings.ContainsAny(n.Value, "*?")
		n.phrase = nil
		if ws := tokenize(n.Value, opts.FoldAccents); len(ws) > 1 {
			for _, w := range ws {
				n.phrase = append(n.phrase, w.text)
			}
		}
	case "phrase":
		n.phrase = nil
		for _, w := range tokenize(n.Value, opts.FoldAccents) {
			n.phrase = append(n.phrase, w.text)
		}
	}
	compile(n.Left, opts)
	compile(n.Right, opts)
}

// Match évalue l'AST. Le texte est découpé en mots une seule fois par appel.
func (n *Node) Match(d Document) bool {
	var ws []word
	split := false
	var eval func(n *Node) bool
	eval = func(n *Node) bool {
		if n == nil {
			return false
		}
		switch n.Op {
		case "AND":
			return eval(n.Left) && eval(n.Right)
		case "OR":
			return eval(n.Left) || eval(n.Right)
		case "NOT":
			return !eval(n.Left)
		case "note":
			for _, t := range d.Notes {
				if n.matchNote(tokenize(t, n.fold)) {
					return true
				}
			}
			return false
		case "tag", "entity", "color":
			values := d.Tags
			switch n.Op {
			case "entity":
				values = d.Entities
			case "color":
				values = d.Colors
			}
			for _, v := range values {
				if n.matchValue(normalize(v, n.fold)) {
					return true
				}
			}
			return false
		default:
			if !split {
				ws, split = tokenize(d.Text, n.fold), true
			}
			return n.found(ws)
		}
	}
	return eval(n)
}

// word est un mot normalisé et sa position (runes) dans le texte d'origine.
type word struct {
	text       string
	start, end int
}

type span struct{ start, end int } // indices de mots

func isWordRune(r rune) bool {
	if r < 0x80 { // chemin rapide ASCII
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'
	}
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

// tokenize découpe en mots (lettres Unicode, chiffres, _) en gardant les positions (runes).
// Un seul tampon par texte : les mots sont des sous-chaînes, sans allocation par mot.
func tokenize(s string, fold bool) []word {
	buf := make([]byte, 0, len(s))
	type cut struct{ b0, b1, start, end int }
	cuts := make([]cut, 0, len(s)/6+1)
	start, b0, pos := -1, 0, 0
	for _, r := range s {
		if isWordRune(r) {
			if start < 0 {
				start, b0 = pos, len(buf)
			}
			lr := unicode.ToLower(r)
			if fold && lr >= 0x80 {
				if f, ok := accentFold[lr]; ok {
					buf = append(buf, f...)
					pos++
					continue
				}
			}
			buf = utf8.AppendRune(buf, lr)
		} else if start >= 0 {
			cuts = append(cuts, cut{b0, len(buf), start, pos})
			start = -1
		}
		pos++
	}
	if start >= 0 {
		cuts = append(cuts, cut{b0, len(buf), start, pos})
	}
	all := string(buf)
	out := make([]word, len(cuts))
	for i, c := range cuts {
		out[i] = word{all[c.b0:c.b1], c.start, c.end}
	}
	return out
}

func normalize(s string, fold bool) string {
	s = strings.ToLower(s)
	if fold {
		s = foldAccents(s)
	}
	return s
}

// matchValue compare un motif (avec ou sans jokers) à une valeur déjà normalisée.
func (n *Node) matchValue(v string) bool {
	if n.pattern == nil {
		compile(n, Options{}) // AST construit à la main, sans Parse
	}
	if !n.wildcard {
		return string(n.pattern) == v
	}
	return globMatch(n.pattern, []rune(v)) // jokers seulement : conversion acceptable
}

// globMatch : * = zéro ou plusieurs caractères, ? = un caractère. Linéaire, sans regexp.
func globMatch(p, s []rune) bool {
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(s) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == s[si]):
			pi++
			si++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// matchNote : mot ou expression dans le texte d'une note.
func (n *Node) matchNote(ws []word) bool {
	if n.pattern == nil {
		compile(n, Options{})
	}
	if len(n.phrase) > 1 {
		return (&Node{Op: "phrase", phrase: n.phrase}).found(ws)
	}
	for _, w := range ws {
		if n.matchValue(w.text) {
			return true
		}
	}
	return false
}

// found : comme spans, mais s'arrête au premier résultat (termes et expressions).
func (n *Node) found(ws []word) bool {
	switch n.Op {
	case "term":
		for _, w := range ws {
			if n.matchValue(w.text) {
				return true
			}
		}
		return false
	case "phrase":
		if n.phrase == nil {
			compile(n, Options{})
		}
		ps := n.phrase
		if len(ps) == 0 {
			return false
		}
	next:
		for i := 0; i+len(ps) <= len(ws); i++ {
			for j, p := range ps {
				if ws[i+j].text != p {
					continue next
				}
			}
			return true
		}
		return false
	default:
		return len(n.spans(ws)) > 0
	}
}

func (n *Node) spans(ws []word) []span {
	out := []span{}
	switch n.Op {
	case "NEAR":
		seen := map[span]bool{}
		for _, a := range n.Left.spans(ws) {
			for _, b := range n.Right.spans(ws) {
				if a.start <= b.end && b.start <= a.end {
					continue
				}
				gap := b.start - a.end - 1
				if a.start > b.end {
					gap = a.start - b.end - 1
				}
				if gap <= n.Distance {
					s := span{min(a.start, b.start), max(a.end, b.end)}
					if !seen[s] {
						out = append(out, s)
						seen[s] = true
					}
				}
			}
		}
	case "phrase":
		if n.phrase == nil {
			compile(n, Options{}) // AST construit à la main, sans Parse
		}
		ps := n.phrase
		if len(ps) == 0 {
			return out
		}
		for i := 0; i+len(ps) <= len(ws); i++ {
			ok := true
			for j, p := range ps {
				if ws[i+j].text != p {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, span{i, i + len(ps) - 1})
			}
		}
	case "term":
		for i, w := range ws {
			if n.matchValue(w.text) {
				out = append(out, span{i, i})
			}
		}
	}
	return out
}

// foldAccents retire les diacritiques des lettres latines minuscules (Latin-1 et Latin étendu A).
func foldAccents(s string) string {
	if isASCII(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if f, ok := accentFold[r]; ok {
			b.WriteString(f)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

var accentFold = func() map[rune]string {
	m := map[rune]string{}
	groups := map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő",
		"r": "ŕŗř", "s": "śŝşš", "t": "ţťŧ", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
		"ae": "æ", "oe": "œ", "ss": "ß",
	}
	for base, letters := range groups {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()
