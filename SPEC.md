# Contrat fonctionnel proposé

Cette spécification est une cible à implémenter, pas une description de fonctions déjà livrées.

## Variables

- Conserver les syntaxes `{{nom}}` et `$nom` sans changer les documents existants.
- Le contexte d’appel fournit texte, URL, titre, tags, commentaire, date, fuseau, locale et variables personnelles. Le cœur ne lit ni presse-papiers, ni préférences globales, ni fichiers arbitraires.
- Les variables natives restent prioritaires ; rejeter explicitement les collisions des variables personnelles.
- Résolution en une seule passe : un texte capturé contenant `{{date}}` ou `$client` ne doit pas être interprété une deuxième fois.
- Noms complets et frontières explicites : `$client` ne doit pas modifier `$clientèle` ou `$client2`.
- Variable inconnue conservée littéralement avec un diagnostic ; aucune disparition silencieuse de contenu.
- Les échappements Markdown et URL appartiennent à des fonctions explicites, pas à un remplacement général de chaînes.
- Exposer validation, rendu et complétion via des API typées et documentées.

## Recherche booléenne

- Opérateurs AND, OR, NOT ; regroupement par parenthèses ; expressions exactes entre guillemets doubles.
- Priorité NOT > AND > OR ; opérateurs insensibles à la casse ; plusieurs termes adjacents équivalent à AND.
- Termes ordinaires : recherche de sous-chaîne ; termes entre guillemets : sous-chaîne exacte, espaces compris.
- Recherche insensible à la casse par défaut ; gestion des accents configurable et documentée.
- Requête vide : tous les documents. Requête invalide : erreur structurée avec position, sans recherche approximative silencieuse.
- Les termes peuvent contenir des tags comme #projet ou @client. Les filtres field:value, regex et jokers restent hors de la première version.
- Ne jamais utiliser eval ou exécuter le contenu d’une requête.
- Exposer parse(query), compile(query, options) et match(document) ou leurs équivalents idiomatiques.
- Limiter la longueur et la profondeur des requêtes et tester ces limites.

## Exemples attendus

- `client AND facture` : les deux termes sont présents.
- `client OR fournisseur` : au moins un terme est présent.
- `projet AND NOT archivé` : projet présent, archivé absent.
- `(client OR fournisseur) AND "à relancer"` : groupe et expression exacte.

## Validation requise avant première version

Tests sur priorités et parenthèses, erreurs de syntaxe, Unicode et accents, limites de complexité, non-récursivité des variables, collisions et variables inconnues. Ajouter un test d’intégration dans un vrai éditeur consommateur, sans modifier les documents utilisateur.
