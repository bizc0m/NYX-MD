# Catalogue et explorateur partagés — aperçu expérimental 0.1

État : branche `feat/transversal-explorer`, aucune release ni fusion automatique.
Le moteur Go et ses contrats 0.3.0 restent inchangés ; les nouveaux produits sont Swift.

## Audit et choix

- `AGENTS.md`, `README.md`, `SPEC.md`, `UPDATES.md`, `docs/API.md`, les vecteurs de conformité et les consommateurs ont été inspectés.
- `SPEC.md` est une cible historique : sa description sous-chaîne/opérateurs sans distinction de casse contredit le contrat effectivement livré ; `docs/API.md` et les tests du moteur font référence.
- Avant cette branche : moteur booléen, variables, notes, interface C et adaptateur Swift ; aucun catalogue partagé, stockage, hiérarchie de ressources ou explorateur générique.
- SimpleEdit Bold (`bizc0m/MDMistress`, base master `cce1b439…`) utilise Swift/AppKit, SwiftPM et un hôte XCUITest isolé ; l'explorateur et les favoris existants sont conservés.
- NoteMistress (base `feat/nyx-md-variables`, `39f16572…`) utilise Swift/AppKit ; son build de production compile une liste explicite de fichiers, certaines classes `*ViewController` étant des travaux non intégrés ; son Package.swift exige Swift 6.3, mais sa compilation manuelle actuelle fonctionne avec Swift 5.10.
- Choix : `NyxCatalog` (Foundation + SQLite système + NyxMD), `NyxExplorerUI` (AppKit + NyxCatalog), connecteur fichier séparé ; aucun nouveau runtime embarqué.
- Le moteur Go ne connaît toujours ni disque, ni catalogue, ni interface ; il n'a pas été recopié.

## Données et sémantique

`CatalogResource` : UUID stable, type link/file/card, titre, URL de référence, texte de fiche, source, dates, métadonnées, associations et bookmark macOS facultatif ; les dates sont persistées avec l'encodage Date de Foundation (secondes depuis 2001-01-01).
`CatalogNode` : UUID, type folder/tag/context, nom et parent du même type ; les trois arbres sont indépendants, sans cycles ni noms frères identiques (comparaison sans distinction de casse).
Une fiche peut appartenir à plusieurs dossiers, tags et contextes ; ces dossiers sont des collections virtuelles, sans correspondance implicite avec le Finder.
Les noms ne contiennent pas `/`, `#`, `@`, guillemet ou caractère de contrôle ; `/` sépare les niveaux d'un chemin.

- Filtre de nœud exact : association directe seulement
- Filtre avec descendants : association à ce nœud ou à n'importe quel descendant
- Plusieurs filtres structurés : AND ; plusieurs types de ressource : OR
- `#marketing/SEO` et `@client/Acme` sont des chemins complets ; le moteur reçoit ces métadonnées explicitement
- Avec `metadataMatch = .descendants`, les chemins des ancêtres sont ajoutés au document évalué ; le langage NyxBoolean ne change pas
- Les chemins avec espaces sont toujours sélectionnables par UUID dans les filtres ; pour une recherche textuelle de métadonnées, utiliser la syntaxe de guillemets prise en charge par NyxBoolean
- Une recherche enregistrée contient une requête, pas une copie des résultats ; elle constitue une collection dynamique
- La suppression d'un classement enlève les associations de sa branche, conserve les fiches et refuse les branches utilisées par des filtres enregistrés

## Persistance et concurrence

SQLite système, WAL, synchronous FULL et busy_timeout de 5 secondes ; base créée en 0600 et nouveau répertoire en 0700.
Migration 0 → 1 transactionnelle, versions supérieures refusées, aucun remplacement silencieux.
Le catalogue v1 conserve un état JSON et une projection persistante des documents exacts/avec ancêtres, indexée par identifiant, type et source ; ces projections sont reconstruites transactionnellement après mutation.
La recherche générale évalue NyxBoolean sur les candidats : il ne s'agit pas encore d'un index inversé plein texte ; le coût croît avec le nombre de ressources et le benchmark doit décider du besoin d'une évolution.
Chaque mutation relit l'état dans BEGIN IMMEDIATE ; la révision attendue empêche d'écraser une édition concurrente ; les deux panneaux et les autres applications se rafraîchissent à la révision suivante (interrogation chaque seconde quand la fenêtre est visible).
L'annulation est en mémoire, jusqu'à 50 modifications de catalogue, et refuse les changements intervenus dans une autre application ; elle ne survit pas à un redémarrage.

## Fichiers et autorisations

L'import référence le fichier, sans copie ni lecture automatique de son contenu ; seul le titre, l'URL et le texte saisi sont recherchés.
Le bookmark macOS aide à retrouver un fichier déplacé ; un UUID de catalogue reste indépendant du chemin ; une référence peut être disponible, absente, refusée ou à actualiser.
La réassociation est explicite et conserve l'UUID et les classements ; un fichier absent n'est jamais supprimé automatiquement du catalogue.
Le classement, le glisser-déposer et « Déplacer réf… » ne déplacent aucun fichier réel.
« Déplacer le fichier réel… » est une opération distincte, confirmée, sans écrasement ; si SQLite échoue, le connecteur tente de remettre le fichier à sa place et signale les chemins si la compensation échoue ; son annulation dédiée refuse une révision intermédiaire.
La transaction SQLite et le déplacement système ne constituent pas une transaction distribuée résistante à un arrêt brutal ; une interruption entre les deux opérations exige une réconciliation via le bookmark ou « Réassocier ».

