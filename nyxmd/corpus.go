package nyxmd

// Corpus (NYX-MD 0.4) : documents gardés dans le moteur entre deux recherches.
//
// Avant 0.4, chaque recherche renvoyait tous les textes au moteur (JSON) et les redécoupait en mots.
// Un Corpus garde les documents par identifiant (chemin de fichier, par exemple) ; l'appelant ne transmet
// que ce qui change (Put / Remove). Les mots sont découpés une seule fois par document et par réglage
// d'accents, puis réutilisés. La recherche est répartie sur les cœurs ; l'ordre des résultats est
// celui des identifiants (déterministe). Un Corpus est sûr entre goroutines.

import (
	"runtime"
	"sort"
	"sync"
)

// CorpusHit est un document trouvé : identifiant et plages (runes) des termes trouvés.
type CorpusHit struct {
	ID    string `json:"id"`
	Spans []Span `json:"spans"`
}

type corpusEntry struct {
	doc   Document
	words [2][]word // [accents conservés, accents ignorés], découpés à la première recherche
}

type Corpus struct {
	mu   sync.RWMutex
	docs map[string]*corpusEntry
	ids  []string // trié, recalculé après modification
	dirt bool
	// CacheWords : garde les mots découpés entre deux recherches. Plus rapide (≈ ×30) mais coûte environ
	// cinq fois la taille des textes par réglage d'accents. Désactivé par défaut : découpage réparti sur les cœurs.
	CacheWords bool
}

func NewCorpus() *Corpus { return &Corpus{docs: map[string]*corpusEntry{}} }

// Put ajoute ou remplace des documents (les mots en cache du document remplacé sont oubliés).
func (c *Corpus) Put(docs map[string]Document) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, d := range docs {
		if _, ok := c.docs[id]; !ok {
			c.dirt = true
		}
		c.docs[id] = &corpusEntry{doc: d}
	}
}

// Remove retire des documents (identifiants inconnus ignorés).
func (c *Corpus) Remove(ids []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		if _, ok := c.docs[id]; ok {
			delete(c.docs, id)
			c.dirt = true
		}
	}
}

// Len : nombre de documents.
func (c *Corpus) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.docs)
}

// IDs : identifiants triés.
func (c *Corpus) IDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sortIDs()
	return append([]string(nil), c.ids...)
}

func (c *Corpus) sortIDs() {
	if !c.dirt && len(c.ids) == len(c.docs) {
		return
	}
	c.ids = c.ids[:0]
	for id := range c.docs {
		c.ids = append(c.ids, id)
	}
	sort.Strings(c.ids)
	c.dirt = false
}

// Find évalue la requête sur tous les documents. withSpans : plages des termes trouvés (sinon nil).
func (c *Corpus) Find(q *Query, withSpans bool) []CorpusHit {
	if q == nil {
		return nil
	}
	// Verrou exclusif : les mots découpés sont rangés dans les entrées pendant la recherche.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sortIDs()
	n := len(c.ids)
	results := make([]*CorpusHit, n)
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if n < 64 {
		workers = 1
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo, hi := w*chunk, (w+1)*chunk
		if hi > n {
			hi = n
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				e := c.docs[c.ids[i]]
				var src *wordSource
				if c.CacheWords {
					src = newWordSource(e.doc.Text, &e.words)
				} else {
					src = newWordSource(e.doc.Text, nil)
				}
				var ok bool
				var spans []Span
				if withSpans {
					ok, spans = q.find(e.doc, src)
				} else {
					ok = q.root == nil || q.root.match(e.doc, src)
				}
				if ok {
					if spans == nil {
						spans = []Span{}
					}
					results[i] = &CorpusHit{ID: c.ids[i], Spans: spans}
				}
			}
		}(lo, hi)
	}
	wg.Wait()
	out := make([]CorpusHit, 0)
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}
