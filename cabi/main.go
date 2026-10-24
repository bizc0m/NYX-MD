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
	"sync"
	"unsafe"

	"github.com/bizc0m/NYX-MD/nyxmd"
)

// ABIVersion augmente quand une opération change ou apparaît. 2 : poignées, search.find, options.
// 3 : couche de notes (notes.*), champs notes/colors des documents (note:, couleur:).
const ABIVersion = 3

// Requêtes compilées conservées entre les appels (compilées une fois, évaluées sur de nombreux lots).
const maxHandles = 1024

var (
	handlesMu  sync.Mutex
	handles    = map[int64]*nyxmd.Query{}
	nextHandle int64
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
	Handle    int64            `json:"handle"`
	Options   nyxmd.Options    `json:"options"`
	Start     int              `json:"start"`
	End       int              `json:"end"`
	Color     string           `json:"color"`
	Body      string           `json:"body"`
	ID        string           `json:"id"`
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
			msg := "panique interne"
			if e, ok := r.(error); ok {
				msg = e.Error()
			} else if str, ok := r.(string); ok {
				msg = str
			}
			resp = fail("internal_error", msg)
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
		return map[string]any{"ok": true, "version": nyxmd.Version, "abi": ABIVersion}
	case "search.compile":
		q, errResp := compileQuery(req)
		if errResp != nil {
			return errResp
		}
		handlesMu.Lock()
		defer handlesMu.Unlock()
		if len(handles) >= maxHandles {
			return fail("too_many_handles", "trop de requêtes compilées ouvertes (search.release oublié ?)")
		}
		nextHandle++
		handles[nextHandle] = q
		return map[string]any{"ok": true, "handle": nextHandle, "terms": nonNilStrings(q.Terms())}
	case "search.release":
		handlesMu.Lock()
		delete(handles, req.Handle)
		handlesMu.Unlock()
		return map[string]any{"ok": true}
	case "search.validate", "search.match", "search.find":
		var q *nyxmd.Query
		if req.Handle != 0 {
			handlesMu.Lock()
			q = handles[req.Handle]
			handlesMu.Unlock()
			if q == nil {
				return fail("unknown_handle", "requête compilée inconnue ou déjà libérée")
			}
		} else {
			var errResp map[string]any
			if q, errResp = compileQuery(req); errResp != nil {
				return errResp
			}
		}
		resp := map[string]any{"ok": true, "terms": nonNilStrings(q.Terms())}
		switch req.Op {
		case "search.match":
			matches := make([]bool, len(req.Documents))
			for i, d := range req.Documents {
				matches[i] = q.Match(d)
			}
			resp["matches"] = matches
		case "search.find":
			type found struct {
				Match bool         `json:"match"`
				Spans []nyxmd.Span `json:"spans"`
			}
			results := make([]found, len(req.Documents))
			for i, d := range req.Documents {
				ok, spans := q.Find(d)
				if spans == nil {
					spans = []nyxmd.Span{}
				}
				results[i] = found{ok, spans}
			}
			resp["results"] = results
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
	case "notes.parse":
		d := nyxmd.ParseNotes(req.Text)
		return map[string]any{"ok": true, "notes": d.Notes, "markers": d.Markers, "block": d.Block,
			"extra": nonNilStrings(d.Extra), "diagnostics": nonNil(d.Diagnostics), "masked": d.Masked}
	case "notes.strip":
		return map[string]any{"ok": true, "output": nyxmd.StripNotes(req.Text)}
	case "notes.footnotes":
		return map[string]any{"ok": true, "output": nyxmd.NotesToFootnotes(req.Text)}
	case "notes.add":
		out, id, err := nyxmd.AddNote(req.Text, req.Start, req.End, req.Color, req.Body)
		if err != nil {
			return noteFail(err)
		}
		return map[string]any{"ok": true, "output": out, "id": id}
	case "notes.update":
		out, err := nyxmd.UpdateNote(req.Text, req.ID, req.Color, req.Body)
		if err != nil {
			return noteFail(err)
		}
		return map[string]any{"ok": true, "output": out}
	case "notes.delete":
		out, err := nyxmd.DeleteNote(req.Text, req.ID)
		if err != nil {
			return noteFail(err)
		}
		return map[string]any{"ok": true, "output": out}
	case "notes.colors":
		return map[string]any{"ok": true, "colors": nyxmd.NoteColors, "default": nyxmd.DefaultNoteColor}
	case "markdown.link":
		return map[string]any{"ok": true, "output": nyxmd.MarkdownLink(req.Label, req.URL)}
	default:
		return fail("unknown_op", "opération inconnue « "+req.Op+" »")
	}
}

func compileQuery(req request) (*nyxmd.Query, map[string]any) {
	q, err := nyxmd.CompileWith(req.Query, req.Options)
	if err != nil {
		qe := err.(*nyxmd.QueryError)
		return nil, map[string]any{"ok": false, "error": apiError{Code: qe.Code, Message: qe.Message, Position: qe.Position}}
	}
	return q, nil
}

func noteFail(err error) map[string]any {
	if ne, ok := err.(*nyxmd.NoteError); ok {
		return fail(ne.Code, ne.Message)
	}
	return fail("internal_error", err.Error())
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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
