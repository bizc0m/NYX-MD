package nyxmd

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// Variables NYX-MD, compatibles NoteMistress :
//   - modèles {{texte}} {{source}} {{url}} {{titre}} {{date}} {{heure}} {{tags}} {{commentaire}}
//   - natives $date $day $time $datetime $month $year
//   - personnelles $nom (définies par l'utilisateur, jamais prioritaires sur les natives)
//
// Rendu en une seule passe : une valeur insérée n'est jamais réinterprétée.
// Le cœur ne lit ni presse-papiers, ni préférences, ni fichiers : tout vient du Context.

// TemplateNames liste les variables {{…}} dans l'ordre d'affichage.
var TemplateNames = []string{"texte", "source", "url", "titre", "date", "heure", "tags", "commentaire"}

// NativeNames liste les variables $… natives dans l'ordre d'affichage.
var NativeNames = []string{"date", "day", "time", "datetime", "month", "year"}

// ReservedNames ne peuvent pas être des variables personnelles (natives + syntaxes
// spéciales NoteMistress comme $section(…) ou $LLM). Comparaison insensible à la casse.
var ReservedNames = []string{"date", "day", "time", "datetime", "month", "year", "section", "llm", "source", "url", "file"}

// Variable est une variable personnelle.
type Variable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Context fournit toutes les valeurs ; aucune n'est lue ailleurs.
type Context struct {
	Text      string     `json:"text,omitempty"`
	URL       string     `json:"url,omitempty"`
	Title     string     `json:"title,omitempty"`
	Tags      string     `json:"tags,omitempty"`
	Comment   string     `json:"comment,omitempty"`
	Timestamp int64      `json:"timestamp"`          // secondes Unix
	UTCOffset int        `json:"utc_offset_seconds"` // fuseau de l'appelant
	Locale    string     `json:"locale,omitempty"`   // "fr_FR" (défaut) ou "en_US"
	Personal  []Variable `json:"personal,omitempty"`
}

// Syntaxes activées pour un rendu ou une validation.
type Syntax struct {
	Braces          bool `json:"braces"`           // {{nom}}
	Dollar          bool `json:"dollar"`           // $nom
	IndentMultiline bool `json:"indent_multiline"` // une valeur {{…}} sur plusieurs lignes reprend l'indentation de sa ligne
}

// AllSyntaxes active {{…}} et $… avec indentation des valeurs multilignes.
var AllSyntaxes = Syntax{Braces: true, Dollar: true, IndentMultiline: true}