`CatalogAccess` est appliqué avant les résultats et avant l'ouverture ; les hôtes peuvent fournir une politique plus restrictive que `.localUser` ; les instantanés de présentation et les exports filtrés ne contiennent pas les classements cachés ni les recherches enregistrées d'un autre périmètre.
Les opérations propriétaire (backup complet/restauration) sont distinguées ; les API de mutation et snapshot brut sont réservées à l'hôte de confiance, pas exposables directement à des clients non fiables.
Le catalogue est local au même utilisateur système : pas de séparation multi-utilisateur, pas de service réseau, pas de coffre chiffré, pas de synchronisation cloud ni d'accès sandbox macOS partagé automatique ; pour une app sandboxée, prévoir un App Group et des bookmarks de sécurité propres à chaque hôte avant adoption.

## Sauvegarde, export et restauration

- `backup(to:)` : état complet JSON versionné, sans les fichiers référencés ; destination écrite atomiquement en 0600
- `export(to:access:)` : seulement les ressources autorisées et leurs classements nécessaires ; recherches enregistrées omises pour ne pas divulguer des requêtes hors périmètre
- `restore(from:expectedRevision:)` : validation, limite de 64 Mo, remplacement transactionnel, nouvelle révision et jeton d'annulation ; les projections sont reconstruites
- La sauvegarde ne peut remplacer la base, son WAL ni son SHM ; les chemins et bookmarks restent des données privées de l'utilisateur

## Interface et intégration

Un panneau initial, F3 ou bouton pour basculer en deux, séparateur redimensionnable ; requête, scope, tri et sélection indépendants ; Entrée ouvre, Retour arrière remonte, Tab passe à l'autre panneau, F4 modifie, Cmd-Z annule le classement.
Navigation par chemin et sous-niveaux ; sélection multiple et glisser-déposer des références vers un classement, fichiers/liens externes référencés ; classement par lots, retrait d'associations, édition des fiches et des arbres ; constructeur visuel AND/OR/NOT par termes et champs séparés pour les hiérarchies.
L'interface de cet aperçu est en français ; la localisation de l'explorateur partagé n'est pas encore extraite en ressources.
Le champ texte de fiche est un éditeur simple ; pas de rendu riche, de prévisualisation des fichiers ou de recherche sémantique IA.

```swift
import NyxCatalog
import NyxExplorerUI
let store = try CatalogStore(url: CatalogStore.defaultURL())
let window = NyxExplorerWindow(store: store, source: "mon-app", access: .localUser) { resource in
    // Ouvrir dans l'hôte, après le contrôle d'accès du module.
}
window.showWindow(nil) // conserver window dans le contrôleur hôte
```

Les deux branches consommatrices épinglent un commit exact de NYX-MD ; un changement du module ne met jamais une app distribuée à jour automatiquement.
SimpleEdit : menu Fichier → Explorateur partagé NYX ; ses fichiers s'ouvrent dans l'éditeur existant.
NoteMistress : menu et icône de statut → Explorateur partagé NYX ; ouverture via l'application système associée.
Les destinations de capture, syntaxes de variables et préférences existantes ne changent pas.
`NYX_CATALOG_PATH` permet une base temporaire explicitement choisie pour les tests ; la démo utilise par défaut un catalogue temporaire distinct du catalogue réel.

## Vérification reproductible

macOS : `swift test`, `swift run -c release NyxCatalogBenchmark 10000`, `swift run -c release NyxExplorerDemo` (uniquement pour essais isolés).
Workflow `Explorer validation` : tests, compilation Release et poids des exécutables stripés de la démo et d'un hôte AppKit minimal avec le même moteur, même architecture et même compilateur.
SimpleEdit : workflow `Explorer integration`, tests existants et nouveau test, `build.sh --test`, comparaison des bundles complets avant/après sur le même runner, puis XCUITest isolé ; aucune app installée n'est lancée.
NoteMistress : tests existants, harness du connecteur réel, compilation de la liste de sources de production sans signature, comparaison des exécutables avant/après avec le même cœur et les mêmes options ; aucune installation.
Les résultats et limites mesurés sont consignés dans `EXPLORER_VALIDATION.md` ; aucune estimation de poids n'est présentée comme mesure.

## Limites de l'aperçu

Le catalogue et les opérations s'exécutent synchroniquement dans l'interface actuelle : un grand catalogue peut bloquer la fenêtre pendant une recherche ou une réindexation ; mesurer avant de généraliser, puis déplacer les travaux longs vers une file dédiée avec annulation des requêtes obsolètes.
Les services externes et applications hors Swift/AppKit nécessitent des connecteurs supplémentaires ; cet aperçu valide la réutilisation dans deux applications macOS, pas l'intégration automatique de toute la suite.

Les projections décodées et l'ordre de tri sont mis en cache en mémoire, invalidés sur chaque écriture locale ou changement de `PRAGMA data_version` d'une autre connexion ; les permissions de fichiers sont revérifiées à chaque recherche, y compris avec un cache chaud, et la saisie est temporisée de 180 ms.
Un identifiant système de fichier (volume/inode) détecte les remplacements au même chemin ; ils sont signalés `replaced` et exigent une réassociation explicite avant ouverture/déplacement.

Développement Linux (sans interface AppKit) : construire le pont C avec `go build -buildmode=c-shared -o .build/native/libnyxmd.so ./cabi`, puis `LD_LIBRARY_PATH="$PWD/.build/native" swift test --jobs 1 -Xlinker -L"$PWD/.build/native"` ; SQLite et ses en-têtes doivent être installés ; le produit macOS continue d'utiliser le XCFramework existant.
