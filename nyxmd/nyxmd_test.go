package nyxmd

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

type searchVectors struct {
	Documents map[string]Document `json:"documents"`
	Cases     []struct {
		Doc   string `json:"doc"`
		Query string `json:"query"`
		Match bool   `json:"match"`
	} `json:"cases"`
	Invalid []struct {
		Query    string `json:"query"`
		Code     string `json:"code"`
		Position int    `json:"position"`
	} `json:"invalid"`
}

type variableVectors struct {
	Context Context `json:"context"`
	Render  []struct {
		Template string   `json:"template"`
		Output   string   `json:"output"`
		Warnings []string `json:"warnings"`
	} `json:"render"`
	PersonalText []struct {
		Text   string   `json:"text"`
		Names  []string `json:"names"`
		Errors []string `json:"errors"`
	} `json:"personal_text"`
	Complete []struct {
		Text   string   `json:"text"`
		Caret  int      `json:"caret"`
		Start  int      `json:"start"`
		End    int      `json:"end"`
		Labels []string `json:"labels"`
		None   bool     `json:"none"`
	} `json:"complete"`
}

func load(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestSearchVectors(t *testing.T) {
	var v searchVectors
	load(t, "../testdata/search.json", &v)
	for _, c := range v.Cases {
		t.Run(c.Doc+"/"+c.Query, func(t *testing.T) {
			q, err := Compile(c.Query)
			if err != nil {
				t.Fatal(err)
			}
			if got := q.Match(v.Documents[c.Doc]); got != c.Match {
				t.Fatalf("got %v want %v", got, c.Match)
			}
		})
	}
	for _, c := range v.Invalid {
		t.Run("invalid/"+c.Query, func(t *testing.T) {
			_, err := Compile(c.Query)
			var qe *QueryError
			if !errors.As(err, &qe) {
				t.Fatalf("accepted or untyped error: %v", err)
			}
			if qe.Code != c.Code || qe.Position != c.Position {
				t.Fatalf("got %s@%d (%s) want %s@%d", qe.Code, qe.Position, qe.Message, c.Code, c.Position)
			}
		})
	}
}

func TestStrictParseRejectsEmpty(t *testing.T) {
	// Contrat NyxBoolean conservé : Parse refuse une requête vide ; Compile l'accepte.
	for _, q := range []string{"", " "} {
		if _, err := Parse(q); err == nil {
			t.Fatalf("Parse(%q) accepted", q)
		}
	}
}

func TestLimits(t *testing.T) {
	if _, err := Compile(strings.Repeat("a ", MaxQueryBytes)); err == nil || err.(*QueryError).Code != ErrTooLong {
		t.Fatalf("length: %v", err)
	}
	deep := strings.Repeat("(", MaxQueryDepth+1) + "a" + strings.Repeat(")", MaxQueryDepth+1)
	if _, err := Compile(deep); err == nil || err.(*QueryError).Code != ErrTooDeep {
		t.Fatalf("depth: %v", err)
	}
	ok := strings.Repeat("(", MaxQueryDepth-1) + "a" + strings.Repeat(")", MaxQueryDepth-1)
	if _, err := Compile(ok); err != nil {
		t.Fatalf("depth limit too strict: %v", err)
	}
	nots := strings.Repeat("NOT ", MaxQueryDepth+1) + "a"
	if _, err := Compile(nots); err == nil || err.(*QueryError).Code != ErrTooDeep {
		t.Fatalf("NOT chain: %v", err)
	}
	// Une longue suite de termes n'est pas une imbrication.
	if _, err := Compile(strings.Repeat("mot ", 800)); err != nil {
		t.Fatalf("flat query rejected: %v", err)
	}
}

func TestTerms(t *testing.T) {
	q, _ := Compile(`(client OR "à relancer") -archivé NOT (x OR y) #tag`)
	if got := q.Terms(); !reflect.DeepEqual(got, []string{"client", "à relancer"}) {
		t.Fatalf("terms %v", got)
	}
}

func TestVariableVectors(t *testing.T) {
	var v variableVectors
	load(t, "../testdata/variables.json", &v)
	for _, c := range v.Render {
		t.Run("render/"+c.Template, func(t *testing.T) {
			out, diags := Render(c.Template, v.Context, AllSyntaxes)
			if out != c.Output {
				t.Fatalf("got %q want %q", out, c.Output)
			}
			var names []string
			for _, d := range diags {
				names = append(names, d.Name)
			}
			if len(names) != len(c.Warnings) || (len(names) > 0 && !reflect.DeepEqual(names, c.Warnings)) {
				t.Fatalf("warnings %v want %v", names, c.Warnings)
			}
		})
	}
	for _, c := range v.PersonalText {
		t.Run("personal/"+c.Text, func(t *testing.T) {
			vars, diags := ParsePersonal(c.Text)
			names := []string{}
			for _, x := range vars {
				names = append(names, x.Name)
			}
			codes := []string{}
			for _, d := range diags {
				codes = append(codes, d.Code)
			}
			sort.Strings(codes)
			want := append([]string{}, c.Errors...)
			sort.Strings(want)
			if !reflect.DeepEqual(names, c.Names) || !reflect.DeepEqual(codes, want) {
				t.Fatalf("names %v codes %v", names, codes)
			}
		})
	}
	for _, c := range v.Complete {
		t.Run("complete/"+c.Text, func(t *testing.T) {
			got := Complete(c.Text, c.Caret, v.Context, AllSyntaxes)
			if c.None {
				if got != nil {
					t.Fatalf("unexpected completion %+v", got)
				}
				return
			}
			if got == nil || got.Start != c.Start || got.End != c.End {
				t.Fatalf("range %+v", got)
			}
			var labels []string
			for _, it := range got.Items {
				labels = append(labels, it.Label)
			}
			if !reflect.DeepEqual(labels, c.Labels) {
				t.Fatalf("labels %v", labels)
			}
		})
	}
}

func TestSinglePassNoRecursion(t *testing.T) {
	// La valeur d'une variable contenant des variables n'est jamais redéveloppée.
	ctx := Context{Text: "{{texte}} $date {{date}}", Personal: []Variable{{"a", "$b"}, {"b", "$a"}}}
	out, _ := Render("{{texte}} $a $b", ctx, AllSyntaxes)
	if out != "{{texte}} $date {{date}} $b $a" {
		t.Fatalf("got %q", out)
	}
}

func TestSyntaxSelection(t *testing.T) {
	ctx := Context{Timestamp: 0}
	out, _ := Render("{{date}} $date", ctx, Syntax{Dollar: true})
	if out != "{{date}} 1970-01-01" {
		t.Fatalf("dollar only: %q", out)
	}
	out, _ = Render("{{date}} $date", ctx, Syntax{Braces: true})
	if out != "1970-01-01 $date" {
		t.Fatalf("braces only: %q", out)
	}
}

func TestPersonalNeverOverridesNative(t *testing.T) {
	ctx := Context{Timestamp: 0, Personal: []Variable{{"date", "PIRATE"}}}
	out, diags := Render("$date", ctx, AllSyntaxes)
	if out != "1970-01-01" || len(diags) != 1 || diags[0].Code != DiagReservedName {
		t.Fatalf("got %q %v", out, diags)
	}
}

func TestEnglishDay(t *testing.T) {
	out, _ := Render("$day", Context{Timestamp: 1792843509, UTCOffset: 7200, Locale: "en_US"}, AllSyntaxes)
	if out != "Saturday" {
		t.Fatalf("got %q", out)
	}
}

func TestReferencesPositionsUnicode(t *testing.T) {
	refs := References("😀 é $date {{x}}", Context{}, AllSyntaxes)
	if len(refs) != 2 || refs[0].Start != 4 || refs[0].End != 9 || refs[1].Start != 10 || refs[1].Kind != "unknown" {
		t.Fatalf("refs %+v", refs)
	}
}

func FuzzCompile(f *testing.F) {
	for _, s := range []string{"a AND b", "NOT", `@"Élodie Martin"`, `a NEAR/2 b`, "((", "(client OR fournisseur) AND NOT archivé"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q, err := Compile(s)
		if err == nil {
			q.Match(Document{Text: "a b c", Tags: []string{"dev"}})
		} else if _, ok := err.(*QueryError); !ok {
			t.Fatalf("untyped error %v", err)
		}
	})
}

