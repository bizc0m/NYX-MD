// NyxCatalog 0.1 — 2026-10-28 — shared resource model; no UI or filesystem side effects.
import Foundation
import NyxMD

public enum ResourceKind: String, Codable, CaseIterable, Sendable { case link, file, card }
public enum NodeKind: String, Codable, CaseIterable, Sendable { case folder, tag, context }
public enum HierarchyMatch: String, Codable, Sendable { case exact, descendants }
public enum ReferenceStatus: String, Codable, Sendable { case available, missing, denied, stale, replaced }

public struct CatalogResource: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: ResourceKind
    public var title: String
    /// URL absolue ; fichier référencé, jamais copié dans le catalogue.
    public var locator: String
    public var body: String
    public var source: String
    public var createdAt: Date
    public var updatedAt: Date
    public var nodeIDs: Set<UUID>
    public var metadata: [String: String]
    /// Bookmark macOS facultatif ; ne constitue pas une autorisation inter-app.
    public var bookmark: Data?
    public init(id: UUID = UUID(), kind: ResourceKind, title: String, locator: String = "",
                body: String = "", source: String, nodeIDs: Set<UUID> = [],
                metadata: [String: String] = [:], bookmark: Data? = nil) {
        self.id = id; self.kind = kind; self.title = title; self.locator = locator
        self.body = body; self.source = source; self.nodeIDs = nodeIDs; self.metadata = metadata
        self.bookmark = bookmark; createdAt = Date(); updatedAt = createdAt
    }
}

public struct CatalogNode: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: NodeKind
    public var name: String
    public var parentID: UUID?
    public init(id: UUID = UUID(), kind: NodeKind, name: String, parentID: UUID? = nil) {
        self.id = id; self.kind = kind; self.name = name; self.parentID = parentID
    }
}

public struct NodeFilter: Codable, Equatable, Sendable {
    public var id: UUID
    public var match: HierarchyMatch
    public init(_ id: UUID, match: HierarchyMatch = .exact) { self.id = id; self.match = match }
}

public struct CatalogQuery: Codable, Equatable, Sendable {
    public var boolean: String
    /// Filtres en AND, indépendants du langage NyxBoolean.
    public var nodes: [NodeFilter]
    public var kinds: Set<ResourceKind>
    /// Affecte #chemin / @chemin : exact par défaut, descendants sur demande.
    public var metadataMatch: HierarchyMatch
    public var foldAccents: Bool
    public init(boolean: String = "", nodes: [NodeFilter] = [], kinds: Set<ResourceKind> = [],
                metadataMatch: HierarchyMatch = .exact, foldAccents: Bool = false) {
        self.boolean = boolean; self.nodes = nodes; self.kinds = kinds
        self.metadataMatch = metadataMatch; self.foldAccents = foldAccents
    }
}

public struct SavedSearch: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var name: String
    public var query: CatalogQuery
    public init(id: UUID = UUID(), name: String, query: CatalogQuery) {
        self.id = id; self.name = name; self.query = query
    }
}

public struct CatalogSnapshot: Codable, Equatable, Sendable {
    public var schemaVersion = 1
    public var revision: Int64 = 0
    public var resources: [CatalogResource] = []
    public var nodes: [CatalogNode] = []
    public var searches: [SavedSearch] = []
    public init() {}

    public func path(of id: UUID) -> String {
        var parts: [String] = [], seen: Set<UUID> = [], cursor: UUID? = id
        let byID = Dictionary(uniqueKeysWithValues: nodes.map { ($0.id, $0) })
        while let key = cursor, let n = byID[key], seen.insert(key).inserted {
            parts.append(n.name); cursor = n.parentID
        }
        return parts.reversed().joined(separator: "/")
    }

    public func descendants(of id: UUID) -> Set<UUID> {
        var result: Set<UUID> = [id], frontier = [id]
        let children = Dictionary(grouping: nodes, by: { $0.parentID })
        while let p = frontier.popLast() {
            for n in children[p] ?? [] where result.insert(n.id).inserted { frontier.append(n.id) }
        }
        return result
    }

    public func document(for resource: CatalogResource, ancestors: Bool = false) -> NyxDocument {
        let byID = Dictionary(uniqueKeysWithValues: nodes.map { ($0.id, $0) })
        var ids = resource.nodeIDs
        if ancestors {
            for id in resource.nodeIDs {
                var cursor = byID[id]?.parentID
                while let p = cursor, ids.insert(p).inserted { cursor = byID[p]?.parentID }
            }
        }
        let selected = ids.compactMap { byID[$0] }
        return NyxDocument(text: [resource.title, resource.body, resource.locator].joined(separator: "\n"),
                           tags: selected.filter { $0.kind == .tag }.map { path(of: $0.id) },
                           entities: selected.filter { $0.kind == .context }.map { path(of: $0.id) })
    }

