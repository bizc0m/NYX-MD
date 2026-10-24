// NyxMD — API Swift du moteur partagé NYX-MD (recherche booléenne, variables, couche de notes).
// Toutes les positions renvoyées par le moteur sont en scalaires Unicode ;
// utiliser NyxText pour convertir en String.Index ou en NSRange (UTF-16).
import CNyxMD
import Foundation

// MARK: - Types publics

/// Erreur renvoyée par le moteur. `position` est en scalaires Unicode (-1 si sans objet).
public struct NyxError: Error, Equatable, LocalizedError, Decodable, Sendable {
    public let code: String
    public let message: String
    public let position: Int
    public var errorDescription: String? { position >= 0 ? "\(message) (position \(position + 1))" : message }
}

public struct NyxDocument: Codable, Equatable, Sendable {
    public var text: String
    public var tags: [String]
    public var entities: [String]
    /// Textes et couleurs de la couche de notes (requêtes note: et couleur:). Voir NyxNotes.searchDocument.
    public var notes: [String]
    public var colors: [String]
    public init(text: String, tags: [String] = [], entities: [String] = [], notes: [String] = [], colors: [String] = []) {
        self.text = text; self.tags = tags; self.entities = entities; self.notes = notes; self.colors = colors
    }
}

public struct NyxVariable: Codable, Equatable, Sendable {
    public var name: String
    public var value: String
    public init(name: String, value: String) { self.name = name; self.value = value }
}

/// Contexte de rendu : toutes les valeurs viennent de l'appelant.
public struct NyxContext: Equatable, Sendable {
    public var text = "", url = "", title = "", tags = "", comment = ""
    public var date = Date()
    public var timeZone = TimeZone.current
    public var locale = "fr_FR"
    public var personal: [NyxVariable] = []
    public init() {}
}

public struct NyxSyntax: Codable, Equatable, Sendable {
    public var braces: Bool
    public var dollar: Bool
    public var indentMultiline: Bool
    public init(braces: Bool, dollar: Bool, indentMultiline: Bool) {
        self.braces = braces; self.dollar = dollar; self.indentMultiline = indentMultiline
    }
    public static let all = NyxSyntax(braces: true, dollar: true, indentMultiline: true)
    enum CodingKeys: String, CodingKey { case braces, dollar, indentMultiline = "indent_multiline" }
}

public struct NyxDiagnostic: Decodable, Equatable, Sendable {
    public let severity: String
    public let code: String
    public let message: String
    public let name: String?
    public let start: Int
    public let end: Int
}

public struct NyxReference: Decodable, Equatable, Sendable {
    public let syntax: String
    public let name: String
    /// template, native, personal, reserved ou unknown
    public let kind: String
    public let start: Int
    public let end: Int
}

public struct NyxCompletionItem: Decodable, Equatable, Sendable {
    public let label: String
    public let insert: String
    public let kind: String
    public let detail: String
}

public struct NyxCompletion: Decodable, Equatable, Sendable {
    public let start: Int
    public let end: Int
    public let items: [NyxCompletionItem]
}

// MARK: - Appels au moteur

