package nyxmd

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
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
