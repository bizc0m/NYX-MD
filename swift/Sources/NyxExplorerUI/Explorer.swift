// NyxExplorerUI 0.1 — 2026-10-28 — reusable AppKit explorer, isolated from host preferences.
#if canImport(AppKit)
import AppKit
import NyxCatalog

fileprivate final class ExplorerTable: NSTableView {
    var enter: (() -> Void)?, parent: (() -> Void)?, other: (() -> Void)?, edit: (() -> Void)?, undoAction: (() -> Void)?
    override func keyDown(with event: NSEvent) {
        if event.modifierFlags.contains(.command), event.charactersIgnoringModifiers == "z" { undoAction?(); return }
        switch event.keyCode {
        case 36: enter?()
        case 51: parent?()
        case 48: other?()
        case 118: edit?() // F4
        default: super.keyDown(with: event)
        }
    }
}

/// One controller can be embedded in any AppKit host; window helper is optional.
/// Host open handlers receive only resources rechecked against the access policy.
public final class NyxExplorerController: NSViewController {
    public let store: CatalogStore
    public let source: String
    public let access: CatalogAccess
    public var onOpen: ((CatalogResource) -> Void)?
    public private(set) var isDualPane = false
    private let split = NSSplitView()
    private var panes: [ExplorerPane] = []
    private var status = NSTextField(labelWithString: "")
    private var timer: Timer?
    private var seenRevision: Int64 = -1
    private let history = CatalogHistory()
    private var fileUndo: FileMoveReceipt?
    private var dualButton = NSButton()
    fileprivate var active: ExplorerPane? { panes.first(where: { $0.isActive }) ?? panes.first }