func FuzzRender(f *testing.F) {
	for _, s := range []string{"{{texte}} $date", "$", "{{", "$a$b{{c}}", "😀$client"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ctx := Context{Text: s, Personal: []Variable{{"client", s}}}
		Render(s, ctx, AllSyntaxes)
		References(s, ctx, AllSyntaxes)
		for i := 0; i <= len([]rune(s)); i++ {
			Complete(s, i, ctx, AllSyntaxes)
		}
	})
}

func TestFindSpans(t *testing.T) {
	q, _ := Compile(`(client OR "à relancer") -archivé`)
	ok, spans := q.Find(Document{Text: "😀 client : facture à relancer, client"})
	want := []Span{{2, 8}, {19, 29}, {31, 37}}
	if !ok || !reflect.DeepEqual(spans, want) {
		t.Fatalf("got %v %v", ok, spans)
	}
	if ok, _ := q.Find(Document{Text: "client archivé"}); ok {
		t.Fatal("negation ignored")
	}
	q, _ = Compile("alpha NEAR/1 gamma")
	_, spans = q.Find(Document{Text: "alpha beta gamma"})
	if !reflect.DeepEqual(spans, []Span{{0, 16}}) {
		t.Fatalf("near %v", spans)
	}
	q, _ = Compile("")
	if ok, spans := q.Find(Document{Text: "x"}); !ok || spans != nil {
		t.Fatal("empty query")
	}
	q, _ = Compile("#tag")
	if ok, spans := q.Find(Document{Text: "x", Tags: []string{"tag"}}); !ok || spans != nil {
		t.Fatal("metadata spans")
	}
}

