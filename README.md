# NYX-MD

Module partagé de variables et de recherche booléenne pour les éditeurs de texte et les applications de capture.

## État réel

Ce dépôt contient la spécification et les instructions d’implémentation. Le moteur exécutable, les adaptateurs et les mises à jour automatiques ne sont pas encore implémentés. Ne pas l’ajouter comme dépendance exécutable avant une première version testée.

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
