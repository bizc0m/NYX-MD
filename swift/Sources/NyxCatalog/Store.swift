// NyxCatalog 0.1 — transactional SQLite persistence, projection and revision-aware undo.
import Foundation
import CSQLite
import NyxMD
#if os(Linux)
import Glibc
#else
import Darwin
#endif

private final class Database {
    var handle: OpaquePointer?
    init(_ url: URL) throws {
        let parent = url.deletingLastPathComponent()
        if !FileManager.default.fileExists(atPath: parent.path) {
            try FileManager.default.createDirectory(at: parent, withIntermediateDirectories: true,
                                                   attributes: [.posixPermissions: 0o700])
        }
        // Precreate privately so SQLite's WAL inherits 0600, without changing an existing directory.
        let fd = open(url.path, O_CREAT | O_EXCL | O_WRONLY, mode_t(0o600))
        if fd >= 0 { close(fd) } else if errno != EEXIST { throw CatalogError.database("Création impossible") }
        guard sqlite3_open_v2(url.path, &handle, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_FULLMUTEX, nil) == SQLITE_OK else {
            let detail = handle.map { String(cString: sqlite3_errmsg($0)) } ?? "Ouverture impossible"
            sqlite3_close(handle); handle = nil; throw CatalogError.database(detail)
        }
        sqlite3_busy_timeout(handle, 5000)
        try exec("PRAGMA journal_mode=WAL")
        try exec("PRAGMA synchronous=FULL")
    }
    deinit { sqlite3_close(handle) }
    func exec(_ sql: String) throws {
        guard sqlite3_exec(handle, sql, nil, nil, nil) == SQLITE_OK else { throw error() }
    }
    func error() -> CatalogError { .database(String(cString: sqlite3_errmsg(handle))) }
    func statement(_ sql: String, _ arguments: [String] = []) throws -> OpaquePointer {
        var s: OpaquePointer?
        guard sqlite3_prepare_v2(handle, sql, -1, &s, nil) == SQLITE_OK, let s else { throw error() }
        for (i, v) in arguments.enumerated() {
            let result = v.withCString { sqlite3_bind_text(s, Int32(i + 1), $0, -1, unsafeBitCast(-1, to: sqlite3_destructor_type.self)) }
            guard result == SQLITE_OK else { sqlite3_finalize(s); throw error() }
        }
        return s
    }
    func write(_ sql: String, _ arguments: [String] = []) throws {
        let s = try statement(sql, arguments); defer { sqlite3_finalize(s) }
        guard sqlite3_step(s) == SQLITE_DONE else { throw error() }
    }
    func rows(_ sql: String, _ arguments: [String] = []) throws -> [[String]] {
        let s = try statement(sql, arguments); defer { sqlite3_finalize(s) }
        var result: [[String]] = []
        while true {
            switch sqlite3_step(s) {
            case SQLITE_ROW:
                result.append((0..<sqlite3_column_count(s)).map { i in
                    sqlite3_column_text(s, i).map { String(cString: $0) } ?? ""
                })
            case SQLITE_DONE: return result
            default: throw error()
            }
        }
    }
}

public struct CatalogUndo {
    fileprivate var snapshot: CatalogSnapshot
    public let expectedRevision: Int64
}

