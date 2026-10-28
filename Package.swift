// swift-tools-version:5.9
// NYX-MD — paquet Swift : adaptateur typé au-dessus du moteur Go compilé (CNyxMD).
// Le moteur n'est pas recopié : l'artefact binaire est produit par scripts/build-xcframework.sh.
import PackageDescription

#if os(Linux)
let engineTarget: Target = .systemLibrary(name: "CNyxMD", path: "dev/CNyxMD")
let engineLinker: [LinkerSetting] = [.linkedLibrary("nyxmd")]
#else
let engineTarget: Target = .binaryTarget(name: "CNyxMD", path: "artifacts/CNyxMD.xcframework")
let engineLinker: [LinkerSetting] = [.linkedFramework("CoreFoundation"), .linkedFramework("Security")]
#endif

let package = Package(
    name: "NyxMD",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "NyxMD", targets: ["NyxMD"]),
        .library(name: "NyxCatalog", targets: ["NyxCatalog"]),
        .library(name: "NyxExplorerUI", targets: ["NyxExplorerUI"]),
        .executable(name: "NyxExplorerDemo", targets: ["NyxExplorerDemo"]),
        .executable(name: "NyxCatalogBenchmark", targets: ["NyxCatalogBenchmark"]),
        .executable(name: "NyxEngineBaseline", targets: ["NyxEngineBaseline"]),
    ],
    targets: [
        engineTarget,
        .target(
            name: "NyxMD",
            dependencies: ["CNyxMD"],
            path: "swift/Sources/NyxMD",
            linkerSettings: engineLinker
        ),
        .systemLibrary(name: "CSQLite", path: "swift/CSQLite", pkgConfig: "sqlite3"),
        .target(name: "NyxCatalog", dependencies: ["NyxMD", "CSQLite"], path: "swift/Sources/NyxCatalog"),
        .target(name: "NyxExplorerUI", dependencies: ["NyxCatalog"], path: "swift/Sources/NyxExplorerUI"),
        .executableTarget(name: "NyxExplorerDemo", dependencies: ["NyxExplorerUI"], path: "swift/Examples/Explorer"),
        .executableTarget(name: "NyxEngineBaseline", dependencies: ["NyxMD"], path: "swift/Examples/Baseline"),
        .executableTarget(name: "NyxCatalogBenchmark", dependencies: ["NyxCatalog"], path: "swift/Examples/Benchmark"),
        .testTarget(name: "NyxCatalogTests", dependencies: ["NyxCatalog"], path: "swift/Tests/NyxCatalogTests"),
        .testTarget(name: "NyxExplorerUITests", dependencies: ["NyxExplorerUI"], path: "swift/Tests/NyxExplorerUITests"),
        .testTarget(name: "NyxMDTests", dependencies: ["NyxMD"], path: "swift/Tests/NyxMDTests"),
    ]
)
