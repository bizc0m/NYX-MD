// Package nyxmd est le moteur partagé NYX-MD : recherche booléenne et variables.
//
// La grammaire de recherche reprend NyxBoolean (Nyx, révision 4473c1e) sans changer
// ses règles : mots entiers, opérateurs en majuscules, jokers, NEAR/n, métadonnées
// #tag et @entité explicites. Ajouts NYX-MD : erreurs structurées avec position,
// limite de profondeur, motifs précompilés, requête vide = tous les documents (Compile).
package nyxmd

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Limites de complexité d'une requête.
const (
	MaxQueryBytes   = 4096
	MaxQueryDepth   = 64
	MaxNearDistance = 4096
)

// Document contient le texte et les métadonnées explicites fournis par l'appelant.
type Document struct {
	Text     string   `json:"text"`
	Tags     []string `json:"tags,omitempty"`
	Entities []string `json:"entities,omitempty"`
}

// Node est l'AST portable. Op vaut term, phrase, tag, entity, AND, OR, NOT ou NEAR.
type Node struct {
	Op, Value   string
	Distance    int
	Left, Right *Node
	matcher     *regexp.Regexp // joker précompilé (term, tag, entity)
	phrase      []string       // mots d'une expression exacte
}

// QueryError décrit une requête invalide. Position est un décalage en caractères
// Unicode (runes) depuis le début de la requête, base 0.
type QueryError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

func (e *QueryError) Error() string {
	return e.Message + " (position " + strconv.Itoa(e.Position) + ")"
}

// Codes d'erreur stables.
const (
	ErrTooLong            = "query_too_long"
	ErrTooDeep            = "query_too_deep"
	ErrUnterminatedQuote  = "unterminated_quote"
	ErrEmptyOperand       = "empty_operand"
	ErrExpectedOperand    = "expected_operand"
	ErrUnexpectedToken    = "unexpected_token"
	ErrMissingParenthesis = "missing_closing_parenthesis"
	ErrInvalidProximity   = "invalid_proximity"
	ErrNearOperand        = "near_operand"
)

func qerr(code string, pos int, msg string) *QueryError {
	return &QueryError{Code: code, Message: msg, Position: pos}
}

type token struct {
	kind, value string
	pos         int
}

type parser struct {
	tokens []token
	pos    int
	depth  int
}

// Parse analyse une requête stricte (une requête vide est une erreur, comme NyxBoolean).
// Un espace entre deux expressions vaut AND.
func Parse(query string) (*Node, error) {
	if len(query) > MaxQueryBytes {
		return nil, qerr(ErrTooLong, 0, "requête trop longue (maximum "+strconv.Itoa(MaxQueryBytes)+" octets)")
	}
	ts, err := lex(query)
	if err != nil {
		return nil, err
	}
	p := parser{tokens: ts}
	n, err := p.expr(1)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != "EOF" {
		if t.kind == ")" {
			return nil, qerr(ErrUnexpectedToken, t.pos, "parenthèse fermante sans ouverture")
		}
		return nil, qerr(ErrUnexpectedToken, t.pos, "élément inattendu « "+t.value+" »")
	}
	compile(n)
	return n, nil
}

// Query est une requête compilée, prête à être évaluée sur de nombreux documents.
type Query struct {
	root *Node
}

// Compile accepte une requête vide ou blanche, qui correspond à tous les documents.
func Compile(query string) (*Query, error) {
	if strings.TrimSpace(query) == "" {
		return &Query{}, nil
	}
	n, err := Parse(query)
	if err != nil {
		return nil, err
	}
	return &Query{root: n}, nil
}

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

// Root renvoie l'AST (nil pour une requête vide).
func (q *Query) Root() *Node { return q.root }

// Terms liste les termes et expressions de texte non niés, pour surligner des extraits.
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
	walk(q.root, false)
	return out
}

func lex(s string) ([]token, error) {
	r := []rune(s)
	out := []token{}
	for i := 0; i < len(r); {
		if unicode.IsSpace(r[i]) {
			i++
			continue
		}
		if strings.ContainsRune("()-", r[i]) {
			out = append(out, token{string(r[i]), string(r[i]), i})
			i++
			continue
		}
		start := i
		kind := "term"
		if r[i] == '#' || r[i] == '@' {
			if r[i] == '#' {
				kind = "tag"
			} else {
				kind = "entity"
			}
			i++
		}
		value := ""
		quoted := i < len(r) && r[i] == '"'
		if quoted {
			i++
			closed := false
			var b strings.Builder
			for i < len(r) {
				if r[i] == '"' {
					i++
					closed = true
					break
				}
				if r[i] == '\\' {
					i++
					if i >= len(r) {
						break
					}
				}
				b.WriteRune(r[i])
				i++
			}
			if !closed {
				return nil, qerr(ErrUnterminatedQuote, start, "guillemet non fermé")
			}
			value = b.String()
			if kind == "term" {
				kind = "phrase"
			}
		} else {
			vs := i
			for i < len(r) && !unicode.IsSpace(r[i]) && !strings.ContainsRune("()\"", r[i]) {
				i++
			}
			value = string(r[vs:i])
		}
		if strings.TrimSpace(value) == "" {
			return nil, qerr(ErrEmptyOperand, start, "élément vide")
		}
		if kind == "term" {
			switch value {
			case "AND", "OR", "NOT":
				kind = value
			}
			if strings.HasPrefix(value, "NEAR/") {
				kind = "NEAR"
			}
		}
		out = append(out, token{kind, value, start})
	}
	return append(out, token{"EOF", "", len(r)}), nil
}