/// Thread-safe connection; each mutation rereads current state within BEGIN IMMEDIATE.
/// Multiple processes share WAL; no persistent connection cache of resource state.
public final class CatalogStore {
    public let url: URL
    private let db: Database
    private let lock = NSRecursiveLock()
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    public static func defaultURL() throws -> URL {
        let base = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask,
                                              appropriateFor: nil, create: true)
        return base.appendingPathComponent("NYX-Suite", isDirectory: true).appendingPathComponent("catalog.sqlite")
    }

    public init(url: URL) throws {
        self.url = url; db = try Database(url)
        encoder.outputFormatting = [.sortedKeys]
        try db.exec("BEGIN IMMEDIATE")
        do {
            let version = Int(try db.rows("PRAGMA user_version").first?.first ?? "0") ?? 0
            guard version <= 1 else { throw CatalogError.unsupportedSchema(version) }
            // Migration 0 → 1 : création seulement ; aucune donnée utilisateur écrasée.
            try db.exec("CREATE TABLE IF NOT EXISTS catalog (id INTEGER PRIMARY KEY CHECK(id=1), payload TEXT NOT NULL)")
            try db.exec("CREATE TABLE IF NOT EXISTS projection (id TEXT PRIMARY KEY, kind TEXT NOT NULL, source TEXT NOT NULL, exact TEXT NOT NULL, expanded TEXT NOT NULL)")
            try db.exec("CREATE INDEX IF NOT EXISTS projection_kind ON projection(kind)")
            try db.exec("CREATE INDEX IF NOT EXISTS projection_source ON projection(source)")
            if try db.rows("SELECT id FROM catalog").isEmpty { try persist(CatalogSnapshot()) }
            try db.exec("PRAGMA user_version=1")
            try db.exec("COMMIT")
        } catch { try? db.exec("ROLLBACK"); throw error }
    }

    private func encoded<T: Encodable>(_ value: T) throws -> String {
        String(decoding: try encoder.encode(value), as: UTF8.self)
    }
    private func read() throws -> CatalogSnapshot {
        guard let raw = try db.rows("SELECT payload FROM catalog WHERE id=1").first?.first else { throw CatalogError.notFound }
        return try decoder.decode(CatalogSnapshot.self, from: Data(raw.utf8))
    }
    /// Owner-level snapshot, including metadata; apply host policy before presentation/export.
    public func snapshot() throws -> CatalogSnapshot {
        lock.lock(); defer { lock.unlock() }; return try read()
    }
    public func revision() throws -> Int64 { try snapshot().revision }

    private func persist(_ state: CatalogSnapshot) throws {
        try db.write("INSERT OR REPLACE INTO catalog(id,payload) VALUES(1,?)", [try encoded(state)])
        try db.exec("DELETE FROM projection")
        for r in state.resources {
            try db.write("INSERT INTO projection(id,kind,source,exact,expanded) VALUES(?,?,?,?,?)",
                         [r.id.uuidString, r.kind.rawValue, r.source,
                          try encoded(state.document(for: r)), try encoded(state.document(for: r, ancestors: true))])
        }
    }

    @discardableResult
    public func transact(expectedRevision: Int64? = nil, _ change: (inout CatalogSnapshot) throws -> Void) throws -> CatalogUndo {
        lock.lock(); defer { lock.unlock() }
        try db.exec("BEGIN IMMEDIATE")
        do {
            let before = try read()
            if let expectedRevision, before.revision != expectedRevision { throw CatalogError.conflict }
            var after = before
            try change(&after)
            after.schemaVersion = 1; after.revision = before.revision + 1
            try after.validate()
            try persist(after)
            try db.exec("COMMIT")
            return CatalogUndo(snapshot: before, expectedRevision: after.revision)
        } catch { try? db.exec("ROLLBACK"); throw error }
    }

    /// Returns inverse token (redo); refuses to erase intervening changes from another app.
    @discardableResult
    public func undo(_ token: CatalogUndo) throws -> CatalogUndo {
        try transact(expectedRevision: token.expectedRevision) { $0 = token.snapshot }
    }

    public func search(_ query: CatalogQuery, access: CatalogAccess) throws -> [CatalogResource] {
        let engine = try NyxQuery(query.boolean, options: NyxSearchOptions(foldAccents: query.foldAccents))
        lock.lock(); defer { lock.unlock() }
        try db.exec("BEGIN")
        let state: CatalogSnapshot
        let records: [[String]]
        do {
            state = try read()
            records = try db.rows("SELECT id,\(query.metadataMatch == .exact ? "exact" : "expanded") FROM projection ORDER BY id")
            try db.exec("COMMIT")
        } catch { try? db.exec("ROLLBACK"); throw error }
        let byID = Dictionary(uniqueKeysWithValues: state.resources.map { ($0.id.uuidString, $0) })
        let filters: [Set<UUID>] = try query.nodes.map { f in
            guard state.nodes.contains(where: { $0.id == f.id }) else { throw CatalogError.notFound }
            return f.match == .exact ? [f.id] : state.descendants(of: f.id)
        }
        var resources: [CatalogResource] = [], docs: [NyxDocument] = []
        for record in records {
            guard let r = byID[record[0]], access.permits(r),
                  query.kinds.isEmpty || query.kinds.contains(r.kind),
                  filters.allSatisfy({ !r.nodeIDs.isDisjoint(with: $0) }) else { continue }
            resources.append(r)
            docs.append(try decoder.decode(NyxDocument.self, from: Data(record[1].utf8)))
        }
        let matches = try engine.match(docs)
        return zip(resources, matches).compactMap { $0.1 ? $0.0 : nil }.sorted {
            let order = $0.title.localizedStandardCompare($1.title)
            return order == .orderedSame ? $0.id.uuidString < $1.id.uuidString : order == .orderedAscending
        }
    }

    /// Exports only allowed resources and their ancestors; saved searches can disclose hidden names,
    /// so filtered exports intentionally omit them. Full owner backup uses backup(to:).
    public func export(to destination: URL, access: CatalogAccess) throws {
        var state = try snapshot()
        state.resources = state.resources.filter(access.permits)
        let byID = Dictionary(uniqueKeysWithValues: state.nodes.map { ($0.id, $0) })
        var visible = state.resources.reduce(into: Set<UUID>()) { $0.formUnion($1.nodeIDs) }
        for id in Array(visible) {
            var cursor = byID[id]?.parentID
            while let parent = cursor, visible.insert(parent).inserted { cursor = byID[parent]?.parentID }
        }
        state.nodes = state.nodes.filter { visible.contains($0.id) }; state.searches = []
        try writeExport(state, to: destination)
    }
    public func backup(to destination: URL) throws { try writeExport(snapshot(), to: destination) }
    private func writeExport(_ state: CatalogSnapshot, to destination: URL) throws {
        guard destination.standardizedFileURL != url.standardizedFileURL else { throw CatalogError.invalid("La sauvegarde ne peut remplacer la base") }
        let temp = destination.deletingLastPathComponent().appendingPathComponent(".nyx-export-\(UUID().uuidString)")
        let fd = open(temp.path, O_WRONLY | O_CREAT | O_EXCL, mode_t(0o600))
        guard fd >= 0 else { throw CatalogError.database("Destination non accessible") }
        let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
        defer { try? handle.close(); try? FileManager.default.removeItem(at: temp) }
        try handle.write(contentsOf: encoder.encode(state)); try handle.synchronize(); try handle.close()
        guard rename(temp.path, destination.path) == 0 else { throw CatalogError.database("Écriture de sauvegarde impossible") }
    }
    @discardableResult
    public func restore(from source: URL, expectedRevision: Int64) throws -> CatalogUndo {
        let data = try Data(contentsOf: source, options: .mappedIfSafe)
        guard data.count <= 64 * 1024 * 1024 else { throw CatalogError.invalid("Sauvegarde supérieure à 64 Mo") }
        let state = try decoder.decode(CatalogSnapshot.self, from: data)
        try state.validate()
        return try transact(expectedRevision: expectedRevision) { $0 = state }
    }
}

