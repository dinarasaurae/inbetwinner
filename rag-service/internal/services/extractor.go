package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractText extracts plain text from a file given its raw bytes and MIME type.
// Supported formats:
//   - text/plain, text/markdown, text/csv  → returned as-is
//   - application/pdf                       → page text via ledongthuc/pdf
//   - application/vnd.openxmlformats-officedocument.wordprocessingml.document (.docx)
//   - application/vnd.openxmlformats-officedocument.spreadsheetml.sheet (.xlsx) → tab-separated cells
//
// Falls back to UTF-8 interpretation for unknown types.
func ExtractText(filename string, contentType string, data []byte) (string, error) {
	ct := normalizeContentType(contentType, filename)
	switch ct {
	case "text":
		return string(data), nil
	case "pdf":
		return extractPDF(data)
	case "docx":
		return extractDOCX(data)
	case "xlsx":
		return extractXLSX(data)
	default:
		// Best-effort: return raw bytes as string
		return string(data), nil
	}
}

func normalizeContentType(contentType, filename string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(ct, "text/"):
		return "text"
	case ct == "application/pdf":
		return "pdf"
	case ct == "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		ct == "application/msword":
		return "docx"
	case ct == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		ct == "application/vnd.ms-excel":
		return "xlsx"
	}
	// Fallback by extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".xlsx":
		return "xlsx"
	case ".txt", ".md", ".csv":
		return "text"
	}
	return "unknown"
}

// ── PDF ──────────────────────────────────────────────────────────────────────
// Uses pdftotext (poppler-utils) when available — handles CID fonts, Cyrillic,
// complex layouts, and virtually any non-scanned PDF.
// Falls back to a pure-Go page-text scan when the binary is absent so that
// development environments without poppler still work (with reduced quality).

func extractPDF(data []byte) (string, error) {
	// Primary path: pdftotext (poppler-utils)
	if path, err := exec.LookPath("pdftotext"); err == nil {
		return extractPDFWithPoppler(path, data)
	}
	log.Println("[extractor] pdftotext not found, falling back to pure-Go PDF parser (reduced quality for Cyrillic/CID fonts)")
	return extractPDFFallback(data)
}

func extractPDFWithPoppler(pdftotextBin string, data []byte) (string, error) {
	tmp, err := os.CreateTemp("", "inbetwin-*.pdf")
	if err != nil {
		return "", fmt.Errorf("pdf tmp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("pdf write tmp: %w", err)
	}
	tmp.Close()

	// "-" as output file → stdout; "-enc UTF-8" → explicit UTF-8 output
	out, err := exec.Command(pdftotextBin, "-enc", "UTF-8", "-nopgbrk", tmp.Name(), "-").Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("pdftotext extracted no text (possibly a scanned/image PDF)")
	}
	return text, nil
}

// extractPDFFallback is the original pure-Go implementation using the
// ledongthuc/pdf approach reimplemented inline. It works for simple
// Latin-script PDFs but may return empty or garbled text for Cyrillic/CID fonts.
func extractPDFFallback(data []byte) (string, error) {
	// Minimal pure-Go extraction: scan for BT...ET blocks and collect Tj/TJ strings.
	// This is a best-effort fallback — not a full PDF parser.
	content := string(data)
	var sb strings.Builder
	inBT := false
	i := 0
	for i < len(content)-1 {
		if !inBT && i+2 <= len(content) && content[i:i+2] == "BT" {
			inBT = true
			i += 2
			continue
		}
		if inBT && i+2 <= len(content) && content[i:i+2] == "ET" {
			inBT = false
			sb.WriteString("\n")
			i += 2
			continue
		}
		if inBT && content[i] == '(' {
			// Collect (text) Tj
			j := i + 1
			for j < len(content) && content[j] != ')' {
				if content[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(content) {
				sb.WriteString(content[i+1 : j])
				sb.WriteString(" ")
			}
			i = j + 1
			continue
		}
		i++
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", fmt.Errorf("pure-go PDF fallback extracted no text")
	}
	return text, nil
}

// ── DOCX ─────────────────────────────────────────────────────────────────────
// DOCX files are ZIP archives. The main content lives in word/document.xml
// with <w:t> elements containing the actual text runs.

type docxBody struct {
	XMLName xml.Name    `xml:"body"`
	Paras   []docxPara  `xml:"p"`
	Tables  []docxTable `xml:"tbl"`
}

type docxPara struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

type docxTable struct {
	Rows []docxRow `xml:"tr"`
}

type docxRow struct {
	Cells []docxCell `xml:"tc"`
}

type docxCell struct {
	Paras []docxPara `xml:"p"`
}

func extractDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("docx open document.xml: %w", err)
		}
		defer rc.Close()
		xmlBytes, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("docx read document.xml: %w", err)
		}
		return parseDOCXXML(xmlBytes), nil
	}
	return "", fmt.Errorf("docx: word/document.xml not found")
}

