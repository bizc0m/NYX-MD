// NyxCatalog 0.1 — explicit local file connector; no implicit content indexing or network fetch.
import Foundation

public enum LocalFileConnector {
    public static func reference(_ url: URL, source: String) throws -> CatalogResource {
        guard url.isFileURL else { throw CatalogError.invalid("Fichier local requis") }
        var bookmark: Data?
        #if os(macOS)
        bookmark = try url.bookmarkData(options: [], includingResourceValuesForKeys: nil, relativeTo: nil)
        #endif
        return CatalogResource(kind: .file, title: url.lastPathComponent, locator: url.absoluteString,
                               source: source, bookmark: bookmark)
    }
    public static func resolved(_ resource: CatalogResource) throws -> (URL, ReferenceStatus) {
        guard resource.kind == .file, let original = URL(string: resource.locator), original.isFileURL else {
            throw CatalogError.invalid("Référence fichier requise")
        }
        var url = original, stale = false
        #if os(macOS)
        if let bookmark = resource.bookmark {
            do {
                url = try URL(resolvingBookmarkData: bookmark, options: [.withoutUI, .withoutMounting],
                              relativeTo: nil, bookmarkDataIsStale: &stale)
            } catch { stale = true }
        }
        #endif
        guard FileManager.default.fileExists(atPath: url.path) else { return (url, .missing) }
        guard FileManager.default.isReadableFile(atPath: url.path) else { return (url, .denied) }
        return (url, stale || url.resolvingSymlinksInPath().standardizedFileURL != original.resolvingSymlinksInPath().standardizedFileURL ? .stale : .available)
    }
    /// Relink after an external move; stable catalogue identity and classification are preserved.
    public static func relink(_ resource: CatalogResource, to url: URL) throws -> CatalogResource {
        var result = resource
        let new = try reference(url, source: resource.source)
        result.locator = new.locator; result.bookmark = new.bookmark; result.updatedAt = Date()
        return result
    }
}
