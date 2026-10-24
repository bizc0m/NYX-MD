# Instructions pour les agents

NYX-MD est le dépôt canonique envisagé pour les variables et la recherche booléenne partagées.

- Lire README.md, SPEC.md et UPDATES.md avant implémentation.
- Distinguer les fonctions proposées et les fonctions réellement livrées.
- Inspecter les langages et les contraintes du premier consommateur avant de choisir le langage du cœur.
- Préserver les syntaxes de variables déjà utilisées par NoteMistress.
- Garder le cœur indépendant de l’interface, du stockage, du réseau et des préférences globales.
- Ne pas copier le moteur dans chaque application : utiliser une dépendance versionnée.
- Tester les comportements métier et les erreurs ; documenter les incompatibilités et les migrations.
- Ne pas prétendre que les projets consommateurs sont mis à jour sans avoir modifié et testé leur dépendance.
