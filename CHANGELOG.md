# Changelog

Format SemVer. Toute modification du comportement est listée ici avant publication.

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
