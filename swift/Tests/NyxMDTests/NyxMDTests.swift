// Rejoue les vecteurs de conformité communs (testdata/) à travers l'adaptateur Swift :
// mêmes résultats que les tests Go, donc le pont C et le décodage sont vérifiés.
import Foundation
import XCTest
@testable import NyxMD

final class NyxMDTests: XCTestCase {
    private func vectors(_ name: String) throws -> [String: Any] {
        let url = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("testdata/\(name).json")
        return try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as! [String: Any]
    }

    func testVersion() {
        XCTAssertEqual(NyxMD.version, "0.3.0")
        XCTAssertNil(NyxMD.compatibilityError)
    }

    func testSearchVectors() throws {
        let v = try vectors("search")
        let docs = v["documents"] as! [String: [String: Any]]
        func doc(_ k: String) -> NyxDocument {
            let d = docs[k]!
            return NyxDocument(text: d["text"] as! String, tags: d["tags"] as? [String] ?? [], entities: d["entities"] as? [String] ?? [])
        }
        for c in v["cases"] as! [[String: Any]] {
            let q = c["query"] as! String
            let got = try NyxSearch.match(q, documents: [doc(c["doc"] as! String)])
            XCTAssertEqual(got, [c["match"] as! Bool], "\(c["doc"]!) / \(q)")
        }
        for c in v["invalid"] as! [[String: Any]] {
            let q = c["query"] as! String
            guard case .failure(let e) = NyxSearch.validate(q) else { XCTFail("accepté : \(q)"); continue }
            XCTAssertEqual(e.code, c["code"] as! String, q)
            XCTAssertEqual(e.position, c["position"] as! Int, q)
            XCTAssertThrowsError(try NyxSearch.match(q, documents: []))
        }
    }

    func testVariableVectors() throws {
        let v = try vectors("variables")
        let c = v["context"] as! [String: Any]
        var ctx = NyxContext()
        ctx.text = c["text"] as! String; ctx.url = c["url"] as! String; ctx.title = c["title"] as! String
        ctx.tags = c["tags"] as! String; ctx.comment = c["comment"] as! String
        ctx.date = Date(timeIntervalSince1970: TimeInterval(c["timestamp"] as! Int))
        ctx.timeZone = TimeZone(secondsFromGMT: c["utc_offset_seconds"] as! Int)!
        ctx.personal = (c["personal"] as! [[String: String]]).map { NyxVariable(name: $0["name"]!, value: $0["value"]!) }
        for r in v["render"] as! [[String: Any]] {
            let (out, diags) = try NyxVariables.render(r["template"] as! String, context: ctx)
            XCTAssertEqual(out, r["output"] as! String)
            XCTAssertEqual(diags.compactMap(\.name), r["warnings"] as! [String])
        }
        for p in v["personal_text"] as! [[String: Any]] {
            let (vars, diags) = try NyxVariables.parsePersonal(p["text"] as! String)
            XCTAssertEqual(vars.map(\.name), p["names"] as! [String])
            XCTAssertEqual(diags.map(\.code).sorted(), (p["errors"] as! [String]).sorted())
        }
        for k in v["complete"] as! [[String: Any]] {
            let comp = try NyxVariables.complete(k["text"] as! String, caret: k["caret"] as! Int, context: ctx)
            if k["none"] as? Bool == true { XCTAssertNil(comp); continue }
            XCTAssertEqual(comp?.start, k["start"] as? Int)
            XCTAssertEqual(comp?.end, k["end"] as? Int)
            XCTAssertEqual(comp?.items.map(\.label), k["labels"] as? [String])
        }
    }

    func testUnicodePositions() throws {
        let text = "😀 é $date"
        let refs = try NyxVariables.references(text, context: NyxContext())
        XCTAssertEqual(refs.first?.start, 4)
        let r = NyxText.nsRange(start: refs[0].start, end: refs[0].end, in: text)
        XCTAssertEqual((text as NSString).substring(with: r), "$date")
        XCTAssertEqual(NyxText.scalarOffset(utf16: r.location, in: text), 4)
        XCTAssertEqual(NyxText.scalarOffset(utf16: 1, in: text), 0) // milieu de l'emoji
    }

    func testPersonalFromSystemTimeZone() throws {
        var ctx = NyxContext()
        ctx.personal = [NyxVariable(name: "client", value: "ACME")]
        XCTAssertEqual(try NyxVariables.render("Client $client, pas $clientèle", context: ctx).output, "Client ACME, pas $clientèle")
    }
}

final class NyxMD02Tests: XCTestCase {
    func testABICompatible() {
        XCTAssertNil(NyxMD.compatibilityError)
        XCTAssertEqual(NyxMD.version, "0.3.0")
        XCTAssertNil(NyxMD.compatibilityError)
    }

