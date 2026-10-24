// Commande cabi : interface C du moteur NYX-MD, compilée en bibliothèque statique
// (go build -buildmode=c-archive) pour les applications Swift et C.
//
// Un seul point d'entrée : nyxmd_call reçoit une requête JSON UTF-8 et renvoie une
// réponse JSON à libérer avec nyxmd_free. Aucune panique ne traverse la frontière C.
// Les positions sont des décalages en caractères Unicode (runes).
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/bizc0m/NYX-MD/nyxmd"
)

type request struct {
	Op        string           `json:"op"`
	Query     string           `json:"query"`
	Documents []nyxmd.Document `json:"documents"`
	Text      string           `json:"text"`
	Caret     int              `json:"caret"`
	Context   nyxmd.Context    `json:"context"`
	Syntax    *nyxmd.Syntax    `json:"syntax"`
	Label     string           `json:"label"`
	URL       string           `json:"url"`
}

type apiError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position int    `json:"position"`
}

func fail(code, msg string) map[string]any {
	return map[string]any{"ok": false, "error": apiError{Code: code, Message: msg, Position: -1}}
}

func handle(raw []byte) (resp map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			resp = fail("internal_error", fmt.Sprint(r))
		}
	}()
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return fail("bad_request", "JSON invalide : "+err.Error())
	}
	syn := nyxmd.AllSyntaxes
	if req.Syntax != nil {
		syn = *req.Syntax
	}
	switch req.Op {
	case "version":
		return map[string]any{"ok": true, "version": nyxmd.Version, "abi": 1}
	case "search.validate", "search.match":
		q, err := nyxmd.Compile(req.Query)
		if err != nil {
			qe := err.(*nyxmd.QueryError)
			return map[string]any{"ok": false, "error": apiError{Code: qe.Code, Message: qe.Message, Position: qe.Position}}
		}
		resp := map[string]any{"ok": true, "terms": q.Terms()}
		if req.Op == "search.match" {
			matches := make([]bool, len(req.Documents))
			for i, d := range req.Documents {
				matches[i] = q.Match(d)
			}
			resp["matches"] = matches
		}
		return resp
	case "variables.render":
		out, diags := nyxmd.Render(req.Text, req.Context, syn)
		return map[string]any{"ok": true, "output": out, "diagnostics": nonNil(diags)}
	case "variables.validate":
		return map[string]any{"ok": true, "diagnostics": nonNil(nyxmd.Validate(req.Text, req.Context, syn))}
	case "variables.references":
		refs := nyxmd.References(req.Text, req.Context, syn)
		if refs == nil {
			refs = []nyxmd.Reference{}
		}
		return map[string]any{"ok": true, "references": refs}
	case "variables.complete":
		return map[string]any{"ok": true, "completion": nyxmd.Complete(req.Text, req.Caret, req.Context, syn)}
	case "variables.parse_personal":
		vars, diags := nyxmd.ParsePersonal(req.Text)
		if vars == nil {
			vars = []nyxmd.Variable{}
		}
		return map[string]any{"ok": true, "variables": vars, "diagnostics": nonNil(diags)}
	case "markdown.link":
		return map[string]any{"ok": true, "output": nyxmd.MarkdownLink(req.Label, req.URL)}
	default:
		return fail("unknown_op", "opération inconnue « "+req.Op+" »")
	}
}

func nonNil(d []nyxmd.Diagnostic) []nyxmd.Diagnostic {
	if d == nil {
		return []nyxmd.Diagnostic{}
	}
	return d
}

//export nyxmd_call
func nyxmd_call(request *C.char) *C.char {
	if request == nil {
		b, _ := json.Marshal(fail("bad_request", "requête nulle"))
		return C.CString(string(b))
	}
	b, err := json.Marshal(handle([]byte(C.GoString(request))))
	if err != nil {
		b, _ = json.Marshal(fail("internal_error", err.Error()))
	}
	return C.CString(string(b))
}

//export nyxmd_free
func nyxmd_free(p *C.char) { C.free(unsafe.Pointer(p)) }

func main() {}
