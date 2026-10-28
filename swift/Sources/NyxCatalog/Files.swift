// NyxCatalog 0.1 — explicit local file connector; no implicit content indexing or network fetch.
import Foundation

public enum LocalFileConnector {
    private static func identity(_ url: URL) throws -> String? {
        let a = try FileManager.default.attributesOfItem(atPath: url.path)
        guard let device = a[.systemNumber] as? NSNumber, let inode = a[.systemFileNumber] as? NSNumber else { return nil }
        return "\(device):\(inode)"
    }
    public static func reference(_ url: URL, source: String) throws -> CatalogResource {
        guard url.isFileURL else { throw CatalogError.invalid("Fichier local requis") }
        guard (try? url.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) != true else {
            throw CatalogError.invalid("Sélectionnez des fichiers ; les dossiers de classement sont virtuels")
        }
        var bookmark: Data?
        #if os(macOS)
        bookmark = try url.bookmarkData(options: [], includingResourceValuesForKeys: nil, relativeTo: nil)
        #endif
        var r = CatalogResource(kind: .file, title: url.lastPathComponent, locator: url.absoluteString,
                                source: source, bookmark: bookmark)
        r.metadata["nyx.fileIdentity"] = try? identity(url)
        return r
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
        do { _ = try FileManager.default.attributesOfItem(atPath: url.path) }
        catch {
            let e = error as NSError
            let missing = (e.domain == NSCocoaErrorDomain && [NSFileNoSuchFileError, NSFileReadNoSuchFileError].contains(e.code)) ||
                (e.domain == NSPOSIXErrorDomain && e.code == 2)
            return (url, missing ? .missing : .denied)
        }
        guard FileManager.default.isReadableFile(atPath: url.path) else { return (url, .denied) }
        if let recorded = resource.metadata["nyx.fileIdentity"], let current = try? identity(url), current != recorded { return (url, .replaced) }
        return (url, stale || url.resolvingSymlinksInPath().standardizedFileURL != original.resolvingSymlinksInPath().standardizedFileURL ? .stale : .available)
    }
    /// Relink after an external move; stable catalogue identity and classification are preserved.
    public static func relink(_ resource: CatalogResource, to url: URL) throws -> CatalogResource {
        var result = resource
        let new = try reference(url, source: resource.source)
        result.locator = new.locator; result.bookmark = new.bookmark; result.metadata["nyx.fileIdentity"] = new.metadata["nyx.fileIdentity"]; result.updatedAt = Date()
        return result
    }
}
