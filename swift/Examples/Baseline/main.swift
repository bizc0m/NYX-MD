import NyxMD
#if canImport(AppKit)
import AppKit
_ = NSApplication.shared
let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 100, height: 100), styleMask: [.titled], backing: .buffered, defer: false)
window.title = "Baseline"
#endif
let q = try NyxQuery(CommandLine.arguments.dropFirst().first ?? "baseline")
print(try q.match([NyxDocument(text: "baseline")]))
