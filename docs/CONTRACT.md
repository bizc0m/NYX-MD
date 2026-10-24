# Contrat de conformité NYX-MD

Les fichiers `testdata/search.json` et `testdata/variables.json` sont le **contrat** du moteur.
Champ `contract` : version du contrat (actuellement **2**). La couche de notes a son propre fichier,
`testdata/notes.json` (contrat notes **1**, NYX-MD 0.3) : lecture, masque, exports, modifications, erreurs, `note:` / `couleur:`.

- Tout adaptateur (Go natif, Swift, WASM/JS, futur port) doit rejouer **tous** les vecteurs et obtenir exactement
  les mêmes résultats : correspondances, codes et positions d'erreur, plages trouvées, rendu des variables, diagnostics, complétion.
- Positions en caractères Unicode (runes Go = scalaires Unicode Swift), base 0, plages [début, fin).
- Un vecteur existant ne change jamais de résultat sans nouvelle version majeure du contrat et une entrée dans le CHANGELOG.
- Ajouter un comportement = ajouter des vecteurs (dans la même version du contrat si rien d'existant ne change).

Rejoués aujourd'hui par : `nyxmd/nyxmd_test.go` (Go), `swift/Tests/NyxMDTests` (Swift, à travers l'interface C).