func parseDOCXXML(xmlBytes []byte) string {
	// We decode the <w:document> element and collect all <w:t> text nodes.
	// Using a streaming decoder to avoid namespace issues.
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var sb strings.Builder
	var lastWasRun bool
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// <w:p> = paragraph: add newline before new paragraph
			if t.Name.Local == "p" && sb.Len() > 0 {
				sb.WriteString("\n")
				lastWasRun = false
			}
		case xml.CharData:
			text := string(t)
			if strings.TrimSpace(text) != "" {
				if lastWasRun {
					sb.WriteString(" ")
				}
				sb.WriteString(text)
				lastWasRun = true
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// ── XLSX ─────────────────────────────────────────────────────────────────────
// XLSX files are also ZIP archives. Content lives in xl/worksheets/sheet*.xml.
// We extract cell text values and format them as tab-separated rows.

type xlsxWorksheet struct {
	XMLName  xml.Name      `xml:"worksheet"`
	SheetData xlsxSheetData `xml:"sheetData"`
}

type xlsxSheetData struct {
	Rows []xlsxRow `xml:"row"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Type  string `xml:"t,attr"`
	Value string `xml:"v"`
	// Inline string
	Is struct {
		T string `xml:"t"`
	} `xml:"is"`
}

func extractXLSX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("xlsx zip: %w", err)
	}

	// First, collect shared strings (sst)
	sharedStrings := parseXLSXSharedStrings(zr)

	var allSheets strings.Builder
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/sheet") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		xmlBytes, _ := io.ReadAll(rc)
		rc.Close()

		var ws xlsxWorksheet
		if err := xml.Unmarshal(xmlBytes, &ws); err != nil {
			continue
		}
		for _, row := range ws.SheetData.Rows {
			var cells []string
			for _, cell := range row.Cells {
				var val string
				switch cell.Type {
				case "s": // shared string index
					if idx := parseInt(cell.Value); idx >= 0 && idx < len(sharedStrings) {
						val = sharedStrings[idx]
					}
				case "inlineStr":
					val = cell.Is.T
				default:
					val = cell.Value
				}
				cells = append(cells, val)
			}
			if len(cells) > 0 {
				allSheets.WriteString(strings.Join(cells, "\t"))
				allSheets.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(allSheets.String()), nil
}

func parseXLSXSharedStrings(zr *zip.Reader) []string {
	for _, f := range zr.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil
		}
		defer rc.Close()
		xmlBytes, _ := io.ReadAll(rc)

		type sstEntry struct {
			T  string `xml:"t"`
			Rs []struct {
				T string `xml:"t"`
			} `xml:"r"`
		}
		type sst struct {
			SI []sstEntry `xml:"si"`
		}
		var s sst
		if err := xml.Unmarshal(xmlBytes, &s); err != nil {
			return nil
		}
		result := make([]string, len(s.SI))
		for i, si := range s.SI {
			if si.T != "" {
				result[i] = si.T
			} else {
				var parts []string
				for _, r := range si.Rs {
					parts = append(parts, r.T)
				}
				result[i] = strings.Join(parts, "")
			}
		}
		return result
	}
	return nil
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