    public func validate() throws {
        guard schemaVersion == 1 else { throw CatalogError.unsupportedSchema(schemaVersion) }
        guard Set(nodes.map(\.id)).count == nodes.count,
              Set(resources.map(\.id)).count == resources.count,
              Set(searches.map(\.id)).count == searches.count else { throw CatalogError.invalid("Identifiant dupliqué") }
        let byID = Dictionary(uniqueKeysWithValues: nodes.map { ($0.id, $0) })
        var siblings: Set<String> = []
        for n in nodes {
            let name = n.name.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty, name == n.name, !name.contains("/"),
                  !name.contains("#"), !name.contains("@"), !name.contains("\""),
                  !name.unicodeScalars.contains(where: { CharacterSet.controlCharacters.contains($0) }) else {
                throw CatalogError.invalid("Nom de classement invalide")
            }
            let sibling = "\(n.kind.rawValue):\(n.parentID?.uuidString ?? "root"):\(name.lowercased())"
            guard siblings.insert(sibling).inserted else { throw CatalogError.invalid("Classement déjà existant") }
            var visited: Set<UUID> = [n.id], cursor = n.parentID
            while let parent = cursor {
                guard let p = byID[parent], p.kind == n.kind else { throw CatalogError.invalid("Parent absent ou d'une autre hiérarchie") }
                guard visited.insert(parent).inserted else { throw CatalogError.invalid("Cycle de classement") }
                cursor = p.parentID
            }
        }
        for r in resources {
            guard !r.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, !r.source.isEmpty,
                  r.nodeIDs.isSubset(of: Set(byID.keys)) else { throw CatalogError.invalid("Fiche ou classement invalide") }
            if r.kind != .card {
                guard let u = URL(string: r.locator) else { throw CatalogError.invalid("URL invalide") }
                if r.kind == .file {
                    guard u.isFileURL, u.host == nil || u.host == "" || u.host == "localhost",
                          u.path.hasPrefix("/") else { throw CatalogError.invalid("Fichier local absolu requis") }
                } else {
                    guard ["http", "https"].contains(u.scheme?.lowercased() ?? ""), u.host != nil else {
                        throw CatalogError.invalid("Lien HTTP(S) requis")
                    }
                }
            }
        }
        for s in searches {
            guard !s.name.isEmpty, s.query.nodes.allSatisfy({ byID[$0.id] != nil }) else {
                throw CatalogError.invalid("Recherche enregistrée invalide")
            }
            _ = try NyxQuery(s.query.boolean)
        }
    }
}

public enum CatalogError: Error, LocalizedError {
    case database(String), invalid(String), unsupportedSchema(Int), conflict, notFound, denied, physicalMoveRequiresExplicitAction
    public var errorDescription: String? {
        switch self {
        case .database(let s): return "Catalogue : \(s)"
        case .invalid(let s): return s
        case .unsupportedSchema(let n): return "Version de catalogue non prise en charge : \(n)"
        case .conflict: return "Le catalogue a changé dans une autre fenêtre ou application ; actualisez avant de recommencer"
        case .notFound: return "Élément introuvable"
        case .denied: return "Accès refusé à cette ressource"
        case .physicalMoveRequiresExplicitAction: return "Le classement déplace seulement la référence ; le fichier réel reste à sa place"
        }
    }
}

/// Frontière de confiance : catalogue local du même utilisateur, pas un service multi-utilisateur.
/// Chaque hôte doit appliquer ses propres autorisations AVANT de présenter les résultats.
public struct CatalogAccess {
    public var permits: (CatalogResource) -> Bool
    public let allowsOwnerOperations: Bool
    public init(allowsOwnerOperations: Bool = false, permits: @escaping (CatalogResource) -> Bool) {
        self.allowsOwnerOperations = allowsOwnerOperations; self.permits = permits
    }
    public static var localUser: CatalogAccess {
        CatalogAccess(allowsOwnerOperations: true) { r in
            guard r.kind == .file else { return true }
            guard let status = try? LocalFileConnector.resolved(r).1 else { return false }
            return status != .denied
        }
    }
}

/// Construit un groupe sans permettre qu'une saisie soit interprétée comme opérateur.
public enum BooleanBuilder {
    public static func group(_ values: [String], operation: String = "AND", negated: Bool = false) throws -> String {
        guard ["AND", "OR"].contains(operation), !values.isEmpty else { throw CatalogError.invalid("Groupe vide ou opérateur invalide") }
        let terms = try values.map { value -> String in
            guard !value.contains("\""), !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                throw CatalogError.invalid("Terme vide ou contenant un guillemet")
            }
            return "\"\(value)\""
        }
        let query = "\(negated ? "NOT " : "")(\(terms.joined(separator: " \(operation) ")))"
        _ = try NyxQuery(query)
        return query
    }
}