    public init(store: CatalogStore, source: String, access: CatalogAccess, onOpen: ((CatalogResource) -> Void)? = nil) {
        self.store = store; self.source = source; self.access = access; self.onOpen = onOpen
        super.init(nibName: nil, bundle: nil)
    }
    required init?(coder: NSCoder) { fatalError("init(coder:) unavailable") }
    deinit { timer?.invalidate() }
    override public func loadView() {
        view = NSView(frame: NSRect(x: 0, y: 0, width: 1050, height: 650))
        let tools = NSStackView()
        tools.orientation = .horizontal; tools.spacing = 7
        func add(_ title: String, _ action: Selector, _ key: String = "") -> NSButton {
            let b = NSButton(title: title, target: self, action: action); b.keyEquivalent = key
            b.setAccessibilityLabel(title); tools.addArrangedSubview(b); return b
        }
        _ = add("+ Fiche / lien", #selector(createResource))
        _ = add("+ Fichiers", #selector(importFiles))
        _ = add("+ Classement", #selector(createNode))
        _ = add("Modifier", #selector(editSelection))
        _ = add("Classer…", #selector(classifySelection))
        _ = add("Déplacer réf…", #selector(moveReferences))
        _ = add("Annuler", #selector(undoLast))
        dualButton = add("Deux panneaux (F3)", #selector(toggleDualPane), "\u{F706}")
        let more = NSPopUpButton()
        more.addItems(withTitles: ["Actions…", "Retirer les fiches…", "Renommer / déplacer le classement…", "Retirer le classement…",
                                  "Réassocier le fichier…", "Déplacer le fichier réel…", "Sauvegarder…", "Restaurer…", "Exporter les ressources visibles…", "Retirer un classement des fiches…", "Supprimer la collection sélectionnée…"])
        more.target = self; more.action = #selector(moreAction(_:)); tools.addArrangedSubview(more)
        split.isVertical = true; split.dividerStyle = .thin
        for _ in 0..<2 {
            let pane = ExplorerPane(owner: self)
            addChild(pane); split.addArrangedSubview(pane.view); panes.append(pane)
        }
        panes[1].view.isHidden = true
        status.textColor = .secondaryLabelColor; status.font = .systemFont(ofSize: 11)
        let stack = NSStackView(views: [tools, split, status]); stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 8
        stack.translatesAutoresizingMaskIntoConstraints = false; view.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: view.leadingAnchor, constant: 10),
            stack.trailingAnchor.constraint(equalTo: view.trailingAnchor, constant: -10),
            stack.topAnchor.constraint(equalTo: view.topAnchor, constant: 10),
            stack.bottomAnchor.constraint(equalTo: view.bottomAnchor, constant: -10),
            split.widthAnchor.constraint(equalTo: stack.widthAnchor), split.heightAnchor.constraint(greaterThanOrEqualToConstant: 250)
        ])
        refresh()
        // data_version/revision polling also covers writers in other processes, without global defaults.
        timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            guard let self, self.view.window?.isVisible == true else { return }
            do { if try self.store.revision() != self.seenRevision { self.refresh() } }
            catch { self.report(error) }
        }
    }
    public func refresh() {
        do {
            let state = try store.snapshot(); seenRevision = state.revision
            try panes.forEach { try $0.reload(state) }
            status.stringValue = "Catalogue partagé · révision \(state.revision) · classement des références sans copie des fichiers"
        } catch { report(error) }
    }
    @objc public func toggleDualPane() {
        isDualPane.toggle(); panes[1].view.isHidden = !isDualPane
        dualButton.title = isDualPane ? "Un panneau (F3)" : "Deux panneaux (F3)"
        if !isDualPane { view.window?.makeFirstResponder(panes[0].table) }
        split.adjustSubviews()
    }
    fileprivate func focusOther(_ sender: ExplorerPane) {
        guard isDualPane, let pane = panes.first(where: { $0 !== sender }) else { return }
        view.window?.makeFirstResponder(pane.table)
    }
    fileprivate func report(_ error: Error) { status.stringValue = error.localizedDescription; NSSound.beep() }
    fileprivate func perform(_ operation: (inout CatalogSnapshot) throws -> Void) {
        do {
            history.record(try store.transact(expectedRevision: seenRevision, operation)); fileUndo = nil
            refresh()
        } catch { refresh(); report(error) }
    }
    @objc public func undoLast() {
        do {
            if let receipt = fileUndo { _ = try store.undoFileMove(receipt, access: access); fileUndo = nil }
            else { try history.undo(in: store) }
            refresh()
        } catch { report(error) }
    }
    private func ask(_ title: String, fields: [(String, String)], options: [String]? = nil) -> ([String], Int)? {
        let a = NSAlert(); a.messageText = title
        a.addButton(withTitle: "Valider"); a.addButton(withTitle: "Annuler")
        let inputs = fields.map { pair -> NSTextField in
            let f = NSTextField(string: pair.1); f.placeholderString = pair.0; f.setAccessibilityLabel(pair.0); return f
        }
        let stack = NSStackView(); stack.orientation = .vertical; stack.alignment = .leading
        stack.frame = NSRect(x: 0, y: 0, width: 440, height: CGFloat(inputs.count * 34 + (options == nil ? 0 : 34)))
        for (i, f) in inputs.enumerated() {
            let label = NSTextField(labelWithString: fields[i].0); label.font = .systemFont(ofSize: 11)
            let row = NSStackView(views: [label, f]); row.orientation = .vertical; row.alignment = .leading
            f.widthAnchor.constraint(equalToConstant: 430).isActive = true; stack.addArrangedSubview(row)
        }
        let popup = NSPopUpButton()
        if let options { popup.addItems(withTitles: options); stack.addArrangedSubview(popup) }
        a.accessoryView = stack; a.window.initialFirstResponder = inputs.first
        guard a.runModal() == .alertFirstButtonReturn else { return nil }
        return (inputs.map(\.stringValue), popup.indexOfSelectedItem)
    }
    private func confirm(_ title: String) -> Bool {
        let a = NSAlert(); a.messageText = title; a.addButton(withTitle: "Annuler"); a.addButton(withTitle: "Confirmer")
        return a.runModal() == .alertSecondButtonReturn
    }
    @objc public func createResource() {
        guard let values = ask("Nouvelle ressource", fields: [("Titre", ""), ("URL HTTP(S), vide pour une fiche", ""), ("Texte", "")])?.0 else { return }
        var r = CatalogResource(kind: values[1].isEmpty ? .card : .link, title: values[0], locator: values[1], body: values[2], source: source)
        if let scope = active?.scope { r.nodeIDs.insert(scope) }
        perform { CatalogActions.upsert(r, in: &$0) }
    }
    @objc public func importFiles() {
        let panel = NSOpenPanel(); panel.allowsMultipleSelection = true; panel.canChooseDirectories = false
        guard panel.runModal() == .OK else { return }
        importURLs(panel.urls, into: active?.scope)
    }
    fileprivate func importURLs(_ urls: [URL], into scope: UUID?) {
        do {
            let values = try urls.map { url -> CatalogResource in
                var r: CatalogResource
                if url.isFileURL { r = try LocalFileConnector.reference(url, source: source) }
                else { r = CatalogResource(kind: .link, title: url.absoluteString, locator: url.absoluteString, source: source) }
                if let scope { r.nodeIDs.insert(scope) }; return r
            }
            perform { state in for r in values { CatalogActions.upsert(r, in: &state) } }
        } catch { report(error) }
    }
    @objc public func createNode() {
        guard let result = ask("Nouveau classement", fields: [("Nom (un niveau)", "")], options: ["Dossier", "#Tag", "@Contexte"]) else { return }
        let kind = NodeKind.allCases[result.1]
        let parent = active?.state.nodes.first(where: { $0.id == active?.scope && $0.kind == kind })?.id
        perform { $0.nodes.append(CatalogNode(kind: kind, name: result.0[0], parentID: parent)) }
    }
    @objc public func editSelection() {
        guard let r = active?.selectedResources.first, active?.selectedResources.count == 1 else { return }
        guard let values = ask("Modifier la fiche", fields: [("Titre", r.title), ("URL / référence", r.locator), ("Texte", r.body)])?.0 else { return }
        var changed = r; changed.title = values[0]; changed.body = values[2]
        // File references must be changed via relink, not a freeform URL field.
        if r.kind != .file { changed.locator = values[1] }
        perform { CatalogActions.upsert(changed, in: &$0) }
    }
    @objc public func classifySelection() {
        guard let pane = active, !pane.selectedResources.isEmpty else { return }
        let nodes = pane.state.nodes.sorted { pane.state.path(of: $0.id) < pane.state.path(of: $1.id) }
        guard !nodes.isEmpty, let result = ask("Classer les références sélectionnées", fields: [],
                                               options: nodes.map { "\($0.kind.rawValue) : \(pane.state.path(of: $0.id))" }) else { return }
        let ids = Set(pane.selectedResources.map(\.id)), target = nodes[result.1]
        perform { try CatalogActions.classify(ids, into: target.id, in: &$0) }
    }
    @objc public func moveReferences() {
        guard let pane = active, let from = pane.scope,
              pane.state.nodes.contains(where: { $0.id == from && $0.kind == .folder }),
              !pane.selectedResources.isEmpty else { report(CatalogError.invalid("Sélectionnez les références dans leur dossier source")); return }
        let folders = pane.state.nodes.filter { $0.kind == .folder && $0.id != from }
        guard !folders.isEmpty, let result = ask("Déplacer les références (fichiers réels inchangés)", fields: [],
                                                options: folders.map { pane.state.path(of: $0.id) }) else { return }
        let ids = Set(pane.selectedResources.map(\.id)), target = folders[result.1].id
        perform { try CatalogActions.classify(ids, into: target, replacingFolder: from, in: &$0) }
    }
    fileprivate func open(_ resource: CatalogResource) {
        guard access.permits(resource) else { report(CatalogError.denied); return }
        if resource.kind == .file {
            do {
                let (url, status) = try LocalFileConnector.resolved(resource)
                guard status == .available || status == .stale else { throw CatalogError.invalid("Fichier \(status.rawValue) : réassociez la référence") }
                guard FileManager.default.isReadableFile(atPath: url.path) else { throw CatalogError.denied }
                var r = resource; r.locator = url.absoluteString
                if let onOpen { onOpen(r) } else { NSWorkspace.shared.open(url) }
            } catch { report(error) }
        } else if resource.kind == .link, let url = URL(string: resource.locator) {
            if let onOpen { onOpen(resource) } else { NSWorkspace.shared.open(url) }
        } else { editSelection() }
    }
    @objc private func moreAction(_ sender: NSPopUpButton) {
        let action = sender.indexOfSelectedItem; defer { sender.selectItem(at: 0) }
        guard let pane = active else { return }
        switch action {
        case 1:
            let ids = Set(pane.selectedResources.map(\.id))
            guard !ids.isEmpty, confirm("Retirer \(ids.count) fiche(s) du catalogue ? Les fichiers réels sont conservés") else { return }
            perform { $0.resources.removeAll { ids.contains($0.id) } }
        case 2:
            guard let id = pane.scope, let n = pane.state.nodes.first(where: { $0.id == id }) else { return }
            let parents = pane.state.nodes.filter { $0.kind == n.kind && !pane.state.descendants(of: id).contains($0.id) }
            guard let v = ask("Renommer / déplacer le classement", fields: [("Nom", n.name)], options: ["Racine"] + parents.map { pane.state.path(of: $0.id) }) else { return }
            perform { state in
                guard let i = state.nodes.firstIndex(where: { $0.id == id }) else { throw CatalogError.notFound }
                state.nodes[i].name = v.0[0]; state.nodes[i].parentID = v.1 == 0 ? nil : parents[v.1 - 1].id
            }
        case 3:
            guard let id = pane.scope, confirm("Retirer ce classement et ses sous-niveaux ? Les fiches sont conservées") else { return }
            perform { try CatalogActions.removeNode(id, in: &$0) }
        case 4:
            guard let r = pane.selectedResources.first, r.kind == .file else { return }
            let p = NSOpenPanel(); guard p.runModal() == .OK, let url = p.url else { return }
            do { let changed = try LocalFileConnector.relink(r, to: url); perform { CatalogActions.upsert(changed, in: &$0) } }
            catch { report(error) }
        case 5:
            guard let r = pane.selectedResources.first, pane.selectedResources.count == 1, r.kind == .file else { return }
            let p = NSSavePanel(); p.nameFieldStringValue = r.title
            guard p.runModal() == .OK, let destination = p.url,
                  confirm("Déplacer le fichier réel vers \(destination.path) ? Aucun fichier existant ne sera remplacé") else { return }
            do {
                fileUndo = try store.moveFile(r.id, to: destination, expectedRevision: seenRevision, access: access)
                history.clear(); refresh()
            } catch { report(error) }
        case 6, 8:
            let p = NSSavePanel(); p.nameFieldStringValue = action == 6 ? "nyx-backup.json" : "nyx-export.json"
            guard p.runModal() == .OK, let url = p.url else { return }
            do { if action == 6 { try store.backup(to: url) } else { try store.export(to: url, access: access) } }
            catch { report(error) }
        case 7:
            let p = NSOpenPanel(); guard p.runModal() == .OK, let url = p.url, confirm("Remplacer le catalogue par cette sauvegarde ?") else { return }
            do { history.record(try store.restore(from: url, expectedRevision: seenRevision)); fileUndo = nil; refresh() }
            catch { report(error) }
        case 9:
            let selected = pane.selectedResources
            let memberships = selected.reduce(into: Set<UUID>()) { $0.formUnion($1.nodeIDs) }
            let nodes = pane.state.nodes.filter { memberships.contains($0.id) }
            guard !selected.isEmpty, !nodes.isEmpty, let result = ask("Retirer une association des fiches sélectionnées", fields: [],
                                                                     options: nodes.map { pane.state.path(of: $0.id) }) else { return }
            let ids = Set(selected.map(\.id)), node = nodes[result.1].id
            perform { state in for i in state.resources.indices where ids.contains(state.resources[i].id) { state.resources[i].nodeIDs.remove(node) } }
        case 10:
            guard let id = pane.currentSavedID, confirm("Supprimer cette collection enregistrée ? Les ressources sont conservées") else { return }
            perform { $0.searches.removeAll { $0.id == id } }
        default: break
        }
    }
    // Stable test surface: no host defaults or installed app required.
    public var paneCount: Int { isDualPane ? 2 : 1 }
    public func setQuery(_ query: CatalogQuery, pane index: Int) {
        _ = view; guard panes.indices.contains(index) else { return }; panes[index].scope = nil; panes[index].query = query
        panes[index].search.stringValue = query.boolean; refresh()
    }
    public func visibleResourceIDs(pane index: Int) -> [UUID] { panes.indices.contains(index) ? panes[index].resources.map(\.id) : [] }
}

private struct DragPayload: Codable { var catalog: String; var ids: [UUID]; var folder: UUID? }

fileprivate final class ExplorerPane: NSViewController, NSTableViewDataSource, NSTableViewDelegate, NSSearchFieldDelegate {
    unowned let owner: NyxExplorerController
    let table = ExplorerTable()
    let search = NSSearchField()
    private let scopes = NSPopUpButton()
    private let saved = NSPopUpButton()
    private let descendants = NSButton(checkboxWithTitle: "Inclure les descendants", target: nil, action: nil)
    private let count = NSTextField(labelWithString: "")
    private let kind = NSPopUpButton()
    private var scopeChoices: [CatalogNode] = []
    var scope: UUID?
    var state = CatalogSnapshot()
    var query = CatalogQuery()
    var currentSavedID: UUID?
    var resources: [CatalogResource] = []
    private var childNodes: [CatalogNode] = []
    private static let dragType = NSPasteboard.PasteboardType("org.nyx-suite.catalog.resources.v1")
    var isActive: Bool { view.window?.firstResponder === table || view.window?.firstResponder === search.currentEditor() }
    var selectedResources: [CatalogResource] {
        table.selectedRowIndexes.compactMap { row in let index = row - childNodes.count; return resources.indices.contains(index) ? resources[index] : nil }
    }
    init(owner: NyxExplorerController) { self.owner = owner; super.init(nibName: nil, bundle: nil) }
    required init?(coder: NSCoder) { fatalError("init(coder:) unavailable") }
    override func loadView() {
        view = NSView()
        scopes.target = self; scopes.action = #selector(scopeChanged)
        scopes.setAccessibilityLabel("Dossier, tag ou contexte")
        let up = NSButton(title: "↑", target: self, action: #selector(goUp)); up.setAccessibilityLabel("Niveau parent")
        let navigation = NSStackView(views: [up, scopes]); navigation.orientation = .horizontal
        search.placeholderString = "Recherche NyxBoolean"; search.delegate = self
        search.setAccessibilityLabel("Recherche booléenne dans ce panneau")
        descendants.state = .off; descendants.target = self; descendants.action = #selector(filtersChanged)
        kind.addItems(withTitles: ["Tous les types", "Liens", "Fichiers", "Fiches"]); kind.target = self; kind.action = #selector(filtersChanged)
        let filters = NSStackView(views: [kind, descendants]); filters.orientation = .horizontal
        saved.target = self; saved.action = #selector(savedChanged)
        let save = NSButton(title: "Enregistrer", target: self, action: #selector(saveSearch))
        let build = NSButton(title: "Construire…", target: self, action: #selector(buildQuery))
        let searchTools = NSStackView(views: [saved, save, build]); searchTools.orientation = .horizontal
        let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("title")); column.title = "Ressources / sous-niveaux"
        column.width = 420; table.addTableColumn(column); table.headerView = nil
        table.allowsMultipleSelection = true; table.dataSource = self; table.delegate = self
        table.target = self; table.doubleAction = #selector(openSelection)
        table.usesAlternatingRowBackgroundColors = true
        table.setAccessibilityLabel("Ressources du panneau")
        table.registerForDraggedTypes([Self.dragType, .fileURL, .URL]); table.setDraggingSourceOperationMask([.copy, .move], forLocal: true)
        table.enter = { [weak self] in self?.openSelection() }; table.parent = { [weak self] in self?.goUp() }
        table.edit = { [weak self] in self?.owner.editSelection() }; table.other = { [weak self] in guard let self else { return }; self.owner.focusOther(self) }
        table.undoAction = { [weak self] in self?.owner.undoLast() }
        let scroll = NSScrollView(); scroll.documentView = table; scroll.hasVerticalScroller = true
        let sort = NSPopUpButton(); sort.addItems(withTitles: ["Titre A–Z", "Modification récente", "Type"])
        sort.target = self; sort.action = #selector(sortChanged(_:))
        let footer = NSStackView(views: [sort, count]); footer.orientation = .horizontal
        let stack = NSStackView(views: [navigation, search, filters, searchTools, scroll, footer])
        stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 6
        stack.translatesAutoresizingMaskIntoConstraints = false; view.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: view.leadingAnchor), stack.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            stack.topAnchor.constraint(equalTo: view.topAnchor), stack.bottomAnchor.constraint(equalTo: view.bottomAnchor),
            scopes.widthAnchor.constraint(greaterThanOrEqualToConstant: 160),
            search.widthAnchor.constraint(equalTo: stack.widthAnchor), scroll.widthAnchor.constraint(equalTo: stack.widthAnchor),
            scroll.heightAnchor.constraint(greaterThanOrEqualToConstant: 150), view.widthAnchor.constraint(greaterThanOrEqualToConstant: 360)
        ])
    }
    func reload(_ state: CatalogSnapshot) throws {
        _ = view; self.state = state
        let selection = Set(selectedResources.map(\.id))
        scopeChoices = state.nodes.sorted { state.path(of: $0.id).localizedStandardCompare(state.path(of: $1.id)) == .orderedAscending }
        if let scope, !state.nodes.contains(where: { $0.id == scope }) { self.scope = nil }
        scopes.removeAllItems(); scopes.addItem(withTitle: "Toutes les ressources")
        scopes.addItems(withTitles: scopeChoices.map { "\($0.kind == .tag ? "#" : $0.kind == .context ? "@" : "📁")\(state.path(of: $0.id))" })
        scopes.selectItem(at: scope.flatMap { id in scopeChoices.firstIndex(where: { $0.id == id }).map { $0 + 1 } } ?? 0)
        saved.removeAllItems(); saved.addItem(withTitle: "Collections dynamiques…"); saved.addItems(withTitles: state.searches.map(\.name))
        if let id = currentSavedID, let i = state.searches.firstIndex(where: { $0.id == id }) { saved.selectItem(at: i + 1) }
        query.boolean = search.stringValue
        query.nodes = scope.map { [NodeFilter($0, match: descendants.state == .on ? .descendants : .exact)] } ?? query.nodes
        do { resources = try owner.store.search(query, access: owner.access) }
        catch { resources = []; childNodes = []; table.reloadData(); count.stringValue = error.localizedDescription; throw error }
        childNodes = query.boolean.isEmpty && query.kinds.isEmpty ? state.nodes.filter { $0.parentID == scope && (scope != nil || $0.parentID == nil) }.sorted { $0.name < $1.name } : []
        sortResources(); table.reloadData()
        table.selectRowIndexes(IndexSet(resources.enumerated().compactMap { selection.contains($0.element.id) ? $0.offset + childNodes.count : nil }), byExtendingSelection: false)
        count.stringValue = "\(resources.count) ressource(s)"
    }
    private var sortMode = 0
    private func sortResources() {
        if sortMode == 1 { resources.sort { $0.updatedAt > $1.updatedAt } }
        else if sortMode == 2 { resources.sort { $0.kind.rawValue == $1.kind.rawValue ? $0.title < $1.title : $0.kind.rawValue < $1.kind.rawValue } }
    }
    @objc private func sortChanged(_ sender: NSPopUpButton) { sortMode = sender.indexOfSelectedItem; sortResources(); table.reloadData() }
    @objc private func scopeChanged() {
        scope = scopes.indexOfSelectedItem > 0 ? scopeChoices[scopes.indexOfSelectedItem - 1].id : nil
        query.nodes = []; currentSavedID = nil; owner.refresh()
    }
    @objc func goUp() {
        scope = scope.flatMap { id in state.nodes.first(where: { $0.id == id })?.parentID }; query.nodes = []; owner.refresh()
    }
    @objc private func filtersChanged() {
        query.kinds = kind.indexOfSelectedItem == 0 ? [] : [ResourceKind.allCases[kind.indexOfSelectedItem - 1]]
        query.metadataMatch = descendants.state == .on ? .descendants : .exact; owner.refresh()
    }
    func controlTextDidChange(_ obj: Notification) { owner.refresh() }
    @objc func openSelection() {
        let row = table.clickedRow >= 0 ? table.clickedRow : table.selectedRow
        if childNodes.indices.contains(row) { scope = childNodes[row].id; query.nodes = []; owner.refresh() }
        else { for r in selectedResources { owner.open(r) } }
    }
    @objc private func saveSearch() {
        let a = NSAlert(); a.messageText = "Nom de la collection dynamique"
        let field = NSTextField(frame: NSRect(x: 0, y: 0, width: 300, height: 24)); a.accessoryView = field
        a.addButton(withTitle: "Enregistrer"); a.addButton(withTitle: "Annuler")
        guard a.runModal() == .alertFirstButtonReturn, !field.stringValue.isEmpty else { return }
        owner.perform { state in
            if let i = state.searches.firstIndex(where: { $0.name == field.stringValue }) { state.searches[i].query = self.query }
            else { state.searches.append(SavedSearch(name: field.stringValue, query: self.query)) }
        }
    }
    @objc private func savedChanged() {
        let i = saved.indexOfSelectedItem - 1; guard state.searches.indices.contains(i) else { return }
        currentSavedID = state.searches[i].id; query = state.searches[i].query; scope = nil; search.stringValue = query.boolean
        descendants.state = query.metadataMatch == .descendants ? .on : .off
        kind.selectItem(at: query.kinds.count == 1 ? ResourceKind.allCases.firstIndex(of: query.kinds.first!)! + 1 : 0)
        owner.refresh()
    }
    @objc private func buildQuery() {
        let a = NSAlert(); a.messageText = "Un terme ou une expression par ligne"
        let input = NSTextView(frame: NSRect(x: 0, y: 0, width: 360, height: 100)); input.isRichText = false
        let op = NSPopUpButton(); op.addItems(withTitles: ["ET (AND)", "OU (OR)", "EXCLURE (NOT AND)"])
        let stack = NSStackView(views: [input, op]); stack.orientation = .vertical; stack.frame = NSRect(x: 0, y: 0, width: 360, height: 140)
        a.accessoryView = stack; a.addButton(withTitle: "Appliquer"); a.addButton(withTitle: "Annuler")
        guard a.runModal() == .alertFirstButtonReturn else { return }
        do {
            search.stringValue = try BooleanBuilder.group(input.string.components(separatedBy: .newlines).filter { !$0.isEmpty },
                                                          operation: op.indexOfSelectedItem == 1 ? "OR" : "AND", negated: op.indexOfSelectedItem == 2)
            owner.refresh()
        } catch { owner.report(error) }
    }
    func numberOfRows(in tableView: NSTableView) -> Int { childNodes.count + resources.count }
    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        let text: String
        if childNodes.indices.contains(row) { let n = childNodes[row]; text = "\(n.kind == .folder ? "📁" : n.kind == .tag ? "#" : "@") \(n.name)" }
        else {
            let i = row - childNodes.count; guard resources.indices.contains(i) else { return nil }
            let r = resources[i]; var suffix = ""
            if r.kind == .file, let status = try? LocalFileConnector.resolved(r).1, status != .available { suffix = " [\(status.rawValue)]" }
            text = "\(r.kind == .file ? "▤" : r.kind == .link ? "↗" : "≡") \(r.title)\(suffix)"
        }
        let f = NSTextField(labelWithString: text); f.lineBreakMode = .byTruncatingMiddle; f.toolTip = text; return f
    }
    func tableView(_ tableView: NSTableView, pasteboardWriterForRow row: Int) -> NSPasteboardWriting? {
        let index = row - childNodes.count; guard resources.indices.contains(index) else { return nil }
        let selected = selectedResources
        let ids = selected.contains(where: { $0.id == resources[index].id }) ? selected.map(\.id) : [resources[index].id]
        let folder = state.nodes.first(where: { $0.id == scope && $0.kind == .folder })?.id
        let payload = DragPayload(catalog: owner.store.url.standardizedFileURL.path, ids: ids, folder: folder)
        let item = NSPasteboardItem()
        if let data = try? JSONEncoder().encode(payload) { item.setData(data, forType: Self.dragType) }
        return item
    }
    private func target(row: Int) -> UUID? { childNodes.indices.contains(row) ? childNodes[row].id : scope }
    func tableView(_ tableView: NSTableView, validateDrop info: NSDraggingInfo, proposedRow row: Int, proposedDropOperation operation: NSTableView.DropOperation) -> NSDragOperation {
        if let data = info.draggingPasteboard.data(forType: Self.dragType),
           let p = try? JSONDecoder().decode(DragPayload.self, from: data) {
            guard p.catalog == owner.store.url.standardizedFileURL.path, target(row: row) != nil else { return [] }
            return .copy // Adds membership; never silently removes a different classification.
        }
        return .copy
    }
    func tableView(_ tableView: NSTableView, acceptDrop info: NSDraggingInfo, row: Int, dropOperation: NSTableView.DropOperation) -> Bool {
        let target = target(row: row)
        if let data = info.draggingPasteboard.data(forType: Self.dragType), let p = try? JSONDecoder().decode(DragPayload.self, from: data) {
            guard p.catalog == owner.store.url.standardizedFileURL.path, let target else { return false }
            let ids = Set(p.ids)
            guard ids.isSubset(of: Set(state.resources.filter(owner.access.permits).map(\.id))) else { return false }
            owner.perform { try CatalogActions.classify(ids, into: target, in: &$0) }; return true
        }
        let urls = info.draggingPasteboard.readObjects(forClasses: [NSURL.self], options: nil) as? [URL] ?? []
        guard !urls.isEmpty else { return false }; owner.importURLs(urls, into: target); return true
    }
}

public final class NyxExplorerWindow: NSWindowController {
    public let explorer: NyxExplorerController
    public init(store: CatalogStore, source: String, access: CatalogAccess, onOpen: ((CatalogResource) -> Void)? = nil) {
        explorer = NyxExplorerController(store: store, source: source, access: access, onOpen: onOpen)
        let window = NSWindow(contentViewController: explorer)
        window.title = "Explorateur NYX — liens, fichiers et fiches"
        window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
        window.setContentSize(NSSize(width: 1050, height: 650)); window.center()
        super.init(window: window)
    }
    required init?(coder: NSCoder) { fatalError("init(coder:) unavailable") }
}
#endif
