import Foundation
import NyxCatalog
let count = CommandLine.arguments.dropFirst().first.flatMap(Int.init) ?? 10000
let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
defer { try? FileManager.default.removeItem(at: root) }
let store = try CatalogStore(url: root.appendingPathComponent("catalog.sqlite"))
let tag = CatalogNode(kind: .tag, name: "projet")
let start = Date()
try store.transact { state in
    state.nodes = [tag]
    state.resources = (0..<count).map { i in
        CatalogResource(kind: .card, title: "Document \(i)", body: "budget client \(i % 2 == 0 ? "validé" : "attente")", source: "benchmark", nodeIDs: [tag.id])
    }
}
let writeTime = Date().timeIntervalSince(start)
var samples: [Double] = []
var found = 0
for _ in 0..<10 {
    let t = Date()
    found = try store.search(CatalogQuery(boolean: "#projet AND budget AND NOT attente"), access: .localUser).count
    samples.append(Date().timeIntervalSince(t) * 1000)
}
samples.sort()
let sizes = try FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: [.fileSizeKey]).reduce(0) { sum, url in
    sum + ((try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0)
}
print("resources=\(count) matches=\(found) write_s=\(writeTime) median_ms=\(samples[5]) max_ms=\(samples.last!) data_index_bytes=\(sizes)")
