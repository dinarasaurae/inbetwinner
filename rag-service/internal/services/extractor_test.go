package services

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// ── normalizeContentType ─────────────────────────────────────────────────────

func TestNormalizeContentType_ByMIME(t *testing.T) {
	cases := []struct {
		ct   string
		name string
		want string
	}{
		{"text/plain", "doc.txt", "text"},
		{"text/markdown", "README.md", "text"},
		{"text/csv", "data.csv", "text"},
		{"application/pdf", "report.pdf", "pdf"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "doc.docx", "docx"},
		{"application/msword", "old.doc", "docx"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "sheet.xlsx", "xlsx"},
		{"application/vnd.ms-excel", "old.xls", "xlsx"},
		{"application/octet-stream", "unknown.bin", "unknown"},
	}
	for _, tc := range cases {
		got := normalizeContentType(tc.ct, tc.name)
		if got != tc.want {
			t.Errorf("normalizeContentType(%q, %q) = %q, want %q", tc.ct, tc.name, got, tc.want)
		}
	}
}

func TestNormalizeContentType_ByExtension(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"report.pdf", "pdf"},
		{"brief.docx", "docx"},
		{"data.xlsx", "xlsx"},
		{"notes.txt", "text"},
		{"README.md", "text"},
		{"prices.csv", "text"},
	}
	for _, tc := range cases {
		got := normalizeContentType("", tc.name)
		if got != tc.want {
			t.Errorf("normalizeContentType(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ── ExtractText plain text ───────────────────────────────────────────────────

func TestExtractText_PlainText(t *testing.T) {
	content := "Привет, мир!\n\nЭто тестовый документ."
	got, err := ExtractText("doc.txt", "text/plain", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestExtractText_Markdown(t *testing.T) {
	content := "# Тарифы\n\n**Базовый** — 500 руб/мес"
	got, err := ExtractText("README.md", "text/markdown", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

// ── ExtractText DOCX ─────────────────────────────────────────────────────────

// buildDOCX creates a minimal in-memory DOCX (ZIP + word/document.xml).
func buildDOCX(paragraphs []string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Minimal content_types
	ct, _ := zw.Create("[Content_Types].xml")
	ct.Write([]byte(`<?xml version="1.0"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))

	// word/document.xml with <w:t> elements
	var xmlBuf strings.Builder
	xmlBuf.WriteString(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragraphs {
		xmlBuf.WriteString(`<w:p><w:r><w:t>`)
		xmlBuf.WriteString(p)
		xmlBuf.WriteString(`</w:t></w:r></w:p>`)
	}
	xmlBuf.WriteString(`</w:body></w:document>`)

	doc, _ := zw.Create("word/document.xml")
	doc.Write([]byte(xmlBuf.String()))
	zw.Close()
	return buf.Bytes()
}

func TestExtractText_DOCX(t *testing.T) {
	paras := []string{"Первый абзац.", "Второй абзац.", "Третий абзац."}
	data := buildDOCX(paras)

	got, err := ExtractText("test.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range paras {
		if !strings.Contains(got, p) {
			t.Errorf("extracted text missing %q, got:\n%s", p, got)
		}
	}
}

func TestExtractText_DOCX_ByExtension(t *testing.T) {
	data := buildDOCX([]string{"Hello world"})
	got, err := ExtractText("doc.docx", "", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("expected %q in output, got: %q", "Hello world", got)
	}
}

// ── ExtractText XLSX ─────────────────────────────────────────────────────────

// buildXLSX creates a minimal in-memory XLSX with inline string cells.
func buildXLSX(rows [][]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// [Content_Types].xml
	ct, _ := zw.Create("[Content_Types].xml")
	ct.Write([]byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
</Types>`))

	// xl/worksheets/sheet1.xml with inlineStr cells
	var xmlBuf strings.Builder
	xmlBuf.WriteString(`<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for _, row := range rows {
		xmlBuf.WriteString(`<row>`)
		for _, cell := range row {
			xmlBuf.WriteString(`<c t="inlineStr"><is><t>`)
			xmlBuf.WriteString(cell)
			xmlBuf.WriteString(`</t></is></c>`)
		}
		xmlBuf.WriteString(`</row>`)
	}
	xmlBuf.WriteString(`</sheetData></worksheet>`)

	sheet, _ := zw.Create("xl/worksheets/sheet1.xml")
	sheet.Write([]byte(xmlBuf.String()))
	zw.Close()
	return buf.Bytes()
}

func TestExtractText_XLSX(t *testing.T) {
	rows := [][]string{
		{"Продукт", "Цена", "Количество"},
		{"Тариф Базовый", "500", "100"},
		{"Тариф Про", "1500", "50"},
	}
	data := buildXLSX(rows)

	got, err := ExtractText("prices.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, row := range rows {
		for _, cell := range row {
			if !strings.Contains(got, cell) {
				t.Errorf("expected %q in output, got:\n%s", cell, got)
			}
		}
	}
}

// ── ObjectKey ────────────────────────────────────────────────────────────────

func TestObjectKey(t *testing.T) {
	wid := "workspace-123"
	did := "doc-456"
	fn := "report.pdf"
	got := ObjectKey(wid, did, fn)
	want := "workspace-123/doc-456/report.pdf"
	if got != want {
		t.Errorf("ObjectKey() = %q, want %q", got, want)
	}
}

// ── chunkText ────────────────────────────────────────────────────────────────

func TestChunkText_Empty(t *testing.T) {
	if chunks := chunkText("", 512); len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestChunkText_ShortText(t *testing.T) {
	text := "Короткий текст."
	chunks := chunkText(text, 512)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], text)
	}
}

func TestChunkText_MultiParagraph(t *testing.T) {
	// Use ASCII to keep len() == rune count.
	// Each para is 200 bytes; chunkSize=250 means two paras (200+2+200=402) won't fit,
	// so each paragraph becomes its own chunk.
	para := strings.Repeat("x", 200)
	text := para + "\n\n" + para + "\n\n" + para
	chunks := chunkText(text, 250)
	if len(chunks) < 2 {
		t.Errorf("expected ≥ 2 chunks for 3 paras of 200 bytes each at chunkSize=250, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 250+10 {
			t.Errorf("chunk[%d] length %d exceeds chunkSize", i, len(c))
		}
	}
}

func TestChunkText_ReconstructsAllText(t *testing.T) {
	// Every paragraph that went in should appear in at least one chunk
	paras := []string{"Первый абзац с важной информацией.", "Второй абзац.", "Третий абзац с деталями."}
	text := strings.Join(paras, "\n\n")
	chunks := chunkText(text, 50)
	all := strings.Join(chunks, " ")
	for _, p := range paras {
		// Check that each paragraph's content is present (may be split by sentence)
		words := strings.Fields(p)
		firstWord := words[0]
		if !strings.Contains(all, firstWord) {
			t.Errorf("paragraph starting with %q not found in chunks", firstWord)
		}
	}
}
