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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sancireai/backend/internal/docx"
)

// checksumFile is the manifest written next to the rendered templates.
const checksumFile = "checksums.sha256"

// fail reports an error and stops with a non-zero exit code.
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// templateRow is one line of the checksum manifest.
type templateRow struct{ path, sum string }

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
	outDir := flag.String("out", "templates", "каталог для готовых .docx и "+checksumFile)
	flag.Parse()
	// A bare positional argument keeps the old «mktemplates <dir>» form working.
	if arg := flag.Arg(0); arg != "" {
		*outDir = arg
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail("mkdir %s: %v", *outDir, err)
	}
	var rows []templateRow
	for _, id := range docx.ModuleIDs() {
		mod := docx.ModulesRegistry[id]
		documentXML, err := docx.BuildTemplateDocumentXML(id)
		if err != nil {
			fail("%s: build xml: %v", id, err)
		}
		blob, err := buildDocx(documentXML)
		if err != nil {
			fail("%s: zip: %v", id, err)
		}
		outPath := filepath.Join(*outDir, mod.FileName)
		if err := os.WriteFile(outPath, blob, 0o644); err != nil {
			fail("%s: write: %v", id, err)
		}
		sum := sha256.Sum256(blob)
		rows = append(rows, templateRow{path: outPath, sum: hex.EncodeToString(sum[:])})
		fmt.Printf("wrote %s (%d bytes)\n", outPath, len(blob))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })
	var ck bytes.Buffer
	for _, r := range rows {
		fmt.Fprintf(&ck, "%s  %s\n", r.sum, r.path)
	}
	ckPath := filepath.Join(*outDir, checksumFile)
	if err := os.WriteFile(ckPath, ck.Bytes(), 0o644); err != nil {
		fail("checksums %s: %v", ckPath, err)
	}
	fmt.Println("wrote", ckPath)
}