    func testCompiledQueryAndFind() throws {
        let q = try NyxQuery("(client OR \"à relancer\") -archivé")
        XCTAssertEqual(q.terms, ["client", "à relancer"])
        let docs = [NyxDocument(text: "😀 client : facture à relancer"), NyxDocument(text: "client archivé")]
        XCTAssertEqual(try q.match(docs), [true, false])
        let r = try q.find(docs)
        XCTAssertEqual(r[0].spans.map { [$0.start, $0.end] }, [[2, 8], [19, 29]])
        let text = docs[0].text
        let ns = NyxText.nsRange(start: r[0].spans[0].start, end: r[0].spans[0].end, in: text)
        XCTAssertEqual((text as NSString).substring(with: ns), "client")
        XCTAssertFalse(r[1].match)
        // Plusieurs lots avec la même requête compilée.
        for _ in 0..<50 { XCTAssertEqual(try q.match([NyxDocument(text: "client")]), [true]) }
    }

    func testFoldAccentsOption() throws {
        let doc = [NyxDocument(text: "Dossier archivé, Œuvre")]
        XCTAssertEqual(try NyxSearch.match("archive", documents: doc), [false])
        XCTAssertEqual(try NyxSearch.match("archive oeuvre", documents: doc, options: .init(foldAccents: true)), [true])
    }

    func testContractFindAndFold() throws {
        let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("testdata/search.json")
        let v = try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as! [String: Any]
        XCTAssertEqual(v["contract"] as? Int, 2)
        for c in v["find"] as! [[String: Any]] {
            let r = try NyxQuery(c["query"] as! String).find([NyxDocument(text: c["text"] as! String)])[0]
            XCTAssertEqual(r.match, c["match"] as! Bool)
            XCTAssertEqual(r.spans.map { [$0.start, $0.end] }, c["spans"] as! [[Int]])
        }
        for c in v["fold_accents"] as! [[String: Any]] {
            let d = [NyxDocument(text: c["text"] as! String, tags: c["tags"] as! [String])]
            XCTAssertEqual(try NyxSearch.match(c["query"] as! String, documents: d), [c["strict"] as! Bool])
            XCTAssertEqual(try NyxSearch.match(c["query"] as! String, documents: d, options: .init(foldAccents: true)), [c["folded"] as! Bool])
        }
    }

    func testInvalidQueryThrowsWithPosition() {
        XCTAssertThrowsError(try NyxQuery("x AND (client")) { e in
            XCTAssertEqual((e as? NyxError)?.code, "missing_closing_parenthesis")
            XCTAssertEqual((e as? NyxError)?.position, 6)
        }
    }

    /// Vecteurs communs de la couche de notes (testdata/notes.json), à travers le pont C.
    func testNotesVectors() throws {
        let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("testdata/notes.json")
        let v = try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as! [String: Any]
        XCTAssertEqual(v["contract"] as? Int, 1)
        let docs = v["documents"] as! [String: String]
        func text(_ c: [String: Any]) -> String { (c["doc"] as? String).map { docs[$0]! } ?? c["text"] as! String }
        for c in v["parse"] as! [[String: Any]] {
            let d = try NyxNotes.parse(text(c))
            if let notes = c["notes"] as? [[String: Any]] {
                XCTAssertEqual(d.notes.map { $0.id }, notes.map { $0["id"] as! String })
                XCTAssertEqual(d.notes.map { $0.start }, notes.map { $0["start"] as! Int })
                XCTAssertEqual(d.notes.map { $0.text }, notes.map { $0["text"] as! String })
            }
            XCTAssertEqual(d.diagnostics.map { $0.code }, c["diagnostics"] as! [String])
            if let m = c["masked"] as? String { XCTAssertEqual(d.masked, m) }
        }
        for c in v["strip"] as! [[String: Any]] { XCTAssertEqual(try NyxNotes.strip(text(c)), c["output"] as! String) }
        for c in v["footnotes"] as! [[String: Any]] { XCTAssertEqual(try NyxNotes.footnotes(text(c)), c["output"] as! String) }
        for c in v["edits"] as! [[String: Any]] {
            let src = text(c), op = c["op"] as! String
            let color = c["color"] as? String ?? "", body = c["body"] as? String ?? "", id = c["id"] as? String ?? ""
            do {
                let out: String
                switch op {
                case "add":
                    let r = try NyxNotes.add(src, start: c["start"] as! Int, end: c["end"] as! Int, color: color, body: body)
                    XCTAssertEqual(r.id, id)
                    out = r.text
                case "update": out = try NyxNotes.update(src, id: id, color: color, body: body)
                default: out = try NyxNotes.delete(src, id: id)
                }
                XCTAssertNil(c["error"], "\(op) aurait dû échouer")
                XCTAssertEqual(out, c["output"] as? String)
            } catch let e as NyxError {
                XCTAssertEqual(e.code, c["error"] as? String, op)
            }
        }
        for c in v["search"] as! [[String: Any]] {
            let q = c["query"] as! String
            XCTAssertEqual(try NyxSearch.match(q, documents: [try NyxNotes.searchDocument(text(c))]), [c["match"] as! Bool], q)
        }
        XCTAssertEqual(try NyxNotes.colors().default, "jaune")
    }
}