// Diagnostic signale un problème ; Start/End sont des décalages en runes (-1 si sans objet).
type Diagnostic struct {
	Severity string `json:"severity"` // "error" ou "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
	Name     string `json:"name,omitempty"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
}

// Codes de diagnostic stables.
const (
	DiagUnknownVariable = "unknown_variable"
	DiagInvalidName     = "invalid_name"
	DiagReservedName    = "reserved_name"
	DiagDuplicateName   = "duplicate_name"
	DiagEmptyValue      = "empty_value"
	DiagMalformedEntry  = "malformed_entry"
)

// Reference est une occurrence de variable reconnue dans un texte.
type Reference struct {
	Syntax string `json:"syntax"` // "braces" ou "dollar"
	Name   string `json:"name"`
	Kind   string `json:"kind"` // template, native, personal, reserved, unknown
	Start  int    `json:"start"`
	End    int    `json:"end"`
}

func isReserved(name string) bool {
	l := strings.ToLower(name)
	for _, r := range ReservedNames {
		if r == l {
			return true
		}
	}
	return false
}

func isNativeIgnoringCase(name string) bool {
	for _, n := range NativeNames {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

func asciiLetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func nameRune(r rune) bool    { return asciiLetter(r) || (r >= '0' && r <= '9') || r == '_' }

// ValidName : lettre ASCII puis lettres, chiffres ou _ (règle NoteMistress).
func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !asciiLetter(r) {
			return false
		}
		if !nameRune(r) {
			return false
		}
	}
	return true
}

// CheckPersonal valide une liste : noms invalides, réservés, doublons (casse ignorée)
// et valeurs vides sont rejetés avec un diagnostic.
func CheckPersonal(vars []Variable) ([]Variable, []Diagnostic) {
	var ok []Variable
	var diags []Diagnostic
	seen := map[string]bool{}
	for _, v := range vars {
		d := Diagnostic{Severity: "error", Name: v.Name, Start: -1, End: -1}
		switch {
		case !ValidName(v.Name):
			d.Code, d.Message = DiagInvalidName, "nom invalide « "+v.Name+" » (lettre puis lettres, chiffres ou _)"
		case isReserved(v.Name):
			d.Code, d.Message = DiagReservedName, "« "+v.Name+" » est réservé (variable native ou syntaxe spéciale)"
		case seen[strings.ToLower(v.Name)]:
			d.Code, d.Message = DiagDuplicateName, "« "+v.Name+" » est défini plusieurs fois"
		case strings.TrimSpace(v.Value) == "":
			d.Code, d.Message = DiagEmptyValue, "« "+v.Name+" » n'a pas de valeur"
		default:
			seen[strings.ToLower(v.Name)] = true
			ok = append(ok, v)
			continue
		}
		diags = append(diags, d)
	}
	return ok, diags
}

// ParsePersonal lit le format NoteMistress « nom=valeur, nom2=valeur2 » (virgules ou
// retours à la ligne). Les entrées invalides sont signalées, jamais devinées.
func ParsePersonal(text string) ([]Variable, []Diagnostic) {
	var vars []Variable
	var diags []Diagnostic
	chunks := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' })
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		name, value, found := strings.Cut(c, "=")
		if !found {
			diags = append(diags, Diagnostic{Severity: "error", Code: DiagMalformedEntry,
				Message: "entrée sans « = » : « " + c + " »", Start: -1, End: -1})
			continue
		}
		vars = append(vars, Variable{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	ok, more := CheckPersonal(vars)
	return ok, append(diags, more...)
}

// values calcule les valeurs disponibles pour un contexte.
type valueSet struct {
	template map[string]string
	native   map[string]string
	personal map[string]string
}

var frDays = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}

func buildValues(ctx Context) (valueSet, []Diagnostic) {
	t := time.Unix(ctx.Timestamp, 0).In(time.FixedZone("ctx", ctx.UTCOffset))
	day := frDays[int(t.Weekday())]
	if strings.HasPrefix(strings.ToLower(ctx.Locale), "en") {
		day = t.Weekday().String()
	}
	source := ""
	if ctx.URL != "" {
		source = MarkdownLink(ctx.Title, ctx.URL)
	}
	vs := valueSet{
		template: map[string]string{
			"texte": ctx.Text, "source": source, "url": ctx.URL, "titre": ctx.Title,
			"date": t.Format("2006-01-02"), "heure": t.Format("15:04:05"),
			"tags": ctx.Tags, "commentaire": ctx.Comment,
		},
		native: map[string]string{
			"date": t.Format("2006-01-02"), "day": day, "time": t.Format("15:04"),
			"datetime": t.Format("2006-01-02 15:04"), "month": t.Format("2006-01"), "year": t.Format("2006"),
		},
		personal: map[string]string{},
	}
	ok, diags := CheckPersonal(ctx.Personal)
	for _, v := range ok {
		vs.personal[v.Name] = v.Value
	}
	return vs, diags
}

// scan parcourt le texte une seule fois et appelle emit pour chaque littéral ou référence.
func scan(r []rune, syn Syntax, vs valueSet, emit func(lit []rune, ref *Reference)) {
	litStart := 0
	flush := func(end int) {
		if end > litStart {
			emit(r[litStart:end], nil)
		}
	}
	for i := 0; i < len(r); {
		if syn.Braces && r[i] == '{' && i+1 < len(r) && r[i+1] == '{' {
			// {{nom}} : nom sans accolades, sur une seule ligne.
			j := i + 2
			for j < len(r) && r[j] != '{' && r[j] != '}' && r[j] != '\n' {
				j++
			}
			if j > i+2 && j+1 < len(r) && r[j] == '}' && r[j+1] == '}' {
				name := string(r[i+2 : j])
				kind := "unknown"
				if _, ok := vs.template[name]; ok {
					kind = "template"
				}
				flush(i)
				emit(nil, &Reference{Syntax: "braces", Name: name, Kind: kind, Start: i, End: j + 2})
				i = j + 2
				litStart = i
				continue
			}
		}
		if syn.Dollar && r[i] == '$' && i+1 < len(r) && asciiLetter(r[i+1]) {
			j := i + 1
			for j < len(r) && nameRune(r[j]) {
				j++
			}
			// Frontière stricte : $client ne touche ni $clientèle ni $client2 ;
			// $nom( et $nom[ sont des syntaxes spéciales laissées intactes.
			boundary := j >= len(r) || !(unicode.IsLetter(r[j]) || unicode.IsNumber(r[j]) || r[j] == '_' || r[j] == '(' || r[j] == '[')
			if boundary {
				name := string(r[i+1 : j])
				kind := "unknown"
				if _, ok := vs.native[name]; ok {
					kind = "native"
				} else if _, ok := vs.personal[name]; ok {
					kind = "personal"
				} else if isReserved(name) && !isNativeIgnoringCase(name) {
					kind = "reserved" // $section(…), $LLM… : syntaxes spéciales des applications
				}
				flush(i)
				emit(nil, &Reference{Syntax: "dollar", Name: name, Kind: kind, Start: i, End: j})
				i = j
				litStart = i
				continue
			}
			i = j
			continue
		}
		i++
	}
	flush(len(r))
}

func unknownDiag(ref Reference) Diagnostic {
	label := "$" + ref.Name
	if ref.Syntax == "braces" {
		label = "{{" + ref.Name + "}}"
	}
	return Diagnostic{Severity: "warning", Code: DiagUnknownVariable, Name: ref.Name,
		Message: "variable inconnue " + label + " (conservée telle quelle)", Start: ref.Start, End: ref.End}
}

// References liste les variables trouvées, connues ou non (pour colorer ou valider).
func References(text string, ctx Context, syn Syntax) []Reference {
	vs, _ := buildValues(ctx)
	var refs []Reference
	scan([]rune(text), syn, vs, func(_ []rune, ref *Reference) {
		if ref != nil {
			refs = append(refs, *ref)
		}
	})
	return refs
}

// Validate renvoie les diagnostics d'un texte : variables inconnues et variables
// personnelles invalides du contexte.
func Validate(text string, ctx Context, syn Syntax) []Diagnostic {
	_, diags := buildValues(ctx)
	for _, ref := range References(text, ctx, syn) {
		if ref.Kind == "unknown" {
			diags = append(diags, unknownDiag(ref))
		}
	}
	return diags
}

// Render développe les variables en une seule passe. Les variables inconnues et
// réservées restent littérales ; les inconnues produisent un diagnostic.
func Render(text string, ctx Context, syn Syntax) (string, []Diagnostic) {
	vs, diags := buildValues(ctx)
	r := []rune(text)
	var b strings.Builder
	lineIndent := func(pos int) string {
		start := pos
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		end := start
		for end < len(r) && (r[end] == ' ' || r[end] == '\t') {
			end++
		}
		return string(r[start:end])
	}
	scan(r, syn, vs, func(lit []rune, ref *Reference) {
		if ref == nil {
			b.WriteString(string(lit))
			return
		}
		var value string
		switch ref.Kind {
		case "template":
			value = vs.template[ref.Name]
			if syn.IndentMultiline && strings.Contains(value, "\n") {
				value = strings.ReplaceAll(value, "\n", "\n"+lineIndent(ref.Start))
			}
		case "native":
			value = vs.native[ref.Name]
		case "personal":
			value = vs.personal[ref.Name]
		default:
			if ref.Kind == "unknown" {
				diags = append(diags, unknownDiag(*ref))
			}
			value = string(r[ref.Start:ref.End])
		}
		b.WriteString(value)
	})
	return b.String(), diags
}

// CompletionItem est une proposition de complétion.
type CompletionItem struct {
	Label  string `json:"label"`  // affiché, ex. "$date"
	Insert string `json:"insert"` // texte qui remplace [Start, End)
	Kind   string `json:"kind"`   // template, native, personal
	Detail string `json:"detail"` // valeur actuelle
}

// Completion décrit la zone à remplacer (runes) et les propositions.
type Completion struct {
	Start int              `json:"start"`
	End   int              `json:"end"`
	Items []CompletionItem `json:"items"`
}

// Complete propose des variables quand le curseur suit « {{lettres » ou « $lettres ».
// Renvoie nil hors de ces contextes. caret est un décalage en runes.
func Complete(text string, caret int, ctx Context, syn Syntax) *Completion {
	r := []rune(text)
	if caret < 0 || caret > len(r) {
		return nil
	}
	vs, _ := buildValues(ctx)
	// {{partiel : lettres uniquement, comme NoteMistress.
	if syn.Braces {
		i := caret
		for i > 0 && unicode.IsLetter(r[i-1]) {
			i--
		}
		if i >= 2 && r[i-1] == '{' && r[i-2] == '{' {
			partial := strings.ToLower(string(r[i:caret]))
			end := caret
			if caret+1 < len(r) && r[caret] == '}' && r[caret+1] == '}' {
				end = caret + 2 // le remplacement absorbe les accolades déjà présentes
			}
			c := &Completion{Start: i - 2, End: end}
			for _, n := range TemplateNames {
				if strings.HasPrefix(n, partial) {
					c.Items = append(c.Items, CompletionItem{Label: "{{" + n + "}}", Insert: "{{" + n + "}}", Kind: "template", Detail: vs.template[n]})
				}
			}
			return c
		}
	}
	if syn.Dollar {
		i := caret
		for i > 0 && nameRune(r[i-1]) {
			i--
		}
		if i >= 1 && r[i-1] == '$' && (i == caret || asciiLetter(r[i])) {
			partial := strings.ToLower(string(r[i:caret]))
			c := &Completion{Start: i - 1, End: caret}
			for _, n := range NativeNames {
				if strings.HasPrefix(n, partial) {
					c.Items = append(c.Items, CompletionItem{Label: "$" + n, Insert: "$" + n, Kind: "native", Detail: vs.native[n]})
				}
			}
			names := make([]string, 0, len(vs.personal))
			for n := range vs.personal {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				if strings.HasPrefix(strings.ToLower(n), partial) {
					c.Items = append(c.Items, CompletionItem{Label: "$" + n, Insert: "$" + n, Kind: "personal", Detail: vs.personal[n]})
				}
			}
			return c
		}
	}
	return nil
}

// MarkdownLink construit [libellé](<url>) en échappant le libellé et les chevrons de
// l'URL (règle NoteMistress). Libellé vide : « Source ».
func MarkdownLink(label, url string) string {
	if label == "" {
		label = "Source"
	}
	label = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "\n", " ").Replace(label)
	url = strings.NewReplacer("<", "%3C", ">", "%3E").Replace(url)
	return "[" + label + "](<" + url + ">)"
}
