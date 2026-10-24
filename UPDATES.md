# Distribution et mises à jour

Une modification dans le dépôt ne modifie pas automatiquement une application déjà construite ou distribuée.

## Stratégie cible

1. Livrer le cœur avec des versions SemVer et un CHANGELOG.
2. Chaque consommateur référence une version ou un commit précis ; ne pas copier le code à la main.
3. Utiliser le gestionnaire natif compatible avec le langage retenu : Swift Package Manager, module Go, paquet npm ou autre adaptateur explicitement testé.
4. Configurer un mécanisme de proposition de mise à jour (Dependabot, Renovate ou CI adaptée) pour chaque dépôt consommateur, après inspection de sa configuration et des permissions disponibles.
5. Exécuter les tests du module et de l’application avant adoption. Une version incompatible doit rester bloquée et la précédente doit pouvoir être restaurée.
6. Reconstruire et redistribuer les applications pour que les utilisateurs reçoivent les changements.

Ne pas installer un auto-updater qui télécharge et exécute du code distant au démarrage de l’éditeur. La mise à jour automatique doit passer par les dépendances, les tests et la procédure de livraison de chaque projet.

## État

Aucune publication de paquet ni automatisation de mise à jour n’est configurée à ce stade.
