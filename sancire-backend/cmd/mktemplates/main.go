// Command mktemplates renders the seven approved module templates
// (sancire-backend/templates/*.docx) from the canonical specs in internal/docx and
// refreshes checksums.sha256. The source .docx files are strictly read-only
// at runtime — generation never mutates them (п. 3.1 Методических рекомендаций).
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sancireai/backend/internal/docx"
)

const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`</Types>`

const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

func buildDocx(documentXML []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	write := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := write("[Content_Types].xml", []byte(contentTypes)); err != nil {
		return nil, err
	}
	if err := write("_rels/.rels", []byte(rootRels)); err != nil {
		return nil, err
	}
	if err := write("word/document.xml", documentXML); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	outDir := "templates"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	ids := docx.ModuleIDs()
	type row struct{ path, sum string }
	var rows []row
	for _, id := range ids {
		mod := docx.ModulesRegistry[id]
		documentXML, err := docx.BuildTemplateDocumentXML(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: build xml: %v\n", id, err)
			os.Exit(1)
		}
		blob, err := buildDocx(documentXML)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: zip: %v\n", id, err)
			os.Exit(1)
		}
		outPath := filepath.Join(outDir, mod.FileName)
		if err := os.WriteFile(outPath, blob, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "%s: write: %v\n", id, err)
			os.Exit(1)
		}
		sum := sha256.Sum256(blob)
		rows = append(rows, row{path: outPath, sum: hex.EncodeToString(sum[:])})
		fmt.Printf("wrote %s (%d bytes)\n", outPath, len(blob))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })
	var ck bytes.Buffer
	for _, r := range rows {
		fmt.Fprintf(&ck, "%s  %s\n", r.sum, r.path)
	}
	ckPath := filepath.Join(outDir, "checksums.sha256")
	if err := os.WriteFile(ckPath, ck.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "checksums:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", ckPath)
}