func TestFoldAccents(t *testing.T) {
	d := Document{Text: "Dossier ARCHIVÉ — Œuvre de Zoë, déjà vu", Tags: []string{"Été/Projet"}}
	strict, _ := Compile("archive")
	fold, _ := CompileWith("archive", Options{FoldAccents: true})
	if strict.Match(d) || !fold.Match(d) {
		t.Fatal("fold archive")
	}
	for _, q := range []string{"oeuvre", "zoe", "deja", `"deja vu"`, "#ete/*", "archiv?", "ŒUVRE"} {
		c, err := CompileWith(q, Options{FoldAccents: true})
		if err != nil || !c.Match(d) {
			t.Fatalf("fold %q: %v", q, err)
		}
	}
	_, spans := fold.Find(d)
	if !reflect.DeepEqual(spans, []Span{{8, 15}}) {
		t.Fatalf("fold spans %v", spans)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		p, s string
		ok   bool
	}{
		{"*", "", true}, {"a*", "a", true}, {"a*c", "abbbc", true}, {"a*c", "abbb", false},
		{"?", "é", true}, {"??", "é", false}, {"*a*b*", "xaxxbx", true}, {"a**b", "ab", true},
		{"local-*", "local-first", true}, {"", "", true}, {"", "a", false},
	}
	for _, c := range cases {
		if globMatch([]rune(c.p), []rune(c.s)) != c.ok {
			t.Fatalf("%q ~ %q", c.p, c.s)
		}
	}
}

func TestHandBuiltAST(t *testing.T) {
	n := &Node{Op: "AND", Left: &Node{Op: "term", Value: "Nyx"}, Right: &Node{Op: "phrase", Value: "recherche locale"}}
	if !n.Match(Document{Text: "nyx recherche locale"}) {
		t.Fatal("hand-built AST")
	}
}

var benchText = strings.Repeat("Relance du client Élodie : facture à relancer avant vendredi, projet ACME archivé. ", 50)

func BenchmarkMatchExact(b *testing.B) {
	q, _ := Compile(`(client OR fournisseur) AND NOT archivé AND "à relancer"`)
	d := Document{Text: benchText}
	for i := 0; i < b.N; i++ {
		q.Match(d)
	}
}

func BenchmarkMatchWildcard(b *testing.B) {
	q, _ := Compile(`clien* AND rel?nce*`)
	d := Document{Text: benchText}
	for i := 0; i < b.N; i++ {
		q.Match(d)
	}
}

