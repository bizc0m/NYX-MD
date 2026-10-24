# NYX-MD

Module partagé de variables et de recherche booléenne pour les éditeurs de texte et les applications de capture.

## État réel (0.2.0)

- Moteur Go implémenté et testé : recherche booléenne (grammaire NyxBoolean) et variables (`nyxmd/`).
- Adaptateurs : interface C (`cabi/`, `include/nyxmd.h`), paquet Swift `NyxMD` (bibliothèque universelle macOS 13+ dans `artifacts/`).
- Consommateur intégré et testé : SimpleEdit Bold v1.6 (dépôt MDMistress, commit 0cdb5d8), NyxMD exact 0.1.0. NoteMistress, Nyx et PromptMistress ne sont **pas** encore migrés.
- Documentation : [docs/API.md](docs/API.md), [docs/CONTRACT.md](docs/CONTRACT.md), [CHANGELOG.md](CHANGELOG.md), [ORIGIN.md](ORIGIN.md). Licence MIT.

## Développement

```sh
go test ./...                      # moteur + interface C
./scripts/build-xcframework.sh     # essai local seulement : ne pas commiter cet artefact
swift test                         # adaptateur Swift sur les mêmes vecteurs
```

Publier une version :
1. Sur une branche : modifier le code, `nyxmd/version.go` et `CHANGELOG.md`, pousser.
2. L'action **Artefact** reconstruit `artifacts/CNyxMD.xcframework` sur GitHub, l'atteste et le commite sur la branche.
3. Fusionner dans `main` (la CI refuse un artefact non construit par la CI), taguer `vX.Y.Z` et pousser le tag.
4. L'action **Release** crée la release avec les notes, l'archive et `SHA256SUMS.txt`.

## Intérêt pour un éditeur de texte

- Réutiliser les mêmes variables dans les modèles, les captures, les tags et les destinations.
- Chercher avec AND, OR, NOT, parenthèses et expressions exactes.
- Partager le moteur entre plusieurs applications et corriger les règles à un seul endroit.
- Adopter des versions identifiées, avec des tests dans chaque application avant mise à jour.

## Architecture cible

Un cœur indépendant de l’interface, avec des adaptateurs pour chaque langage et éditeur. Le langage du cœur doit être choisi après inspection des projets consommateurs (Nyx inclut Go et Swift ; NoteMistress utilise Swift/AppKit). Ne pas imposer un runtime à toutes les applications sans vérifier leur compatibilité.

Voir [SPEC.md](SPEC.md), [UPDATES.md](UPDATES.md) et [CLAUDE_INTEGRATION_PROMPT.md](CLAUDE_INTEGRATION_PROMPT.md).

## Référence existante

NoteMistress possède déjà deux syntaxes : variables de modèles `{{texte}}`, `{{source}}`, `{{url}}`, `{{titre}}`, `{{date}}`, `{{heure}}`, `{{tags}}`, `{{commentaire}}` ; variables natives `$date`, `$day`, `$time`, `$datetime`, `$month`, `$year` et variables personnelles `$nom`.

Le code existant est dans https://github.com/bizc0m/NoteMistress : src/CaptureTemplate.swift, src/SettingsStore.swift et src/main.swift. Vérifier les versions locales et distantes avant extraction : les dernières corrections peuvent se trouver sur la branche locale codex/restore-capture-settings.
