# Poids du moteur (mesure 0.2.0, arm64)

| Partie | Poids |
|---|---|
| Runtime Go (ordonnanceur, GC, mémoire) | ~1,37 Mo |
| Métadonnées de types | ~0,40 Mo |
| encoding/json + reflect + fmt (interface C) | ~0,16 Mo |
| Code NYX-MD (`nyxmd/` + `cabi/`) | ~31 Ko |

Bibliothèque statique arm64 : 2,8 Mo ; universelle : 5,2 Mo. Effet dans une app : +2,5 à 2,9 Mo
(SimpleEdit Bold 3,1 Mo, NoteMistress 6,3 → 8,8 Mo), WASM PromptMistress 3,4 Mo.

0.3.0 : couche de notes lue sans regexp, universelle 5,41 → 5,57 Mo (+150 Ko ; avec regexp : +850 Ko).

Gains déjà obtenus en 0.2.0 : regexp et fmt retirés du moteur (bibliothèque 5,8 → 5,2 Mo ; WASM 3,9 → 3,4 Mo).

Pistes évaluées, non retenues pour l'instant :
- **JSON écrit à la main** dans l'interface C : ~10-15 % de gain, risque de bugs de décodage ; non rentable.
- **TinyGo** : non installé ; prise en charge des bibliothèques C (`c-archive`) sur macOS incertaine — à tester si le poids devient bloquant.
- **Port Swift natif** : supprimerait ~2,7 Mo dans les apps Swift, mais ferait deux moteurs à maintenir.
  Acceptable seulement si le port rejoue tout le contrat (`docs/CONTRACT.md`) dans la CI.
