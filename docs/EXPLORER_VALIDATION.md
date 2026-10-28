# Validation de l’explorateur — 2026-10-28

Aucune release ni fusion ; code du module épinglé par les deux consommateurs au commit `b2334729b90c6e665accf77463fe5bc139aa0b37`.

## Contrôles effectués

| Contrôle | Résultat | Preuve |
|---|---|---|
| Go `go test ./...` | 2 packages passent | exécution locale Go 1.23.12, Linux x86_64 |
| Go `go vet ./...` | passe | exécution locale |
| Contrats et catalogue Swift Linux | 22 tests, 0 échec | Swift 6.0.3, pont C Go réel, SQLite système |
| Module Swift macOS | 23 tests, 0 échec (contrats, catalogue, interface AppKit), également rejoués sur la dépendance finale exacte | [validation NYX-MD](https://github.com/bizc0m/NYX-MD/actions/runs/40716535175) et [révision finale via NoteMistress](https://github.com/bizc0m/NoteMistress/actions/runs/40717347984) |
| NoteMistress, dépendance finale exacte | tests du module, tests existants, intégration et compilation Release passent | [validation NoteMistress](https://github.com/bizc0m/NoteMistress/actions/runs/40717347984) |
| SimpleEdit Bold, intégration initiale | tests existants et nouveau test, build complet et comparaison avant/après passent | [validation initiale](https://github.com/bizc0m/MDMistress/actions/runs/40715367569) |
| SimpleEdit Bold, test XCUITest réel | en attente de la dernière exécution après adaptation du format Xcode | [validation finale](https://github.com/bizc0m/MDMistress/actions/runs/40717626741) |

Les premières exécutions ont révélé et corrigé : conformité NSSearchFieldDelegate, comparaison des bookmarks `/var` versus `/private/var`, traduction du menu SimpleEdit, exclusion des fichiers WIP du build NoteMistress, et format de projet XcodeGen incompatible avec Xcode 15.3.
Les échecs intermédiaires restent visibles dans l’historique ; ils ne constituent pas une preuve de succès de la révision finale.

## Couverture métier

- Grammaire NyxBoolean et contrats de variables/notes existants inchangés
- Hiérarchies indépendantes, exact/descendants, cycles, doublons et parent d’un autre type
- Identifiants stables, renommage, classement par lots et déplacement de références sans déplacement des fichiers
- Écrivains concurrents via deux connexions SQLite, absence de perte de mises à jour et refus des éditions/annulations obsolètes
- Annulation sur plusieurs opérations, rollback d’un lot invalide, collections dynamiques
- Sauvegarde/restauration, projection reconstruite, exports filtrés et permissions 0600
- Fichier absent, réassociation, déplacement physique/annulation et détection d’un remplacement au même chemin
- Cache invalidé par un autre écrivain ; permissions réévaluées avec cache chaud
- Un/deux panneaux, requêtes indépendantes et rafraîchissement de l’autre panneau malgré une requête invalide
- Les adaptateurs réels de chaque consommateur lisent une ressource produite avec l’identifiant source de l’autre application, dans une base temporaire

## Poids mesuré

Mesures Release, même runner et même architecture pour chaque paire ; données et index exclus ; le moteur NYX-MD est déjà présent dans la référence.

| Mesure | Avant | Après | Différence |
|---|---:|---:|---:|
| Hôte AppKit minimal + moteur → démo avec catalogue et explorateur (exécutables stripés, ARM64) | 2 400 424 octets | 2 804 312 octets | +403 888 octets |
| SimpleEdit Bold (exécutable du bundle, ARM64, build de production initial) | 4 658 560 octets | 5 258 816 octets | +600 256 octets |
| SimpleEdit Bold (bundle installé, occupation `du -sk`) | 4 588 Kio | 5 172 Kio | +584 Kio |
| NoteMistress (exécutables stripés, intégration finale ARM64) | 4 035 224 octets | 4 416 792 octets | +381 568 octets |

Ces différences concernent les builds indiqués, pas une estimation de toute la suite ; chaque app distribuée lie sa propre copie statique du module, même si le catalogue de données est commun.
Une taille de bibliothèque `.a` universelle ne représente pas le surpoids installé d’un exécutable ARM64.

## Latence et index

Benchmark synthétique : 10 000 fiches courtes, une hiérarchie, requête `#projet AND budget AND NOT attente`, 5 000 résultats, dix recherches successives ; mesure du moteur/catalogue, pas du temps de dessin de 5 000 lignes.

| Environnement | Écriture initiale | Médiane recherche | Maximum observé | Base + WAL + index |
|---|---:|---:|---:|---:|
| macOS 14 ARM64, Swift 5.10, Release, cache de projections | 0,532 s | 115,6 ms | 328,2 ms | 12 710 160 octets |
| macOS 14 ARM64, validation finale NoteMistress du module exact | 0,440 s | 69,1 ms | 245,8 ms | 12 701 944 octets |
| Linux x86_64 du poste d’exécution, Swift 6.0.3 Release, cache de projections | 0,761 s | 594,6 ms | 896,3 ms | 12 759 456 octets |

Les deux environnements ne sont pas comparables en puissance ; la médiane comprend surtout des recherches avec cache chaud, le maximum inclut le premier chargement ; ces chiffres ne préjugent pas de gros documents, d’une taxonomie profonde, de fichiers sur disque lent ou de 100 000 ressources.
La projection est persistante mais la recherche générale reste un scan booléen ; aucune recherche sémantique ni indexation automatique du contenu PDF/Office n’est revendiquée.

## Blocage restant à la livraison

Le dernier workflow SimpleEdit `40717626741` a bien créé son job `122919509465`, demandé sur `macos-15`, mais GitHub le conserve en état `queued`, sans runner attribué (`runner_id=0`, nom vide) ; les précédentes demandes sur `macos-14` restent également en file.
La révision finale du consommateur et son parcours XCUITest réel ne sont donc pas déclarés validés ; le build/test initial fonctionnel et la mesure de poids ne remplacent pas cette preuve finale.
Le test est préparé sur un hôte `com.arnaud.simple-edit.uitest-host`, base temporaire injectée, création d’une fiche, recherche dans deux panneaux et capture d’écran conservée dans le résultat Xcode ; son résultat apparaîtra dans le workflow lié ci-dessus après attribution d’un runner.
Le module exact a toutefois été compilé et ses 23 tests exécutés avec succès dans la validation finale de NoteMistress ; il n’est pas bloqué par la file d’attente du workflow public NYX-MD.

## Limites et suite de validation

Interface synchrone avec temporisation de saisie : pour généraliser à de gros catalogues, déplacer lecture/indexation/recherche hors du thread principal et ignorer les résultats obsolètes ; les mutations reconstruisent actuellement toute la projection.
Une interruption brutale entre déplacement de fichier et commit SQLite demande une réconciliation ; les bookmarks et l’état `replaced` réduisent le risque de mauvaise réassociation sans former une transaction système distribuée.
La politique d’accès est celle d’un catalogue local appartenant au même utilisateur, avec hôte de confiance ; pas de garantie de sécurité multi-utilisateur ni d’accès sandbox inter-app automatique.
Aucun logiciel installé, réglage personnel, fichier réel de l’utilisateur ou destination de capture n’a été utilisé pour les tests.