enum Engine {
    struct Envelope<T: Decodable>: Decodable {
        let ok: Bool
        let error: NyxError?
        let payload: T?
        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: AnyKey.self)
            ok = try c.decode(Bool.self, forKey: AnyKey("ok"))
            error = try c.decodeIfPresent(NyxError.self, forKey: AnyKey("error"))
            payload = ok ? try T(from: decoder) : nil
        }
    }

    struct AnyKey: CodingKey {
        var stringValue: String
        var intValue: Int? { nil }
        init(_ s: String) { stringValue = s }
        init?(stringValue: String) { self.stringValue = stringValue }
        init?(intValue: Int) { nil }
    }

    /// Version d'interface attendue par cet adaptateur (voir cabi/main.go ABIVersion).
    static let expectedABI = 3

    /// Vérifiée une seule fois : une bibliothèque CNyxMD d'une autre version donne une erreur claire.
    static let abiCheck: NyxError? = {
        struct V: Decodable { let abi: Int; let version: String }
        guard let v = try? rawCall(["op": "version"], as: V.self) else {
            return NyxError(code: "abi_mismatch", message: "bibliothèque NYX-MD illisible", position: -1)
        }
        guard v.abi == expectedABI else {
            return NyxError(code: "abi_mismatch",
                            message: "bibliothèque NYX-MD \(v.version) (interface \(v.abi)) incompatible avec l'adaptateur (interface \(expectedABI))",
                            position: -1)
        }
        return nil
    }()

    static func call<T: Decodable>(_ request: [String: Any], as type: T.Type) throws -> T {
        if let e = abiCheck { throw e }
        return try rawCall(request, as: type)
    }

    static func rawCall<T: Decodable>(_ request: [String: Any], as: T.Type) throws -> T {
        let data = try JSONSerialization.data(withJSONObject: request)
        let json = String(decoding: data, as: UTF8.self)
        guard let raw = json.withCString({ nyxmd_call($0) }) else {
            throw NyxError(code: "internal_error", message: "réponse nulle du moteur", position: -1)
        }
        defer { nyxmd_free(raw) }
        let envelope = try JSONDecoder().decode(Envelope<T>.self, from: Data(String(cString: raw).utf8))
        if let error = envelope.error { throw error }
        guard let payload = envelope.payload else {
            throw NyxError(code: "internal_error", message: "réponse incomplète du moteur", position: -1)
        }
        return payload
    }

    static func context(_ c: NyxContext) -> [String: Any] {
        [
            "text": c.text, "url": c.url, "title": c.title, "tags": c.tags, "comment": c.comment,
            "timestamp": Int64(c.date.timeIntervalSince1970.rounded(.down)),
            "utc_offset_seconds": c.timeZone.secondsFromGMT(for: c.date),
            "locale": c.locale,
            "personal": c.personal.map { ["name": $0.name, "value": $0.value] },
        ]
    }

    static func syntax(_ s: NyxSyntax) -> [String: Any] {
        ["braces": s.braces, "dollar": s.dollar, "indent_multiline": s.indentMultiline]
    }
}

public enum NyxMD {
    /// Version du moteur lié (SemVer).
    public static var version: String {
        struct V: Decodable { let version: String }
        return (try? Engine.rawCall(["op": "version"], as: V.self).version) ?? "inconnue"
    }

    /// nil si la bibliothèque liée correspond à l'adaptateur, sinon l'erreur abi_mismatch.
    public static var compatibilityError: NyxError? { Engine.abiCheck }
}

// MARK: - Recherche booléenne

/// Options de recherche.
public struct NyxSearchOptions: Equatable, Sendable {
    /// Ignore les accents des lettres latines (archive = archivé). Désactivé par défaut.
    public var foldAccents: Bool
    public init(foldAccents: Bool = false) { self.foldAccents = foldAccents }
    var json: [String: Any] { ["fold_accents": foldAccents] }
}

/// Résultat de recherche d'un document : correspondance et plages trouvées (scalaires Unicode).
public struct NyxFindResult: Decodable, Equatable, Sendable {
    public struct Span: Decodable, Equatable, Sendable {
        public let start: Int
        public let end: Int
    }
    public let match: Bool
    public let spans: [Span]
}

/// Requête compilée une seule fois puis évaluée sur autant de documents que voulu.
/// Libérée automatiquement à la destruction de l'objet.
public final class NyxQuery {
    private let handle: Int64
    /// Termes et expressions non niés de la requête.
    public let terms: [String]

    private struct Compiled: Decodable { let handle: Int64; let terms: [String] }
    private struct Matches: Decodable { let matches: [Bool] }
    private struct Found: Decodable { let results: [NyxFindResult] }

    /// Lance NyxError (code, message, position) si la requête est invalide.
    public init(_ query: String, options: NyxSearchOptions = .init()) throws {
        let c = try Engine.call(["op": "search.compile", "query": query, "options": options.json], as: Compiled.self)
        handle = c.handle
        terms = c.terms
    }

    deinit { _ = try? Engine.rawCall(["op": "search.release", "handle": handle], as: Empty.self) }

    public func match(_ documents: [NyxDocument]) throws -> [Bool] {
        try Engine.call(["op": "search.match", "handle": handle, "documents": documents.map(\.json)], as: Matches.self).matches
    }

    public func find(_ documents: [NyxDocument]) throws -> [NyxFindResult] {
        try Engine.call(["op": "search.find", "handle": handle, "documents": documents.map(\.json)], as: Found.self).results
    }

    private struct Empty: Decodable {}
}

