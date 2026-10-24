package main

import (
	"encoding/json"
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
	if r := call(t, `{"op":"version"}`); r["version"] != "0.1.0" {
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
