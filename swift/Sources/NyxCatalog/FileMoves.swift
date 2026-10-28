// Explicit physical relocation is isolated from reference classification and has its own undo.
import Foundation

public struct FileMoveReceipt {
    public let resourceID: UUID
    public let from: URL
    public let to: URL
    public let expectedRevision: Int64
}

extension CatalogStore {
    /// No overwrite, no copy, no implicit relocation during drag/drop or classification.
    /// On database failure, compensate the filesystem move; report both paths if compensation fails.
    public func moveFile(_ id: UUID, to destination: URL, expectedRevision: Int64,
                         access: CatalogAccess) throws -> FileMoveReceipt {
        guard destination.isFileURL else { throw CatalogError.invalid("Destination locale requise") }
        var movedFrom: URL?
        do {
            let token = try transact(expectedRevision: expectedRevision) { state in
                guard let i = state.resources.firstIndex(where: { $0.id == id }) else { throw CatalogError.notFound }
                let resource = state.resources[i]
                guard access.permits(resource) else { throw CatalogError.denied }
                let (source, status) = try LocalFileConnector.resolved(resource)
                guard status == .available || status == .stale,
                      source.standardizedFileURL != destination.standardizedFileURL,
                      !FileManager.default.fileExists(atPath: destination.path) else {
                    throw CatalogError.invalid("Source inaccessible ou destination déjà occupée")
                }
                try FileManager.default.moveItem(at: source, to: destination)
                movedFrom = source
                state.resources[i] = try LocalFileConnector.relink(resource, to: destination)
            }
            return FileMoveReceipt(resourceID: id, from: movedFrom!, to: destination, expectedRevision: token.expectedRevision)
        } catch {
            if let source = movedFrom {
                do { try FileManager.default.moveItem(at: destination, to: source) }
                catch { throw CatalogError.invalid("Catalogue non modifié ; déplacement à réconcilier : \(destination.path) → \(source.path) ; \(error.localizedDescription)") }
            }
            throw error
        }
    }
    public func undoFileMove(_ receipt: FileMoveReceipt, access: CatalogAccess) throws -> FileMoveReceipt {
        try moveFile(receipt.resourceID, to: receipt.from, expectedRevision: receipt.expectedRevision, access: access)
    }
}
