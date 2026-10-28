import XCTest
import NyxMD
@testable import NyxCatalog

final class CatalogTests: XCTestCase {
    var directory: URL!
    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }
    override func tearDownWithError() throws { try FileManager.default.removeItem(at: directory) }
    func store(_ filename: String = "catalog.sqlite") throws -> CatalogStore { try CatalogStore(url: directory.appendingPathComponent(filename)) }
    func fixture(_ store: CatalogStore) throws -> (CatalogNode, CatalogNode, CatalogNode, CatalogResource) {
        let root = CatalogNode(kind: .tag, name: "marketing")
        let child = CatalogNode(kind: .tag, name: "SEO", parentID: root.id)
        let context = CatalogNode(kind: .context, name: "Acme")
        let r = CatalogResource(kind: .link, title: "Audit référencement", locator: "https://example.org", body: "budget validé", source: "app-one", nodeIDs: [child.id, context.id])
        try store.transact { $0.nodes = [root, child, context]; $0.resources = [r] }
        return (root, child, context, r)
    }
    func testBooleanAndIndependentHierarchySemantics() throws {
        let db = try store(); let (root, child, context, r) = try fixture(db)
        XCTAssertEqual(try db.search(CatalogQuery(boolean: "budget AND @Acme"), access: .localUser).map(\.id), [r.id])
        XCTAssertTrue(try db.search(CatalogQuery(boolean: "#marketing"), access: .localUser).isEmpty)
        XCTAssertEqual(try db.search(CatalogQuery(boolean: "#marketing", metadataMatch: .descendants), access: .localUser).count, 1)
        XCTAssertEqual(try db.search(CatalogQuery(boolean: "#marketing/SEO"), access: .localUser).count, 1)
        XCTAssertTrue(try db.search(CatalogQuery(nodes: [NodeFilter(root.id)]), access: .localUser).isEmpty)
        XCTAssertEqual(try db.search(CatalogQuery(nodes: [NodeFilter(root.id, match: .descendants), NodeFilter(context.id)]), access: .localUser).count, 1)
        XCTAssertEqual(try db.search(CatalogQuery(nodes: [NodeFilter(child.id)], kinds: [.link]), access: .localUser).count, 1)
        XCTAssertTrue(try db.search(CatalogQuery(kinds: [.file]), access: .localUser).isEmpty)
        XCTAssertThrowsError(try db.search(CatalogQuery(boolean: "AND"), access: .localUser))
    }
    func testCyclesOtherHierarchyAndDuplicateSiblingRollback() throws {
        let db = try store(); let (root, child, context, _) = try fixture(db)
        let before = try db.snapshot()
        XCTAssertThrowsError(try db.transact { $0.nodes[0].parentID = child.id })
        XCTAssertThrowsError(try db.transact { $0.nodes[1].parentID = context.id })
        XCTAssertThrowsError(try db.transact { $0.nodes.append(CatalogNode(kind: .tag, name: root.name)) })
        XCTAssertEqual(try db.snapshot(), before)
    }
    func testStableIDsRenameReparentAndMembershipBatch() throws {
        let db = try store(); let (_, child, _, r) = try fixture(db)
        let folderA = CatalogNode(kind: .folder, name: "Projet A"), folderB = CatalogNode(kind: .folder, name: "Projet B")
        try db.transact { state in
            state.nodes += [folderA, folderB]
            state.nodes[state.nodes.firstIndex(where: { $0.id == child.id })!].name = "Search"
            try CatalogActions.classify([r.id], into: folderA.id, in: &state)
        }
        try db.transact { try CatalogActions.classify([r.id], into: folderB.id, replacingFolder: folderA.id, in: &$0) }
        let current = try db.snapshot().resources[0]
        XCTAssertEqual(current.id, r.id); XCTAssertEqual(current.locator, r.locator)
        XCTAssertFalse(current.nodeIDs.contains(folderA.id)); XCTAssertTrue(current.nodeIDs.contains(folderB.id))
        XCTAssertTrue(current.nodeIDs.contains(child.id))
        XCTAssertEqual(try db.search(CatalogQuery(boolean: "#marketing/Search"), access: .localUser).count, 1)
    }
    func testTwoConnectionsConflictUndoAndDynamicSavedSearch() throws {
        let a = try store(), b = try store(); let (_, _, _, r) = try fixture(a)
        let token = try a.transact { $0.searches.append(SavedSearch(name: "Budget", query: CatalogQuery(boolean: "budget"))) }
        XCTAssertEqual(try b.snapshot().searches.count, 1)
        XCTAssertEqual(try b.search(b.snapshot().searches[0].query, access: .localUser).count, 1)
        try b.transact { state in
            var second = r; second.id = UUID(); state.resources.append(second)
        }
        XCTAssertThrowsError(try a.undo(token))
        XCTAssertThrowsError(try a.transact(expectedRevision: token.expectedRevision) { $0.resources = [] })
        XCTAssertEqual(try a.search(a.snapshot().searches[0].query, access: .localUser).count, 2)
        let remove = try a.transact { $0.resources.removeAll() }
        let redo = try b.undo(remove)
        XCTAssertEqual(try a.snapshot().resources.count, 2)
        _ = try a.undo(redo)
        XCTAssertTrue(try b.snapshot().resources.isEmpty)
    }
    func testConcurrentWritersDoNotLoseUpdates() throws {
        let a = try store(), b = try store()
        let group = DispatchGroup(), errors = ErrorBox()
        for i in 0..<20 {
            group.enter()
            DispatchQueue.global().async {
                defer { group.leave() }
                do { try (i % 2 == 0 ? a : b).transact { $0.resources.append(CatalogResource(kind: .card, title: "\(i)", source: "test")) } }
                catch { errors.append(error) }
            }
        }
        XCTAssertEqual(group.wait(timeout: .now() + 20), .success)
        XCTAssertTrue(errors.values.isEmpty, "\(errors.values)")
        XCTAssertEqual(try a.snapshot().resources.count, 20)
        XCTAssertEqual(try a.revision(), 20)
    }
    func testBackupRestoreProjectionAndFilteredExport() throws {
        let db = try store(); let (_, _, _, r) = try fixture(db)
        let backup = directory.appendingPathComponent("backup.json")
        try db.backup(to: backup)
        let other = try store("other.sqlite")
        _ = try other.restore(from: backup, expectedRevision: 0)
        XCTAssertEqual(try other.search(CatalogQuery(boolean: "#marketing/SEO AND budget"), access: .localUser).map(\.id), [r.id])
        let export = directory.appendingPathComponent("export.json")
        let deny = CatalogAccess { _ in false }
        XCTAssertTrue(try db.search(CatalogQuery(), access: deny).isEmpty)
        try db.export(to: export, access: deny)
        let filtered = try JSONDecoder().decode(CatalogSnapshot.self, from: Data(contentsOf: export))
        XCTAssertTrue(filtered.resources.isEmpty); XCTAssertTrue(filtered.nodes.isEmpty); XCTAssertTrue(filtered.searches.isEmpty)
        let attributes = try FileManager.default.attributesOfItem(atPath: backup.path)
        XCTAssertEqual((attributes[.posixPermissions] as? NSNumber)?.intValue, 0o600)
        XCTAssertThrowsError(try db.backup(to: db.url))
        var invalid = try db.snapshot(); invalid.schemaVersion = 99
        try JSONEncoder().encode(invalid).write(to: export)
        XCTAssertThrowsError(try other.restore(from: export, expectedRevision: other.revision()))
        XCTAssertEqual(try other.snapshot().resources.count, 1)
    }
    func testFilesMissingRelinkPhysicalMoveAndUndo() throws {
        let db = try store()
        let from = directory.appendingPathComponent("original.txt"), to = directory.appendingPathComponent("moved.txt")
        try Data("original content".utf8).write(to: from)
        let r = try LocalFileConnector.reference(from, source: "test")
        try db.transact { $0.resources.append(r) }
        let receipt = try db.moveFile(r.id, to: to, expectedRevision: db.revision(), access: .localUser)
        XCTAssertFalse(FileManager.default.fileExists(atPath: from.path)); XCTAssertTrue(FileManager.default.fileExists(atPath: to.path))
        XCTAssertEqual(try db.snapshot().resources[0].id, r.id)
        _ = try db.undoFileMove(receipt, access: .localUser)
        XCTAssertEqual(try Data(contentsOf: from), Data("original content".utf8))
        try FileManager.default.removeItem(at: from)
        XCTAssertEqual(try LocalFileConnector.resolved(db.snapshot().resources[0]).1, .missing)
        try Data("other".utf8).write(to: to)
        let current = try db.snapshot().resources[0]
        let changed = try LocalFileConnector.relink(current, to: to)
        XCTAssertEqual(changed.id, current.id)
        try db.transact { CatalogActions.upsert(changed, in: &$0) }
        XCTAssertEqual(try LocalFileConnector.resolved(db.snapshot().resources[0]).1, .available)
        let deny = CatalogAccess { _ in false }
        XCTAssertThrowsError(try db.moveFile(r.id, to: from, expectedRevision: db.revision(), access: deny))
        XCTAssertTrue(FileManager.default.fileExists(atPath: to.path))
    }
    func testMultiStepHistoryAndBatchRollback() throws {
        let db = try store(), history = CatalogHistory()
        let a = CatalogResource(kind: .card, title: "One", source: "test")
        let b = CatalogResource(kind: .card, title: "Two", source: "test")
        history.record(try db.transact { $0.resources.append(a) })
        history.record(try db.transact { $0.resources.append(b) })
        try history.undo(in: db); XCTAssertEqual(try db.snapshot().resources.map(\.id), [a.id])
        try history.undo(in: db); XCTAssertTrue(try db.snapshot().resources.isEmpty)
        let before = try db.snapshot()
        XCTAssertThrowsError(try db.transact { state in
            state.resources.append(a)
            state.resources.append(CatalogResource(kind: .link, title: "Invalid", locator: "javascript:alert(1)", source: "test"))
        })
        XCTAssertEqual(try db.snapshot(), before)
    }

    func testSavedScopePreventsDestructiveNodeRemovalAndBuilderEscapesOperators() throws {
        let db = try store(); let (root, _, _, _) = try fixture(db)
        try db.transact { $0.searches.append(SavedSearch(name: "Parent", query: CatalogQuery(nodes: [NodeFilter(root.id)]))) }
        XCTAssertThrowsError(try db.transact { try CatalogActions.removeNode(root.id, in: &$0) })
        let built = try BooleanBuilder.group(["budget", "OR"], operation: "OR")
        XCTAssertEqual(built, "(\"budget\" OR \"OR\")")
        XCTAssertThrowsError(try BooleanBuilder.group(["bad\"quote"]))
    }
}

private final class ErrorBox: @unchecked Sendable {
    private let lock = NSLock()
    private var storage: [Error] = []
    var values: [Error] { lock.lock(); defer { lock.unlock() }; return storage }
    func append(_ error: Error) { lock.lock(); defer { lock.unlock() }; storage.append(error) }
}
