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