public enum CatalogActions {
    public static func upsert(_ resource: CatalogResource, in state: inout CatalogSnapshot) {
        var r = resource; r.updatedAt = Date()
        if let i = state.resources.firstIndex(where: { $0.id == r.id }) {
            r.createdAt = state.resources[i].createdAt; state.resources[i] = r
        } else { state.resources.append(r) }
    }
    /// Folder membership changes a reference only; tags and contexts are retained.
    public static func classify(_ ids: Set<UUID>, into target: UUID, replacingFolder source: UUID? = nil,
                                in state: inout CatalogSnapshot) throws {
        guard let node = state.nodes.first(where: { $0.id == target }),
              ids.isSubset(of: Set(state.resources.map(\.id))) else { throw CatalogError.notFound }
        if let source {
            guard node.kind == .folder, state.nodes.contains(where: { $0.id == source && $0.kind == .folder }) else {
                throw CatalogError.invalid("Le déplacement de référence exige deux dossiers")
            }
        }
        for i in state.resources.indices where ids.contains(state.resources[i].id) {
            if let source { state.resources[i].nodeIDs.remove(source) }
            state.resources[i].nodeIDs.insert(target); state.resources[i].updatedAt = Date()
        }
    }
    public static func removeNode(_ id: UUID, in state: inout CatalogSnapshot) throws {
        guard state.nodes.contains(where: { $0.id == id }) else { throw CatalogError.notFound }
        let removed = state.descendants(of: id)
        // Never silently alter the semantics of a saved collection.
        guard !state.searches.contains(where: { $0.query.nodes.contains(where: { removed.contains($0.id) }) }) else {
            throw CatalogError.invalid("Classement utilisé par une recherche enregistrée")
        }
        state.nodes.removeAll { removed.contains($0.id) }
        for i in state.resources.indices { state.resources[i].nodeIDs.subtract(removed) }
    }
}
