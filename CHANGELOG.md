# Changelog

Format SemVer. Toute modification du comportement est listée ici avant publication.

## 0.3.0 — 2026-10-24

Interface C : version 3 (ajouts uniquement ; les opérations 0.2.0 sont inchangées, mais l'adaptateur Swift exige l'interface 3).

- **Couche de notes** (`nyxmd/notes.go`) : passages annotés `<!--n1-->…<!--/n1-->` (ou repère seul = note ponctuelle),
  bloc `<!-- notes v1 … -->` en fin de fichier (`notes:<app> v1` accepté en lecture), lignes `[n1] (couleur) texte` échappées
  (`\n`, `\\`, `--\>`). 8 couleurs : jaune (défaut), vert, bleu, rose, rouge, violet, orange, gris.
  - Lecture : `ParseNotes` (notes, repères, bloc, diagnostics, texte masqué). Notes orphelines et lignes illisibles conservées.
  - Exports : `StripNotes` (texte seul), `NotesToFootnotes` (notes de bas de page Markdown standard `[^k]`).
  - Modifications : `AddNote`, `UpdateNote`, `DeleteNote` (erreurs `out_of_range`, `inside_marker`, `unknown_color`, `unknown_note`).
  - Interface C : `notes.parse`, `notes.strip`, `notes.footnotes`, `notes.add`, `notes.update`, `notes.delete`, `notes.colors`. Swift : `NyxNotes`.
- **Recherche** : champs `note:mot`, `note:"expression"` (jokers permis) et `couleur:nom` (alias `color:`) ; `Document.Notes` / `Document.Colors` ;
  `NotedDocument` / `NyxNotes.searchDocument` masquent repères et bloc (positions et lignes inchangées) pour que `n1` ou `notes` ne soient pas pris pour des mots.
  Règles NyxBoolean inchangées : les vecteurs de recherche du contrat 2 passent à l'identique.
- **Contrat** : `testdata/notes.json` (contrat notes 1), rejoué en Go et en Swift ; fuzzing `FuzzNotes`.
- **Poids** : lecture des notes écrite sans regexp ; bibliothèque universelle 5,41 → 5,57 Mo.

## 0.2.0 — 2026-10-24

Interface C : version 2 (ajouts uniquement, les opérations 0.1.0 restent compatibles).

- **Requêtes compilées** : `search.compile` / `search.release` (poignées, 1024 max) ; Swift `NyxQuery`, Go `CompileWith`.
- **Positions trouvées** : `Query.Find` / `search.find` / `NyxQuery.find` renvoient les plages des termes, expressions et NEAR non niés.
- **Accents ignorés en option** (`fold_accents`, désactivé par défaut) : lettres latines (Latin-1 et Latin étendu A, œ, æ, ß).
- **Performance** : découpage en mots sans allocation par mot, comparaison directe des mots sans joker, jokers sans regexp,
  arrêt au premier résultat. Mesure (texte de 4 Ko) : 43 → 20,6 µs par document (×2,1), 21 → 4 allocations.
- **Poids** : bibliothèque 5,8 → 5,2 Mo (regexp et fmt retirés du moteur).
- **Contrôle d'interface** : l'adaptateur Swift vérifie au premier appel que la bibliothèque est en interface 2 (`abi_mismatch` sinon).
- **Provenance** : la bibliothèque est construite par l'action GitHub « Artefact » (attestation signée) ; la CI refuse sur main un artefact non construit par la CI ; la release joint l'archive et son SHA-256.
- **Contrat de conformité v2** (`testdata/`, champ `contract`) : vecteurs de positions et d'accents, rejoués en Go et en Swift ; fuzzing de Find.
- Licence MIT.

Règles de recherche inchangées : les 116 cas 0.1.0 passent à l'identique.

## 0.1.0 — 2026-10-24

Première version du moteur.

- Recherche booléenne : grammaire NyxBoolean reprise de Nyx (révision 4473c1e, voir ORIGIN.md) sans changement de règles.
  Ajouts : erreurs structurées avec code et position, limite de 64 niveaux, motifs précompilés, `Compile("")` = tous les documents, `Terms()`.
- Variables `{{…}}`, `$natives`, `$personnelles` compatibles NoteMistress, rendu en une passe, frontières strictes, diagnostics.
- Interface C JSON (`nyxmd_call`), bibliothèque universelle arm64 + x86_64 (macOS 13+), paquet Swift `NyxMD`.
- Vecteurs de conformité communs Go/Swift (`testdata/`), fuzzing, CI.

### Différences avec NoteMistress (migration)
- `$date` n'est plus remplacé à l'intérieur de `$dates`, et `$client` plus à l'intérieur de `$clientèle` : ces textes restent littéraux avec un avertissement.
- Une variable personnelle dont la valeur contient `$autre` n'est plus développée une seconde fois.
