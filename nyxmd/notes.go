package nyxmd

// Couche de notes (NYX-MD 0.3).
//
// Un passage annoté est encadré de repères invisibles dans tout lecteur Markdown :
//
//	Le <!--n1-->budget validé<!--/n1--> pour Q1.
//
// Les notes sont rangées dans un bloc commentaire en fin de fichier :
//
//	<!-- notes v1
//	[n1] (jaune) Vérifier avec compta #urgent
//	-->
//
// Un repère seul (<!--n2-->, sans fermeture) est une note ponctuelle. Une note sans repère
// est orpheline : elle est conservée, jamais perdue. Les textes sont échappés sur une ligne
// (\n, \\, et --\> pour -->). Toutes les positions publiques sont en caractères Unicode (runes).

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// NoteColors liste les couleurs reconnues (noms stockés dans le fichier).
var NoteColors = []string{"jaune", "vert", "bleu", "rose", "rouge", "violet", "orange", "gris"}

// DefaultNoteColor est la couleur d'une note créée sans couleur.
const DefaultNoteColor = "jaune"

// NotesBlockHeader ouvre le bloc écrit par le moteur. À la lecture, « notes:<app> v1 » est aussi accepté.
const NotesBlockHeader = "<!-- notes v1"

// Codes d'erreur et de diagnostic stables de la couche de notes.
const (
	NoteErrOutOfRange   = "out_of_range"
	NoteErrInsideMarker = "inside_marker"
	NoteErrUnknownColor = "unknown_color"
	NoteErrUnknownNote  = "unknown_note"

	NoteDiagInvalidLine     = "invalid_note_line"
	NoteDiagDuplicateNote   = "duplicate_note"
	NoteDiagDuplicateMarker = "duplicate_marker"
	NoteDiagUnpairedMarker  = "unpaired_marker"
	NoteDiagMarkerNoNote    = "marker_without_note"
	NoteDiagUnknownColor    = "unknown_color"
)

