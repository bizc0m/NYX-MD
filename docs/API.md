# API NYX-MD 0.2.0

Un moteur Go unique (`nyxmd/`), exposé par :

| Consommateur | Accès | Dépendance |
|---|---|---|
| Go | package `github.com/bizc0m/NYX-MD/nyxmd` | module Go, tag `vX.Y.Z` (dépôt public) |
| Swift | produit `NyxMD` (paquet à la racine) | Swift Package Manager, tag `X.Y.Z`/`vX.Y.Z` |
| C | `include/nyxmd.h`, bibliothèque `CNyxMD.xcframework` | artefact versionné dans `artifacts/` |

Les positions (erreurs, diagnostics, références, complétion, curseur) sont des
**décalages en caractères Unicode** (runes Go = scalaires Unicode Swift), base 0.
En Swift, `NyxText.nsRange(start:end:in:)` et `NyxText.scalarOffset(utf16:in:)` convertissent.

## Recherche booléenne (grammaire NyxBoolean)

| Syntaxe | Sens |
|---|---|
| `a b`, `a AND b` | les deux |
| `a OR b` | au moins un |
| `NOT a`, `-a` | exclusion |
| `"expression exacte"` | mots contigus, casse et ponctuation ignorées |
| `( … )` | groupement |
| `*`, `?` | joker : zéro ou plusieurs caractères / un caractère |
| `a NEAR/n b` | au plus n mots entre a et b, dans les deux sens |
| `#tag`, `@entité`, `@"Nom Prénom"` | métadonnées fournies par l'appelant (jamais déduites du texte) |

- Priorité : parenthèses, négation, NEAR, AND, OR. **Opérateurs en majuscules** (`and` est un mot).
- **Mots entiers** : `client` ne trouve pas `clientèle` (utiliser `client*`). Casse ignorée, **accents conservés** (`archive` ≠ `archivé`).
- `Compile("")` (ou blancs) : tous les documents. `Parse("")` reste une erreur (contrat NyxBoolean).
- Limites : 4096 octets, 64 niveaux d'imbrication (parenthèses, NOT, opérateurs), NEAR ≤ 4096.

- Option `fold_accents` (désactivée par défaut) : accents des lettres latines ignorés, dans la requête et les documents.
- `Find` renvoie les plages [début, fin) des termes, expressions et NEAR non niés, triées et fusionnées ; les métadonnées n'en produisent pas.

Go : `Compile(query)`, `CompileWith(query, Options)`, `(*Query).Match(Document) bool`, `(*Query).Find(Document) (bool, []Span)`,
`(*Query).Terms() []string`, `Parse(query) (*Node, error)`. Une `Query` est immuable et partageable entre goroutines.
Swift : `NyxQuery(_:options:)` (compilée une fois, `match`, `find`, `terms`, libérée à la destruction),
`NyxSearch.validate(_:options:)`, `NyxSearch.match(_:documents:options:)` (raccourci).

### Erreurs de requête (`QueryError` / `NyxError`)

| Code | Position |
|---|---|
| `query_too_long` | 0 |
| `query_too_deep` | élément qui dépasse la limite |
| `unterminated_quote` | début de l'élément entre guillemets |
| `empty_operand` | `#`, `@` ou `""` vides |
| `expected_operand` | élément fautif ou fin de requête |
| `unexpected_token` | parenthèse fermante en trop, élément en trop |
| `missing_closing_parenthesis` | parenthèse ouvrante non fermée |
| `invalid_proximity` | `NEAR/x` invalide |
| `near_operand` | NEAR appliqué à un groupe ou une métadonnée |

## Variables

| Syntaxe | Noms |
|---|---|
| `{{nom}}` | `texte`, `source`, `url`, `titre`, `date` (AAAA-MM-JJ), `heure` (HH:MM:SS), `tags`, `commentaire` |
| `$nom` natives | `date`, `day` (jour en toutes lettres selon la locale), `time` (HH:MM), `datetime`, `month` (AAAA-MM), `year` |
| `$nom` personnelles | nom `^[A-Za-z][A-Za-z0-9_]*$`, défini par l'utilisateur |

Règles :
- **Une seule passe** : une valeur insérée (texte capturé, variable personnelle) n'est jamais redéveloppée.
- **Frontière stricte** : `$client` ne modifie ni `$clientèle`, ni `$client2`, ni `$client_x`. `$nom(…)` et `$nom[…]` sont laissés intacts (syntaxes spéciales).
- **Natives prioritaires** : une variable personnelle nommée comme une native ou un nom réservé (`section`, `llm`, `source`, `url`, `file`) est rejetée (`reserved_name`).
- **Inconnues conservées** : `{{inconnu}}` et `$absent` restent tels quels avec un avertissement `unknown_variable`. `$LLM`, `$section(…)` restent tels quels sans avertissement.
- `{{source}}` = lien Markdown `[titre](<url>)` échappé (`MarkdownLink`), vide sans URL.
- Une valeur `{{…}}` sur plusieurs lignes reprend l'indentation de sa ligne (`indent_multiline`).
- Le contexte fournit date (`timestamp` + `utc_offset_seconds`), locale et variables : le moteur ne lit ni horloge système dans le rendu, ni presse-papiers, ni préférences, ni fichiers.

Go : `Render`, `Validate`, `References`, `Complete`, `ParsePersonal`, `CheckPersonal`, `MarkdownLink`.
Swift : `NyxVariables.render/validate/references/complete/parsePersonal/markdownLink`.

### Diagnostics

| Code | Gravité |
|---|---|
| `unknown_variable` | warning |
| `invalid_name`, `reserved_name`, `duplicate_name`, `empty_value`, `malformed_entry` | error |

## Interface C (`nyxmd_call`)

Requête JSON → réponse JSON `{"ok": true, …}` ou `{"ok": false, "error": {"code", "message", "position"}}`.
Libérer chaque réponse avec `nyxmd_free`. Version d'interface (`abi`) : **2**. Opérations : `version`,
`search.compile` (→ `handle`), `search.release`, `search.validate`, `search.match`, `search.find`
(ces trois dernières acceptent `handle` ou `query` + `options`), `variables.render`, `variables.validate`, `variables.references`, `variables.complete`,
`variables.parse_personal`, `markdown.link`. Erreurs d'appel : `bad_request`, `unknown_op`, `unknown_handle`, `too_many_handles`, `internal_error`
(Swift ajoute `abi_mismatch` si la bibliothèque ne correspond pas à l'adaptateur)
(une panique Go est convertie en erreur, jamais propagée).