extension NyxDocument {
    var json: [String: Any] { ["text": text, "tags": tags, "entities": entities, "notes": notes, "colors": colors] }
}

/// Grammaire NyxBoolean : mots entiers, AND/OR/NOT en majuscules, -terme, "expression",
/// ( ), jokers * ?, NEAR/n, #tag et @entité, note:mot, couleur:nom. Requête vide = tous les documents.
public enum NyxSearch {
    private struct Validation: Decodable { let terms: [String]? }

    /// Vérifie la requête ; renvoie les termes positifs (pour surligner) ou l'erreur.
    public static func validate(_ query: String, options: NyxSearchOptions = .init()) -> Result<[String], NyxError> {
        do {
            return .success(try Engine.call(["op": "search.validate", "query": query, "options": options.json], as: Validation.self).terms ?? [])
        } catch let e as NyxError {
            return .failure(e)
        } catch {
            return .failure(NyxError(code: "internal_error", message: "\(error)", position: -1))
        }
    }

    /// Raccourci : compile et évalue en un appel. Pour plusieurs lots, préférer NyxQuery.
    public static func match(_ query: String, documents: [NyxDocument], options: NyxSearchOptions = .init()) throws -> [Bool] {
        try NyxQuery(query, options: options).match(documents)
    }
}

// MARK: - Couche de notes

/// Note du bloc de fin de fichier. `start`/`end` : passage annoté (scalaires Unicode, entre les repères),
/// égaux pour une note ponctuelle, -1 si la note est orpheline (aucun repère dans le texte).
public struct NyxNote: Decodable, Equatable, Sendable {
    public let id: String
    public let color: String
    public let text: String
    public let start: Int
    public let end: Int
    public let orphan: Bool
}

/// Repère <!--nX--> ou <!--/nX--> (plage du commentaire, scalaires Unicode).
public struct NyxNoteMarker: Decodable, Equatable, Sendable {
    public let id: String
    public let closing: Bool
    public let start: Int
    public let end: Int
}

public struct NyxNotesDocument: Decodable, Equatable, Sendable {
    public let notes: [NyxNote]
    public let markers: [NyxNoteMarker]
    /// Bloc de notes et lignes vides qui le précèdent (nil s'il n'y en a pas).
    public let block: NyxFindResult.Span?
    public let extra: [String]
    public let diagnostics: [NyxDiagnostic]
    /// Texte où repères et bloc sont remplacés par des espaces : même longueur, mêmes lignes.
    public let masked: String
}

/// Passages annotés `<!--n1-->…<!--/n1-->` et bloc `<!-- notes v1 … -->` en fin de fichier.
public enum NyxNotes {
    private struct Output: Decodable { let output: String }
    private struct Added: Decodable { let output: String; let id: String }
    private struct Colors: Decodable { let colors: [String]; let `default`: String }

    public static func parse(_ text: String) throws -> NyxNotesDocument {
        try Engine.call(["op": "notes.parse", "text": text], as: NyxNotesDocument.self)
    }

    /// Export « propre » : repères et bloc retirés.
    public static func strip(_ text: String) throws -> String {
        try Engine.call(["op": "notes.strip", "text": text], as: Output.self).output
    }

    /// Export « avec notes » : appels [^k] après les passages, définitions [^k]: en fin de document.
    public static func footnotes(_ text: String) throws -> String {
        try Engine.call(["op": "notes.footnotes", "text": text], as: Output.self).output
    }

    /// Annote [start, end) (scalaires Unicode ; start == end : note ponctuelle). color vide = couleur par défaut.
    public static func add(_ text: String, start: Int, end: Int, color: String = "", body: String) throws -> (text: String, id: String) {
        let r = try Engine.call(["op": "notes.add", "text": text, "start": start, "end": end, "color": color, "body": body], as: Added.self)
        return (r.output, r.id)
    }

    /// color vide = inchangée.
    public static func update(_ text: String, id: String, color: String = "", body: String) throws -> String {
        try Engine.call(["op": "notes.update", "text": text, "id": id, "color": color, "body": body], as: Output.self).output
    }

    /// Retire la note et ses repères ; le passage reste.
    public static func delete(_ text: String, id: String) throws -> String {
        try Engine.call(["op": "notes.delete", "text": text, "id": id], as: Output.self).output
    }