// Note est une note du bloc. Start/End délimitent le passage annoté dans le texte (runes,
// contenu entre les repères) ; Start == End pour une note ponctuelle ; -1 si orpheline.
type Note struct {
	ID     string `json:"id"`
	Color  string `json:"color"`
	Text   string `json:"text"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Orphan bool   `json:"orphan"`
}

// Marker est un repère <!--nX--> ou <!--/nX--> (plage du commentaire, en runes).
type Marker struct {
	ID      string `json:"id"`
	Closing bool   `json:"closing"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

// NotesDoc est le résultat de ParseNotes.
type NotesDoc struct {
	Notes   []Note   `json:"notes"`
	Markers []Marker `json:"markers"`
	// Block couvre le bloc de notes et les lignes vides qui le précèdent (nil s'il n'y en a pas).
	Block *Span `json:"block,omitempty"`
	// Extra garde les lignes du bloc non reconnues ; elles sont réécrites telles quelles.
	Extra       []string     `json:"extra,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	// Masked est le texte où repères et bloc sont remplacés par des espaces (retours à la ligne
	// conservés) : même longueur en runes, positions et numéros de ligne inchangés. À utiliser
	// comme Document.Text pour chercher sans que les repères ne soient pris pour des mots.
	Masked string `json:"masked"`
}

// NoteError décrit une modification refusée.
type NoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *NoteError) Error() string { return e.Message }

// Lecture à la main, sans regexp (le paquet regexp ajoute ~0,8 Mo à la bibliothèque).

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// noteID lit « n » suivi de 1 à 6 chiffres au début de s ; renvoie sa longueur (0 si absent).
func noteID(s string) int {
	if len(s) < 2 || s[0] != 'n' {
		return 0
	}
	i := 1
	for i < len(s) && i <= 6 && isDigit(s[i]) {
		i++
	}
	if i == 1 || (i < len(s) && isDigit(s[i])) {
		return 0
	}
	return i
}

// findMarkers trouve les repères <!--nX--> et <!--/nX--> (positions en octets).
func findMarkers(s string) []bmarker {
	var out []bmarker
	for i := 0; ; {
		k := strings.Index(s[i:], "<!--")
		if k < 0 {
			return out
		}
		a := i + k
		j := a + 4
		closing := j < len(s) && s[j] == '/'
		if closing {
			j++
		}
		if n := noteID(s[j:]); n > 0 && strings.HasPrefix(s[j+n:], "-->") {
			out = append(out, bmarker{s[j : j+n], closing, a, j + n + 3})
			i = j + n + 3
			continue
		}
		i = a + 4
	}
}

func skipBlanks(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

// blockHeader : « <!-- notes v1 » ou « <!-- notes:app v1 » suivi d'une fin de ligne, à la position a.
// Renvoie le début du contenu (après le retour à la ligne) ou -1.
func blockHeader(s string, a int) int {
	i := skipBlanks(s, a+4)
	if !strings.HasPrefix(s[i:], "notes") {
		return -1
	}
	i += 5
	if i < len(s) && s[i] == ':' {
		j := i + 1
		for j < len(s) && (isDigit(s[j]) || s[j] == '_' || s[j] == '-' || (s[j]|0x20 >= 'a' && s[j]|0x20 <= 'z')) {
			j++
		}
		if j == i+1 {
			return -1
		}
		i = j
	}
	j := skipBlanks(s, i)
	if j == i || !strings.HasPrefix(s[j:], "v1") {
		return -1
	}
	j = skipBlanks(s, j+2)
	if j < len(s) && s[j] == '\r' {
		j++
	}
	if j >= len(s) || s[j] != '\n' {
		return -1
	}
	return j + 1
}

// noteLine lit « [nX] (couleur) texte » (couleur et texte facultatifs).
func noteLine(l string) (id, color, text string, ok bool) {
	if len(l) < 4 || l[0] != '[' {
		return
	}
	n := noteID(l[1:])
	if n == 0 || 1+n >= len(l) || l[1+n] != ']' {
		return
	}
	id, rest := l[1:1+n], l[2+n:]
	if rest == "" {
		return id, "", "", true
	}
	r := skipBlanks(rest, 0)
	if r == 0 {
		return "", "", "", false
	}
	rest = rest[r:]
	if strings.HasPrefix(rest, "(") {
		e := strings.IndexByte(rest, ')')
		if e < 0 || strings.ContainsAny(rest[1:e], " \t") {
			return "", "", "", false
		}
		color, rest = rest[1:e], rest[e+1:]
		if rest == "" {
			return id, color, "", true
		}
		r = skipBlanks(rest, 0)
		if r == 0 {
			return "", "", "", false
		}
		rest = rest[r:]
	}
	return id, color, rest, true
}

// bmarker / bnote : positions en octets, internes.
type bmarker struct {
	id      string
	closing bool
	s, e    int
}

type bnote struct {
	id, color, text string
}

type parsedNotes struct {
	text       string
	bodyEnd    int // début du bloc (lignes vides comprises), ou len(text)
	blockFound bool
	notes      []bnote
	extra      []string
	markers    []bmarker
	diags      []Diagnostic
}

func runeOffset(s string, b int) int { return utf8.RuneCountInString(s[:b]) }

func byteOffset(s string, r int) (int, bool) {
	if r < 0 {
		return 0, false
	}
	n := 0
	for i := range s {
		if n == r {
			return i, true
		}
		n++
	}
	return len(s), n == r
}

func validColor(c string) bool {
	for _, k := range NoteColors {
		if k == c {
			return true
		}
	}
	return false
}

func escapeNote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", `\n`)
	return strings.ReplaceAll(s, "-->", `--\>`)
}

func unescapeNote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			case '>':
				b.WriteByte('>')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func diag(sev, code, msg, name string, s, e int) Diagnostic {
	return Diagnostic{Severity: sev, Code: code, Message: msg, Name: name, Start: s, End: e}
}

func parseNotes(text string) *parsedNotes {
	p := &parsedNotes{text: text, bodyEnd: len(text)}
	// Bloc : le dernier, suivi seulement d'espaces.
	for a := len(text); ; {
		a = strings.LastIndex(text[:a], "<!--")
		if a < 0 {
			break
		}
		cs := blockHeader(text, a)
		if cs < 0 {
			continue
		}
		ce := strings.Index(text[cs:], "-->")
		if ce < 0 || strings.TrimSpace(text[cs+ce+3:]) != "" {
			continue
		}
		ce += cs
		p.blockFound = true
		start := a
		for start > 0 && strings.ContainsRune("\n\r \t", rune(text[start-1])) {
			start--
		}
		p.bodyEnd = start
		seen := map[string]bool{}
		off := cs
		for _, line := range strings.SplitAfter(text[cs:ce], "\n") {
			ls := off
			off += len(line)
			l := strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(l) == "" {
				continue
			}
			id, color, body, ok := noteLine(strings.TrimSpace(l))
			if !ok {
				p.extra = append(p.extra, l)
				p.diags = append(p.diags, diag("warning", NoteDiagInvalidLine, "ligne de note illisible (conservée)", "",
					runeOffset(text, ls), runeOffset(text, ls+len(l))))
				continue
			}
			if color == "" {
				color = DefaultNoteColor
			} else if !validColor(color) {
				p.diags = append(p.diags, diag("warning", NoteDiagUnknownColor, "couleur inconnue « "+color+" »", id,
					runeOffset(text, ls), runeOffset(text, ls+len(l))))
			}
			if seen[id] {
				p.extra = append(p.extra, l)
				p.diags = append(p.diags, diag("warning", NoteDiagDuplicateNote, "note "+id+" en double (conservée telle quelle)", id,
					runeOffset(text, ls), runeOffset(text, ls+len(l))))
				continue
			}
			seen[id] = true
			p.notes = append(p.notes, bnote{id, color, unescapeNote(body)})
		}
		break
	}
	p.markers = findMarkers(text[:p.bodyEnd])
	return p
}

// pairs renvoie, par identifiant, le premier repère ouvrant et la fermeture qui le suit.
func (p *parsedNotes) pairs() (open map[string]*bmarker, close map[string]*bmarker) {
	open, close = map[string]*bmarker{}, map[string]*bmarker{}
	for i := range p.markers {
		m := &p.markers[i]
		switch {
		case !m.closing && open[m.id] == nil:
			open[m.id] = m
		case m.closing && open[m.id] != nil && close[m.id] == nil:
			close[m.id] = m
		}
	}
	return
}

// ParseNotes lit les repères et le bloc de notes. Ne modifie rien.
func ParseNotes(text string) NotesDoc {
	p := parseNotes(text)
	open, close := p.pairs()
	doc := NotesDoc{Notes: []Note{}, Markers: []Marker{}, Extra: p.extra, Diagnostics: p.diags}
	if doc.Diagnostics == nil {
		doc.Diagnostics = []Diagnostic{}
	}
	inBlock := map[string]bool{}
	for _, n := range p.notes {
		inBlock[n.id] = true
		note := Note{ID: n.id, Color: n.color, Text: n.text, Start: -1, End: -1, Orphan: true}
		if o := open[n.id]; o != nil {
			note.Orphan = false
			note.Start = runeOffset(text, o.e)
			note.End = note.Start
			if c := close[n.id]; c != nil {
				note.End = runeOffset(text, c.s)
			}
		}
		doc.Notes = append(doc.Notes, note)
	}
	seenOpen := map[string]bool{}
	for i := range p.markers {
		m := &p.markers[i]
		rs, re := runeOffset(text, m.s), runeOffset(text, m.e)
		doc.Markers = append(doc.Markers, Marker{m.id, m.closing, rs, re})
		switch {
		case !m.closing && seenOpen[m.id]:
			doc.Diagnostics = append(doc.Diagnostics, diag("warning", NoteDiagDuplicateMarker, "repère "+m.id+" en double", m.id, rs, re))
		case m.closing && close[m.id] != m:
			doc.Diagnostics = append(doc.Diagnostics, diag("warning", NoteDiagUnpairedMarker, "fermeture "+m.id+" sans ouverture", m.id, rs, re))
		case !m.closing && !inBlock[m.id]:
			doc.Diagnostics = append(doc.Diagnostics, diag("warning", NoteDiagMarkerNoNote, "repère "+m.id+" sans note dans le bloc", m.id, rs, re))
		}
		if !m.closing {
			seenOpen[m.id] = true
		}
	}
	if p.blockFound {
		doc.Block = &Span{runeOffset(text, p.bodyEnd), utf8.RuneCountInString(text)}
	}
	doc.Masked = maskNotes(p)
	return doc
}

func maskNotes(p *parsedNotes) string {
	if len(p.markers) == 0 && !p.blockFound {
		return p.text
	}
	var b strings.Builder
	b.Grow(len(p.text))
	blank := func(s string) {
		for _, r := range s {
			if r == '\n' || r == '\r' {
				b.WriteRune(r)
			} else {
				b.WriteByte(' ')
			}
		}
	}
	prev := 0
	for _, m := range p.markers {
		b.WriteString(p.text[prev:m.s])
		blank(p.text[m.s:m.e])
		prev = m.e
	}
	b.WriteString(p.text[prev:p.bodyEnd])
	blank(p.text[p.bodyEnd:])
	return b.String()
}

// MaskNotes : voir NotesDoc.Masked.
func MaskNotes(text string) string { return maskNotes(parseNotes(text)) }

// body sans repères ; le bloc est retiré avec les lignes vides qui le précèdent.
func (p *parsedNotes) cleanBody(anchor func(m *bmarker) string) string {
	var b strings.Builder
	prev := 0
	for i := range p.markers {
		m := &p.markers[i]
		b.WriteString(p.text[prev:m.s])
		if anchor != nil {
			b.WriteString(anchor(m))
		}
		prev = m.e
	}
	b.WriteString(p.text[prev:p.bodyEnd])
	return b.String()
}

func finishBody(body string, hadBlock bool) string {
	if !hadBlock {
		return body
	}
	body = strings.TrimRight(body, " \t\r\n")
	if body == "" {
		return ""
	}
	return body + "\n"
}

// StripNotes renvoie le texte seul : repères et bloc de notes retirés (export « propre »).
func StripNotes(text string) string {
	p := parseNotes(text)
	return finishBody(p.cleanBody(nil), p.blockFound)
}

// NotesToFootnotes convertit la couche de notes en notes de bas de page Markdown standard :
// chaque passage annoté reçoit un appel [^k] (après le passage, numéroté dans l'ordre du texte),
// les notes deviennent des définitions [^k]: texte en fin de document. Les notes orphelines
// et les repères sans note sont omis.
func NotesToFootnotes(text string) string {
	p := parseNotes(text)
	open, close := p.pairs()
	byID := map[string]bnote{}
	for _, n := range p.notes {
		byID[n.id] = n
	}
	num := map[string]int{}
	var defs []string
	// Ordre des appels : position de l'ouverture.
	for i := range p.markers {
		m := &p.markers[i]
		if m.closing || open[m.id] != m {
			continue
		}
		n, ok := byID[m.id]
		if !ok {
			continue
		}
		num[m.id] = len(defs) + 1
		lines := strings.Split(n.text, "\n")
		def := "[^" + strconv.Itoa(len(defs)+1) + "]: " + lines[0]
		for _, l := range lines[1:] {
			def += "\n    " + l
		}
		defs = append(defs, def)
	}
	body := p.cleanBody(func(m *bmarker) string {
		k, ok := num[m.id]
		if !ok {
			return ""
		}
		if c := close[m.id]; c != nil {
			if c == m {
				return "[^" + strconv.Itoa(k) + "]"
			}
			return ""
		}
		if open[m.id] == m { // note ponctuelle
			return "[^" + strconv.Itoa(k) + "]"
		}
		return ""
	})
	if len(defs) == 0 {
		return finishBody(body, p.blockFound)
	}
	body = strings.TrimRight(body, " \t\r\n")
	if body != "" {
		body += "\n\n"
	}
	return body + strings.Join(defs, "\n") + "\n"
}

// serialize réécrit le texte : corps (repères compris), puis le bloc s'il reste des notes.
func serializeNotes(body string, notes []bnote, extra []string) string {
	body = strings.TrimRight(body, " \t\r\n")
	if len(notes) == 0 && len(extra) == 0 {
		if body == "" {
			return ""
		}
		return body + "\n"
	}
	var b strings.Builder
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	b.WriteString(NotesBlockHeader + "\n")
	for _, n := range notes {
		b.WriteString("[" + n.id + "] (" + n.color + ")")
		if n.text != "" {
			b.WriteString(" " + escapeNote(n.text))
		}
		b.WriteByte('\n')
	}
	for _, l := range extra {
		b.WriteString(l + "\n")
	}
	b.WriteString("-->\n")
	return b.String()
}

func (p *parsedNotes) nextID() string {
	max := 0
	consider := func(id string) {
		if n, err := strconv.Atoi(id[1:]); err == nil && n > max {
			max = n
		}
	}
	for _, n := range p.notes {
		consider(n.id)
	}
	for _, m := range p.markers {
		consider(m.id)
	}
	return "n" + strconv.Itoa(max+1)
}

// AddNote annote le passage [start, end) (runes ; start == end pour une note ponctuelle)
// et ajoute la note au bloc. Renvoie le nouveau texte et l'identifiant attribué.
func AddNote(text string, start, end int, color, body string) (string, string, error) {
	p := parseNotes(text)
	if color == "" {
		color = DefaultNoteColor
	}
	if !validColor(color) {
		return "", "", &NoteError{NoteErrUnknownColor, "couleur inconnue « " + color + " » (" + strings.Join(NoteColors, ", ") + ")"}
	}
	limit := utf8.RuneCountInString(text[:p.bodyEnd])
	if start < 0 || end < start || end > limit {
		return "", "", &NoteError{NoteErrOutOfRange, "passage hors du texte (0 à " + strconv.Itoa(limit) + ")"}
	}
	bs, _ := byteOffset(text, start)
	be, _ := byteOffset(text, end)
	for _, m := range p.markers {
		if (bs > m.s && bs < m.e) || (be > m.s && be < m.e) {
			return "", "", &NoteError{NoteErrInsideMarker, "le passage commence ou finit dans un repère de note"}
		}
	}
	id := p.nextID()
	src := text[:p.bodyEnd]
	var nb string
	if be > bs {
		nb = src[:bs] + "<!--" + id + "-->" + src[bs:be] + "<!--/" + id + "-->" + src[be:]
	} else {
		nb = src[:bs] + "<!--" + id + "-->" + src[bs:]
	}
	notes := append(append([]bnote{}, p.notes...), bnote{id, color, body})
	return serializeNotes(nb, notes, p.extra), id, nil
}

// UpdateNote change la couleur (vide = inchangée) et le texte d'une note du bloc.
func UpdateNote(text, id, color, body string) (string, error) {
	p := parseNotes(text)
	if color != "" && !validColor(color) {
		return "", &NoteError{NoteErrUnknownColor, "couleur inconnue « " + color + " » (" + strings.Join(NoteColors, ", ") + ")"}
	}
	notes := append([]bnote{}, p.notes...)
	for i := range notes {
		if notes[i].id == id {
			if color != "" {
				notes[i].color = color
			}
			notes[i].text = body
			return serializeNotes(text[:p.bodyEnd], notes, p.extra), nil
		}
	}
	return "", &NoteError{NoteErrUnknownNote, "note « " + id + " » absente du bloc"}
}

// DeleteNote retire la note du bloc et ses repères (le passage reste dans le texte).
func DeleteNote(text, id string) (string, error) {
	p := parseNotes(text)
	found := false
	var notes []bnote
	for _, n := range p.notes {
		if n.id == id {
			found = true
			continue
		}
		notes = append(notes, n)
	}
	var b strings.Builder
	prev := 0
	for _, m := range p.markers {
		if m.id != id {
			continue
		}
		found = true
		b.WriteString(text[prev:m.s])
		prev = m.e
	}
	b.WriteString(text[prev:p.bodyEnd])
	if !found {
		return "", &NoteError{NoteErrUnknownNote, "note « " + id + " » introuvable"}
	}
	return serializeNotes(b.String(), notes, p.extra), nil
}

// NotedDocument prépare un document de recherche à partir d'un texte annoté :
// texte masqué (repères et bloc invisibles pour la recherche), textes et couleurs des notes
// (pour note: et couleur:).
func NotedDocument(text string, tags, entities []string) Document {
	doc := ParseNotes(text)
	d := Document{Text: doc.Masked, Tags: tags, Entities: entities}
	for _, n := range doc.Notes {
		d.Notes = append(d.Notes, n.Text)
		d.Colors = append(d.Colors, n.Color)
	}
	return d
}