func TestContractFindAndFold(t *testing.T) {
	var v struct {
		Contract int `json:"contract"`
		Find     []struct {
			Text, Query string
			Match       bool
			Spans       [][2]int
		} `json:"find"`
		Fold []struct {
			Text, Query    string
			Tags           []string
			Strict, Folded bool
		} `json:"fold_accents"`
	}
	load(t, "../testdata/search.json", &v)
	if v.Contract != 2 || len(v.Find) == 0 || len(v.Fold) == 0 {
		t.Fatalf("contract %d", v.Contract)
	}
	for _, c := range v.Find {
		q, err := Compile(c.Query)
		if err != nil {
			t.Fatal(err)
		}
		ok, spans := q.Find(Document{Text: c.Text})
		got := [][2]int{}
		for _, s := range spans {
			got = append(got, [2]int{s.Start, s.End})
		}
		if ok != c.Match || !reflect.DeepEqual(got, c.Spans) {
			t.Fatalf("find %q: %v %v", c.Query, ok, got)
		}
	}
	for _, c := range v.Fold {
		d := Document{Text: c.Text, Tags: c.Tags}
		s, _ := Compile(c.Query)
		f, _ := CompileWith(c.Query, Options{FoldAccents: true})
		if s.Match(d) != c.Strict || f.Match(d) != c.Folded {
			t.Fatalf("fold %q", c.Query)
		}
	}
}

func FuzzFind(f *testing.F) {
	for _, s := range []string{"client", `"à relancer"`, "a NEAR/1 b", "app* -x", "é?"} {
		f.Add(s, "😀 client à relancer app éa")
	}
	f.Fuzz(func(t *testing.T, q, text string) {
		for _, fold := range []bool{false, true} {
			c, err := CompileWith(q, Options{FoldAccents: fold})
			if err != nil {
				continue
			}
			ok, spans := c.Find(Document{Text: text})
			n := len([]rune(text))
			for i, s := range spans {
				if !ok || s.Start < 0 || s.End > n || s.Start >= s.End || (i > 0 && s.Start <= spans[i-1].End-1) {
					t.Fatalf("bad spans %v for %q in %q", spans, q, text)
				}
			}
		}
	})
}

// Couche de notes : vecteurs communs testdata/notes.json.
type notesContract struct {
	Contract  int               `json:"contract"`
	Documents map[string]string `json:"documents"`
	Parse     []struct {
		Doc         string   `json:"doc"`
		Notes       []Note   `json:"notes"`
		Block       *Span    `json:"block"`
		Diagnostics []string `json:"diagnostics"`
		Extra       []string `json:"extra"`
		Masked      *string  `json:"masked"`
	} `json:"parse"`
	Strip, Footnotes []struct {
		Doc    string `json:"doc"`
		Text   string `json:"text"`
		Output string `json:"output"`
	}
	Edits []struct {
		Op, Doc, Text, ID, Color, Body, Output, Error string
		Start, End                                    int
	} `json:"edits"`
	Search []struct {
		Doc, Query string
		Match      bool
	} `json:"search"`
}