    /// Couleurs reconnues et couleur par défaut.
    public static func colors() throws -> (all: [String], default: String) {
        let c = try Engine.call(["op": "notes.colors"], as: Colors.self)
        return (c.colors, c.default)
    }

    /// Document de recherche : texte masqué (positions inchangées), notes et couleurs pour note: / couleur:.
    public static func searchDocument(_ text: String, tags: [String] = [], entities: [String] = []) throws -> NyxDocument {
        let d = try parse(text)
        return NyxDocument(text: d.masked, tags: tags, entities: entities,
                           notes: d.notes.map(\.text), colors: d.notes.map(\.color))
    }
}

// MARK: - Variables

public enum NyxVariables {
    private struct Rendered: Decodable { let output: String; let diagnostics: [NyxDiagnostic] }
    private struct Diags: Decodable { let diagnostics: [NyxDiagnostic] }
    private struct Refs: Decodable { let references: [NyxReference] }
    private struct Compl: Decodable { let completion: NyxCompletion? }
    private struct Personal: Decodable { let variables: [NyxVariable]; let diagnostics: [NyxDiagnostic] }
    private struct Output: Decodable { let output: String }

    /// Développe les variables en une seule passe ; inconnues conservées + diagnostic.
    public static func render(_ text: String, context: NyxContext, syntax: NyxSyntax = .all) throws -> (output: String, diagnostics: [NyxDiagnostic]) {
        let r = try Engine.call(["op": "variables.render", "text": text, "context": Engine.context(context), "syntax": Engine.syntax(syntax)], as: Rendered.self)
        return (r.output, r.diagnostics)
    }

    public static func validate(_ text: String, context: NyxContext, syntax: NyxSyntax = .all) throws -> [NyxDiagnostic] {
        try Engine.call(["op": "variables.validate", "text": text, "context": Engine.context(context), "syntax": Engine.syntax(syntax)], as: Diags.self).diagnostics
    }

    public static func references(_ text: String, context: NyxContext, syntax: NyxSyntax = .all) throws -> [NyxReference] {
        try Engine.call(["op": "variables.references", "text": text, "context": Engine.context(context), "syntax": Engine.syntax(syntax)], as: Refs.self).references
    }

    /// `caret` en scalaires Unicode. nil hors d'un contexte « {{… » ou « $… ».
    public static func complete(_ text: String, caret: Int, context: NyxContext, syntax: NyxSyntax = .all) throws -> NyxCompletion? {
        try Engine.call(["op": "variables.complete", "text": text, "caret": caret, "context": Engine.context(context), "syntax": Engine.syntax(syntax)], as: Compl.self).completion
    }

    /// Format NoteMistress « nom=valeur, nom2=valeur2 » (virgules ou lignes).
    public static func parsePersonal(_ text: String) throws -> (variables: [NyxVariable], diagnostics: [NyxDiagnostic]) {
        let r = try Engine.call(["op": "variables.parse_personal", "text": text], as: Personal.self)
        return (r.variables, r.diagnostics)
    }

    public static func markdownLink(label: String, url: String) throws -> String {
        try Engine.call(["op": "markdown.link", "label": label, "url": url], as: Output.self).output
    }
}

// MARK: - Conversion des positions

public enum NyxText {
    /// Décalage UTF-16 (NSRange) d'une position en scalaires Unicode.
    public static func utf16Offset(scalar: Int, in text: String) -> Int {
        let scalars = text.unicodeScalars
        let idx = scalars.index(scalars.startIndex, offsetBy: max(0, min(scalar, scalars.count)))
        return text.utf16.distance(from: text.utf16.startIndex, to: idx)
    }

    /// Position en scalaires Unicode d'un décalage UTF-16 (arrondi au scalaire précédent).
    public static func scalarOffset(utf16: Int, in text: String) -> Int {
        var units = 0, count = 0
        for s in text.unicodeScalars {
            let w = s.utf16.count
            if units + w > utf16 { break }
            units += w
            count += 1
        }
        return count
    }

    /// NSRange UTF-16 pour une plage [start, end) en scalaires.
    public static func nsRange(start: Int, end: Int, in text: String) -> NSRange {
        let a = utf16Offset(scalar: start, in: text)
        return NSRange(location: a, length: utf16Offset(scalar: end, in: text) - a)
    }
}
