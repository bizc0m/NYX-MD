// NyxMD — API Swift du moteur partagé NYX-MD (recherche booléenne + variables).
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
    public init(text: String, tags: [String] = [], entities: [String] = []) {
        self.text = text; self.tags = tags; self.entities = entities
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

    static func call<T: Decodable>(_ request: [String: Any], as: T.Type) throws -> T {
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
        return (try? Engine.call(["op": "version"], as: V.self).version) ?? "inconnue"
    }
}

// MARK: - Recherche booléenne

/// Grammaire NyxBoolean : mots entiers, AND/OR/NOT en majuscules, -terme, "expression",
/// ( ), jokers * ?, NEAR/n, #tag et @entité. Requête vide = tous les documents.
public enum NyxSearch {
    private struct Validation: Decodable { let terms: [String]? }
    private struct Matches: Decodable { let matches: [Bool] }

    /// Vérifie la requête ; renvoie les termes positifs (pour surligner) ou l'erreur.
    public static func validate(_ query: String) -> Result<[String], NyxError> {
        do {
            return .success(try Engine.call(["op": "search.validate", "query": query], as: Validation.self).terms ?? [])
        } catch let e as NyxError {
            return .failure(e)
        } catch {
            return .failure(NyxError(code: "internal_error", message: "\(error)", position: -1))
        }
    }

    /// Évalue la requête sur chaque document (une compilation par appel).
    public static func match(_ query: String, documents: [NyxDocument]) throws -> [Bool] {
        let docs = documents.map { ["text": $0.text, "tags": $0.tags, "entities": $0.entities] as [String: Any] }
        return try Engine.call(["op": "search.match", "query": query, "documents": docs], as: Matches.self).matches
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
