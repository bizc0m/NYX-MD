// swift-tools-version:5.9
// NYX-MD — paquet Swift : adaptateur typé au-dessus du moteur Go compilé (CNyxMD).
// Le moteur n'est pas recopié : l'artefact binaire est produit par scripts/build-xcframework.sh.
import PackageDescription

let package = Package(
    name: "NyxMD",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "NyxMD", targets: ["NyxMD"]),
    ],
    targets: [
        .binaryTarget(name: "CNyxMD", path: "artifacts/CNyxMD.xcframework"),
        .target(
            name: "NyxMD",
            dependencies: ["CNyxMD"],
            path: "swift/Sources/NyxMD",
            linkerSettings: [
                // Requis par le runtime Go sur macOS.
                .linkedFramework("CoreFoundation"),
                .linkedFramework("Security"),
            ]
        ),
        .testTarget(name: "NyxMDTests", dependencies: ["NyxMD"], path: "swift/Tests/NyxMDTests"),
    ]
)
