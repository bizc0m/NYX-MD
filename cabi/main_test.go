package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func call(t *testing.T, req string) map[string]any {
	t.Helper()
	b, err := json.Marshal(handle([]byte(req)))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

func TestABI(t *testing.T) {
	if r := call(t, `{"op":"version"}`); r["version"] != "0.3.0" || r["abi"].(float64) != 3 {
		t.Fatalf("version %v", r)
	}
	r := call(t, `{"op":"search.match","query":"(client OR fournisseur) AND NOT archivé","documents":[{"text":"client"},{"text":"fournisseur archivé"}]}`)
	if m := r["matches"].([]any); m[0] != true || m[1] != false {
		t.Fatalf("match %v", r)
	}
	r = call(t, `{"op":"search.validate","query":"(a"}`)
	if e := r["error"].(map[string]any); r["ok"] != false || e["code"] != "missing_closing_parenthesis" || e["position"].(float64) != 0 {
		t.Fatalf("error %v", r)
	}
	r = call(t, `{"op":"variables.render","text":"$x {{y}}","context":{"personal":[{"name":"x","value":"X"}]}}`)
	if r["output"] != "X {{y}}" || len(r["diagnostics"].([]any)) != 1 {
		t.Fatalf("render %v", r)
	}
	if r = call(t, `{"op":"variables.complete","text":"abc","caret":2}`); r["completion"] != nil {
		t.Fatalf("complete %v", r)
	}
	if r = call(t, `pas du json`); r["ok"] != false {
		t.Fatalf("bad json %v", r)
	}
	if r = call(t, `{"op":"rm -rf"}`); r["error"].(map[string]any)["code"] != "unknown_op" {
		t.Fatalf("unknown op %v", r)
	}
}

func TestHandles(t *testing.T) {
	r := call(t, `{"op":"search.compile","query":"archive","options":{"fold_accents":true}}`)
	h := r["handle"].(float64)
	if r["ok"] != true || h <= 0 {
		t.Fatalf("compile %v", r)
	}
	r = call(t, fmt.Sprintf(`{"op":"search.find","handle":%v,"documents":[{"text":"x archivé"},{"text":"rien"}]}`, h))
	res := r["results"].([]any)
	first := res[0].(map[string]any)
	if first["match"] != true || res[1].(map[string]any)["match"] != false {
		t.Fatalf("find %v", r)
	}
	if sp := first["spans"].([]any)[0].(map[string]any); sp["start"].(float64) != 2 || sp["end"].(float64) != 9 {
		t.Fatalf("spans %v", first)
	}
	call(t, fmt.Sprintf(`{"op":"search.release","handle":%v}`, h))
	if r = call(t, fmt.Sprintf(`{"op":"search.match","handle":%v,"documents":[]}`, h)); r["error"].(map[string]any)["code"] != "unknown_handle" {
		t.Fatalf("released %v", r)
	}
	if r = call(t, `{"op":"version"}`); r["abi"].(float64) != 3 {
		t.Fatalf("abi %v", r)
	}
}

func TestHandleLimit(t *testing.T) {
	var opened []float64
	defer func() {
		for _, h := range opened {
			call(t, fmt.Sprintf(`{"op":"search.release","handle":%v}`, h))
		}
	}()
	for i := 0; i < maxHandles+1; i++ {
		r := call(t, `{"op":"search.compile","query":"a"}`)
		if r["ok"] != true {
			if r["error"].(map[string]any)["code"] != "too_many_handles" || i != maxHandles {
				t.Fatalf("limit at %d: %v", i, r)
			}
			return
		}
		opened = append(opened, r["handle"].(float64))
	}
	t.Fatal("no limit")
}

func TestNotesABI(t *testing.T) {
	r := call(t, `{"op":"notes.add","text":"Café crème.","start":0,"end":4,"color":"rose","body":"chaud"}`)
	out, _ := r["output"].(string)
	if r["ok"] != true || r["id"] != "n1" || out != "<!--n1-->Café<!--/n1--> crème.\n\n<!-- notes v1\n[n1] (rose) chaud\n-->\n" {
		t.Fatalf("add %v", r)
	}
	b, _ := json.Marshal(map[string]any{"op": "notes.parse", "text": out})
	r = call(t, string(b))
	notes := r["notes"].([]any)
	if len(notes) != 1 || notes[0].(map[string]any)["start"].(float64) != 9 || r["masked"] == nil {
		t.Fatalf("parse %v", r)
	}
	b, _ = json.Marshal(map[string]any{"op": "notes.footnotes", "text": out})
	if r = call(t, string(b)); r["output"] != "Café[^1] crème.\n\n[^1]: chaud\n" {
		t.Fatalf("footnotes %v", r)
	}
	if r = call(t, `{"op":"notes.add","text":"abc","start":0,"end":1,"color":"fuchsia"}`); r["error"].(map[string]any)["code"] != "unknown_color" {
		t.Fatalf("color %v", r)
	}
	r = call(t, `{"op":"search.match","query":"note:chaud AND couleur:rose","documents":[{"text":"x","notes":["chaud"],"colors":["rose"]},{"text":"chaud"}]}`)
	if m := r["matches"].([]any); m[0] != true || m[1] != false {
		t.Fatalf("search %v", r)
	}
	if r = call(t, `{"op":"notes.colors"}`); len(r["colors"].([]any)) != 8 || r["default"] != "jaune" {
		t.Fatalf("colors %v", r)
	}
}
