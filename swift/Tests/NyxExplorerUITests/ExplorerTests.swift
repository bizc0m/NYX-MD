#if canImport(AppKit)
import AppKit
import XCTest
import NyxCatalog
import NyxExplorerUI
final class ExplorerTests: XCTestCase {
    func testSingleDualPaneAndIndependentQueries() throws {
        try MainActor.assumeIsolated {
            _ = NSApplication.shared
            let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
            defer { try? FileManager.default.removeItem(at: root) }
            let store = try CatalogStore(url: root.appendingPathComponent("catalog.sqlite"))
            let a = CatalogResource(kind: .card, title: "Alpha", source: "test")
            let b = CatalogResource(kind: .card, title: "Beta", source: "test")
            try store.transact { $0.resources = [a, b] }
            let controller = NyxExplorerController(store: store, source: "test", access: .localUser)
            _ = controller.view
            XCTAssertEqual(controller.paneCount, 1)
            controller.toggleDualPane(); XCTAssertEqual(controller.paneCount, 2)
            controller.setQuery(CatalogQuery(boolean: "Alpha"), pane: 0)
            controller.setQuery(CatalogQuery(boolean: "Beta"), pane: 1)
            XCTAssertEqual(controller.visibleResourceIDs(pane: 0), [a.id])
            XCTAssertEqual(controller.visibleResourceIDs(pane: 1), [b.id])
            controller.setQuery(CatalogQuery(boolean: "AND"), pane: 0)
            XCTAssertTrue(controller.visibleResourceIDs(pane: 0).isEmpty)
            try store.transact { state in state.resources[1].title = "Gamma" }
            controller.refresh()
            XCTAssertTrue(controller.visibleResourceIDs(pane: 1).isEmpty, "other pane refreshes despite invalid query")
            controller.setQuery(CatalogQuery(boolean: "Gamma"), pane: 1)
            XCTAssertEqual(controller.visibleResourceIDs(pane: 1), [b.id])
            controller.toggleDualPane(); XCTAssertEqual(controller.paneCount, 1)
        }
    }
}
#endif