func (p *parser) peek() token { return p.tokens[p.pos] }

func (p *parser) enter(pos int) error {
	p.depth++
	if p.depth > MaxQueryDepth {
		return qerr(ErrTooDeep, pos, "requête trop imbriquée (maximum "+strconv.Itoa(MaxQueryDepth)+" niveaux)")
	}
	return nil
}

func (p *parser) expr(min int) (*Node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		op := t.kind
		prec := 0
		implicit := false
		switch op {
		case "OR":
			prec = 1
		case "AND":
			prec = 2
		case "NEAR":
			prec = 3
		case "term", "phrase", "tag", "entity", "(", "NOT", "-":
			op = "AND"
			prec = 2
			implicit = true
		}
		if prec < min {
			break
		}
		distance := 0
		if op == "NEAR" {
			d, e := strconv.Atoi(strings.TrimPrefix(t.value, "NEAR/"))
			if e != nil || d < 0 || d > MaxNearDistance {
				return nil, qerr(ErrInvalidProximity, t.pos, "distance NEAR invalide « "+t.value+" » (0 à "+strconv.Itoa(MaxNearDistance)+")")
			}
			distance = d
		}
		if !implicit {
			p.pos++
		}
		if err := p.enter(t.pos); err != nil {
			return nil, err
		}
		right, e := p.expr(prec + 1)
		p.depth--
		if e != nil {
			return nil, e
		}
		if op == "NEAR" && (!positional(left) || !positional(right)) {
			return nil, qerr(ErrNearOperand, t.pos, "NEAR n'accepte que des mots, des expressions ou d'autres NEAR")
		}
		left = &Node{Op: op, Distance: distance, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) unary() (*Node, error) {
	t := p.peek()
	switch t.kind {
	case "NOT", "-":
		p.pos++
		if err := p.enter(t.pos); err != nil {
			return nil, err
		}
		n, e := p.unary()
		p.depth--
		if e != nil {
			return nil, e
		}
		return &Node{Op: "NOT", Left: n}, nil
	case "(":
		p.pos++
		if err := p.enter(t.pos); err != nil {
			return nil, err
		}
		n, e := p.expr(1)
		p.depth--
		if e != nil {
			return nil, e
		}
		if p.peek().kind != ")" {
			return nil, qerr(ErrMissingParenthesis, t.pos, "parenthèse ouverte non fermée")
		}
		p.pos++
		return n, nil
	case "term", "phrase", "tag", "entity":
		p.pos++
		return &Node{Op: t.kind, Value: t.value}, nil
	case "EOF":
		return nil, qerr(ErrExpectedOperand, t.pos, "terme attendu en fin de requête")
	default:
		return nil, qerr(ErrExpectedOperand, t.pos, "terme attendu avant « "+t.value+" »")
	}
}

func positional(n *Node) bool {
	return n != nil && (n.Op == "term" || n.Op == "phrase" || n.Op == "NEAR")
}

// compile précalcule les jokers et les mots des expressions (une seule fois par requête).
func compile(n *Node) {
	if n == nil {
		return
	}
	switch n.Op {
	case "term", "tag", "entity":
		n.matcher = globRegexp(n.Value)
	case "phrase":
		n.phrase = words(n.Value)
	}
	compile(n.Left)
	compile(n.Right)
}

// Match évalue l'AST. Les mots du texte sont découpés une seule fois par appel.
func (n *Node) Match(d Document) bool {
	var ws []string
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
		case "tag", "entity":
			values := d.Tags
			if n.Op == "entity" {
				values = d.Entities
			}
			for _, v := range values {
				if n.glob(v) {
					return true
				}
			}
			return false
		default:
			if !split {
				ws, split = words(d.Text), true
			}
			return len(n.spans(ws)) > 0
		}
	}
	return eval(n)
}

type span struct{ start, end int }

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_'
	})
}

func globRegexp(pattern string) *regexp.Regexp {
	p := regexp.QuoteMeta(strings.ToLower(pattern))
	p = strings.ReplaceAll(p, `\*`, ".*")
	p = strings.ReplaceAll(p, `\?`, ".")
	re, err := regexp.Compile("^(?:" + p + ")$")
	if err != nil {
		return nil
	}
	return re
}

func (n *Node) glob(value string) bool {
	re := n.matcher
	if re == nil {
		re = globRegexp(n.Value) // AST construit à la main, sans compile()
	}
	return re != nil && re.MatchString(strings.ToLower(value))
}

func (n *Node) spans(ws []string) []span {
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
		ps := n.phrase
		if ps == nil {
			ps = words(n.Value)
		}
		if len(ps) == 0 {
			return out
		}
		for i := 0; i+len(ps) <= len(ws); i++ {
			ok := true
			for j, p := range ps {
				if ws[i+j] != p {
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
			if n.glob(w) {
				out = append(out, span{i, i})
			}
		}
	}
	return out
}
