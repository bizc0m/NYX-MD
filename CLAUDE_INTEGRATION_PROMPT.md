# Prompt à donner à Claude dans le projet de l’éditeur

Travaille dans le projet de mon éditeur de texte. Je veux intégrer un module partagé dont le dépôt canonique est https://github.com/bizc0m/NYX-MD.

Le dépôt NYX-MD contient actuellement une spécification et des instructions, pas encore de moteur exécutable. Commence par le lire : README.md, SPEC.md, UPDATES.md et AGENTS.md. Ne suppose pas qu’un paquet est déjà publié.

Objectif : un seul moteur maintenu dans NYX-MD, réutilisé par cet éditeur et mes autres projets. Il doit fournir les variables natives et personnelles, la validation et l’autocomplétion, ainsi qu’une recherche booléenne AND / OR / NOT avec parenthèses et expressions exactes. Il faut préserver les syntaxes {{nom}} et $nom existantes de NoteMistress.

Procède concrètement :
1. Inspecte le dépôt de l’éditeur, ses instructions, ses langages, son interface de recherche et son système de dépendances. Inspecte aussi le code NoteMistress accessible, notamment CaptureTemplate.swift, SettingsStore.swift et expandedVariables dans main.swift. Ne modifie pas les notes utilisateur.
2. Choisis et explique brièvement une architecture compatible avec cet éditeur et les autres consommateurs Go/Swift connus. Un cœur unique avec adaptateurs est préférable ; ne prétends pas qu’un paquet Swift fonctionne directement dans n’importe quel langage. Ne crée pas de runtime ou service réseau supplémentaire sans nécessité démontrée.
3. Implémente le moteur et ses tests dans NYX-MD selon SPEC.md. N’utilise ni eval, ni expansion récursive des valeurs capturées, ni accès implicite au presse-papiers ou aux fichiers. Documente les API et les erreurs.
4. Intègre le module à l’éditeur comme dépendance versionnée. Ajoute la recherche booléenne dans la recherche existante, avec erreur lisible si la requête est invalide. Branche les variables sur les modèles et la complétion existants en préservant les raccourcis et les documents.
5. Vérifie le parcours complet dans l’éditeur : variables personnelles, texte Unicode, expression exacte et requête (client OR fournisseur) AND NOT archivé. Ajoute des tests d’intégration pertinents.
6. Prépare une première version documentée et un mécanisme de proposition de mises à jour dans cet éditeur. Vérifie la configuration CI et les permissions avant de l’activer. Une modification du moteur doit pouvoir arriver dans les projets via une mise à jour de dépendance testée ; elle ne doit pas exécuter du code distant au démarrage.
7. Rends les changements concrets, committés et testés. Indique les dépôts et fichiers modifiés, la version utilisée, les tests passés, les étapes de publication encore nécessaires et la procédure de retour arrière. Ne déclare pas les autres projets intégrés sans les avoir réellement modifiés et vérifiés.

L’intérêt attendu dans mon éditeur : les mêmes variables et la même recherche dans tous mes outils, une correction faite à un seul endroit et des mises à jour contrôlées sans recopier le code.