func TestNotesContract(t *testing.T) {
	var c notesContract
	load(t, "../testdata/notes.json", &c)
	if c.Contract != 1 {
		t.Fatalf("contract %d", c.Contract)
	}
	text := func(doc, txt string) string {
		if doc != "" {
			return c.Documents[doc]
		}
		return txt
	}
	for _, p := range c.Parse {
		d := ParseNotes(c.Documents[p.Doc])
		if p.Notes != nil && !reflect.DeepEqual(d.Notes, p.Notes) {
			t.Errorf("%s notes = %+v, attendu %+v", p.Doc, d.Notes, p.Notes)
		}
		if p.Block != nil && (d.Block == nil || *d.Block != *p.Block) {
			t.Errorf("%s block = %v", p.Doc, d.Block)
		}
		var codes []string
		for _, g := range d.Diagnostics {
			codes = append(codes, g.Code)
		}
		if len(codes) != len(p.Diagnostics) || (len(codes) > 0 && !reflect.DeepEqual(codes, p.Diagnostics)) {
			t.Errorf("%s diagnostics = %v, attendu %v", p.Doc, codes, p.Diagnostics)
		}
		if p.Extra != nil && !reflect.DeepEqual(d.Extra, p.Extra) {
			t.Errorf("%s extra = %q", p.Doc, d.Extra)
		}
		if p.Masked != nil && d.Masked != *p.Masked {
			t.Errorf("%s masked = %q", p.Doc, d.Masked)
		}
		src := c.Documents[p.Doc]
		if utf8.RuneCountInString(d.Masked) != utf8.RuneCountInString(src) || strings.Count(d.Masked, "\n") != strings.Count(src, "\n") {
			t.Errorf("%s : le masque doit garder longueur et lignes", p.Doc)
		}
	}
	for _, s := range c.Strip {
		if got := StripNotes(text(s.Doc, s.Text)); got != s.Output {
			t.Errorf("strip = %q, attendu %q", got, s.Output)
		}
	}
	for _, s := range c.Footnotes {
		if got := NotesToFootnotes(text(s.Doc, s.Text)); got != s.Output {
			t.Errorf("footnotes = %q, attendu %q", got, s.Output)
		}
	}
	for i, e := range c.Edits {
		src := text(e.Doc, e.Text)
		var out, id string
		var err error
		switch e.Op {
		case "add":
			out, id, err = AddNote(src, e.Start, e.End, e.Color, e.Body)
		case "update":
			out, err = UpdateNote(src, e.ID, e.Color, e.Body)
		case "delete":
			out, err = DeleteNote(src, e.ID)
		}
		if e.Error != "" {
			if ne, ok := err.(*NoteError); !ok || ne.Code != e.Error {
				t.Errorf("edit %d : erreur %v, attendu %s", i, err, e.Error)
			}
			continue
		}
		if err != nil || out != e.Output || (e.Op == "add" && id != e.ID) {
			t.Errorf("edit %d (%s) = %q %q %v\nattendu %q %q", i, e.Op, out, id, err, e.Output, e.ID)
		}
		// Réversibilité : l'export propre ne dépend pas des notes.
		if StripNotes(out) != StripNotes(src) {
			t.Errorf("edit %d : le texte seul a changé", i)
		}
	}
	for _, s := range c.Search {
		q, err := Compile(s.Query)
		if err != nil {
			t.Errorf("%s : %v", s.Query, err)
			continue
		}
		if got := q.Match(NotedDocument(c.Documents[s.Doc], nil, nil)); got != s.Match {
			t.Errorf("%s = %v, attendu %v", s.Query, got, s.Match)
		}
	}
}

func TestNotesRoundTripAndMaskPositions(t *testing.T) {
	src := "Été <!--n1-->très<!--/n1--> chaud 🔥.\n"
	out, id, err := AddNote(src, 28, 33, "violet", "emoji")
	if err != nil || id != "n2" {
		t.Fatalf("%v %s", err, id)
	}
	d := ParseNotes(out)
	if len(d.Notes) != 1 || d.Notes[0].ID != "n2" {
		// n1 n'a pas d'entrée dans le bloc : seul n2 est une note ; n1 reste un repère signalé.
		t.Fatalf("notes %+v", d.Notes)
	}
	r := []rune(out)
	if got := string(r[d.Notes[0].Start:d.Notes[0].End]); got != "chaud" {
		t.Errorf("passage %q", got)
	}
	ok, spans := mustCompile(t, "chaud").Find(NotedDocument(out, nil, nil))
	if !ok || string(r[spans[0].Start:spans[0].End]) != "chaud" {
		t.Errorf("find %v %v", ok, spans)
	}
}

func mustCompile(t *testing.T, q string) *Query {
	t.Helper()
	c, err := Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Textes arbitraires : jamais de panique, masque de même longueur, ajout de note sans effet sur le texte seul.
func FuzzNotes(f *testing.F) {
	f.Add("Le <!--n1-->budget<!--/n1--> ok.\n\n<!-- notes v1\n[n1] (rouge) x\n-->\n", 0, 2)
	f.Add("<!--/n1--><!--n1--><!-- notes:a v1\n[n9]\n-->", 1, 1)
	f.Add("é🔥<!--n", 0, 1)
	f.Fuzz(func(t *testing.T, s string, a, b int) {
		if !utf8.ValidString(s) {
			return
		}
		d := ParseNotes(s)
		if utf8.RuneCountInString(d.Masked) != utf8.RuneCountInString(s) {
			t.Fatalf("masque %q", s)
		}
		_ = NotesToFootnotes(s)
		clean := StripNotes(s)
		if out, _, err := AddNote(s, a, b, "", "note"); err == nil {
			if got := StripNotes(out); strings.TrimRight(got, " \t\r\n") != strings.TrimRight(clean, " \t\r\n") {
				t.Fatalf("ajout a changé le texte : %q -> %q", clean, got)
			}
		}
	})
}
