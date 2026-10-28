#if canImport(AppKit)
import AppKit
import NyxCatalog
import NyxExplorerUI
final class DemoDelegate: NSObject, NSApplicationDelegate {
    var explorerWindow: NyxExplorerWindow?
    func applicationDidFinishLaunching(_ notification: Notification) {
        do {
            // Isolated from the real suite catalogue unless an explicit path is supplied.
            let path = ProcessInfo.processInfo.environment["NYX_CATALOG_PATH"] ??
                FileManager.default.temporaryDirectory.appendingPathComponent("nyx-explorer-demo/catalog.sqlite").path
            let store = try CatalogStore(url: URL(fileURLWithPath: path))
            explorerWindow = NyxExplorerWindow(store: store, source: "demo", access: .localUser)
            explorerWindow?.showWindow(nil); NSApp.activate(ignoringOtherApps: true)
        } catch { NSApp.presentError(error); NSApp.terminate(nil) }
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = DemoDelegate(); app.delegate = delegate
let main = NSMenu(), appItem = NSMenuItem(), menu = NSMenu()
menu.addItem(withTitle: "Quitter", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
appItem.submenu = menu; main.addItem(appItem); app.mainMenu = main
app.run()
#else
print("NyxExplorerUI requires macOS/AppKit; NyxCatalog and NyxMD are testable on Linux")
#endif
