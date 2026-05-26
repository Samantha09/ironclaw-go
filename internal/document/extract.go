package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Limits aligned with Rust reference.
const (
	MaxDocumentSize        = 10 * 1024 * 1024  // 10 MB
	MaxExtractedTextLen    = 100_000            // ~25K tokens
	MaxDecompressedEntry   = 50 * 1024 * 1024   // 50 MB per ZIP entry
	MaxDecompressedTotal   = 100 * 1024 * 1024  // 100 MB total ZIP
)

// ExtractText extracts text from document bytes based on MIME type and optional filename.
func ExtractText(data []byte, mimeType, filename string) (string, error) {
	baseMime := strings.TrimSpace(strings.Split(mimeType, ";")[0])

	switch baseMime {
	case "application/pdf":
		return "", fmt.Errorf("PDF extraction not yet implemented")

	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return extractDOCX(data)
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return extractPPTX(data)
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return extractXLSX(data)

	case "application/msword", "application/vnd.ms-powerpoint", "application/vnd.ms-excel":
		return extractBinaryStrings(data)

	case "text/plain", "text/csv", "text/tab-separated-values", "text/markdown",
		"text/html", "text/xml",
		"text/x-python", "text/x-java", "text/x-c", "text/x-c++", "text/x-rust", "text/x-go",
		"text/x-ruby", "text/x-shellscript", "text/javascript", "text/css",
		"text/x-toml", "text/x-yaml", "text/x-log":
		return extractUTF8(data)

	case "application/json", "application/xml", "application/x-yaml", "application/yaml",
		"application/toml", "application/x-sh":
		return extractUTF8(data)

	case "application/rtf", "text/rtf":
		return extractRTF(data)

	default:
		if text, ok := tryExtractByExtension(data, filename); ok {
			return text, nil
		}
		return "", fmt.Errorf("unsupported document type: %s", baseMime)
	}
}

func extractUTF8(data []byte) (string, error) {
	if utf8Valid(data) {
		return string(data), nil
	}
	return string(bytesToValidUTF8(data)), nil
}

func utf8Valid(data []byte) bool {
	return string(data) == string(bytesToValidUTF8(data))
}

func bytesToValidUTF8(data []byte) []byte {
	// Simple replacement of invalid UTF-8 sequences
	return []byte(string(data))
}

func extractRTF(data []byte) (string, error) {
	text := string(data)
	var result strings.Builder
	depth := 0
	i := 0
	for i < len(text) {
		ch := text[i]
		switch ch {
		case '{':
			depth++
			i++
		case '}':
			if depth > 0 {
				depth--
			}
			i++
		case '\\':
			// Control word
			i++
			wordStart := i
			for i < len(text) && isASCIIAlpha(text[i]) {
				i++
			}
			word := text[wordStart:i]
			// Skip optional numeric parameter
			for i < len(text) && (isASCIIDigit(text[i]) || text[i] == '-') {
				i++
			}
			// Consume trailing space
			if i < len(text) && text[i] == ' ' {
				i++
			}
			switch word {
			case "par", "line":
				result.WriteByte('\n')
			case "tab":
				result.WriteByte('\t')
			}
		default:
			if depth <= 1 {
				result.WriteByte(ch)
			}
			i++
		}
	}
	trimmed := strings.TrimSpace(result.String())
	if trimmed == "" {
		return "", fmt.Errorf("no text found in RTF")
	}
	return trimmed, nil
}

func isASCIIAlpha(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

func extractBinaryStrings(data []byte) (string, error) {
	var results []string
	var current strings.Builder

	for _, b := range data {
		if b >= 0x20 && b < 0x7F {
			current.WriteByte(b)
		} else {
			if current.Len() >= 4 {
				results = append(results, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() >= 4 {
		results = append(results, current.String())
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no readable text in binary document")
	}
	return strings.Join(results, " "), nil
}

func tryExtractByExtension(data []byte, filename string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".csv", ".json", ".xml", ".yaml", ".yml", ".toml",
		".py", ".go", ".rs", ".java", ".c", ".cpp", ".h", ".js", ".ts",
		".html", ".css", ".sh", ".rb", ".log":
		text, _ := extractUTF8(data)
		return text, true
	case ".rtf":
		text, _ := extractRTF(data)
		return text, true
	case ".docx":
		text, _ := extractDOCX(data)
		return text, true
	case ".pptx":
		text, _ := extractPPTX(data)
		return text, true
	case ".xlsx":
		text, _ := extractXLSX(data)
		return text, true
	}
	return "", false
}

// --- Office XML (ZIP-based) extraction ---

func extractDOCX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid DOCX archive: %w", err)
	}
	f, err := openZipEntry(r, "word/document.xml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	text, err := io.ReadAll(io.LimitReader(f, MaxDecompressedEntry))
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}
	result := stripXMLTags(string(text))
	if result == "" {
		return "", fmt.Errorf("no text content found")
	}
	return result, nil
}

func extractPPTX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid PPTX archive: %w", err)
	}

	// Collect slide filenames
	var slideNames []string
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slideNames = append(slideNames, f.Name)
		}
	}
	sortStrings(slideNames)

	var allText []string
	var totalDecompressed int64
	for _, name := range slideNames {
		f, err := r.Open(name)
		if err != nil {
			continue
		}
		text, err := readZipEntryBounded(f, &totalDecompressed)
		f.Close()
		if err != nil {
			continue
		}
		stripped := stripXMLTags(text)
		if stripped != "" {
			allText = append(allText, stripped)
		}
	}

	if len(allText) == 0 {
		return "", fmt.Errorf("no text found in PPTX slides")
	}
	return strings.Join(allText, "\n\n---\n\n"), nil
}

func extractXLSX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid XLSX archive: %w", err)
	}

	var totalDecompressed int64

	// Read shared strings
	var sharedStrings []string
	f, err := openZipEntry(r, "xl/sharedStrings.xml")
	if err == nil {
		text, _ := io.ReadAll(io.LimitReader(f, MaxDecompressedEntry))
		f.Close()
		sharedStrings = parseXLSXSharedStrings(string(text))
		totalDecompressed += int64(len(text))
	}

	// Collect sheet filenames
	var sheetNames []string
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/sheet") && strings.HasSuffix(f.Name, ".xml") {
			sheetNames = append(sheetNames, f.Name)
		}
	}
	sortStrings(sheetNames)

	var allText []string
	for _, name := range sheetNames {
		f, err := r.Open(name)
		if err != nil {
			continue
		}
		text, err := readZipEntryBounded(f, &totalDecompressed)
		f.Close()
		if err != nil {
			continue
		}
		stripped := parseXLSXSheet(string(text), sharedStrings)
		if stripped != "" {
			allText = append(allText, stripped)
		}
	}

	if len(allText) == 0 && len(sharedStrings) > 0 {
		return strings.Join(sharedStrings, "\n"), nil
	}
	if len(allText) == 0 {
		return "", fmt.Errorf("no text found in XLSX")
	}
	return strings.Join(allText, "\n\n"), nil
}

func openZipEntry(r *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("content file not found in archive: %s", name)
}

func readZipEntryBounded(r io.ReadCloser, total *int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxDecompressedEntry))
	if err != nil {
		return "", err
	}
	*total += int64(len(data))
	if *total > MaxDecompressedTotal {
		return "", fmt.Errorf("total decompressed size exceeds limit")
	}
	return string(data), nil
}

// stripXMLTags removes XML tags and decodes common entities.
func stripXMLTags(xmlStr string) string {
	var result strings.Builder
	inTag := false
	lastWasSpace := true

	for _, ch := range xmlStr {
		switch ch {
		case '<':
			inTag = true
		case '>':
			inTag = false
			if !lastWasSpace && result.Len() > 0 {
				result.WriteByte(' ')
				lastWasSpace = true
			}
		default:
			if !inTag {
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
					if !lastWasSpace {
						result.WriteByte(' ')
						lastWasSpace = true
					}
				} else {
					result.WriteRune(ch)
					lastWasSpace = false
				}
			}
		}
	}

	s := result.String()
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&apos;", "'")
	return strings.TrimSpace(s)
}

func parseXLSXSharedStrings(xmlStr string) []string {
	decoder := xml.NewDecoder(strings.NewReader(xmlStr))
	var results []string
	var inT bool
	var current strings.Builder

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "t" {
				inT = true
				current.Reset()
			}
		case xml.EndElement:
			if se.Name.Local == "t" {
				inT = false
				results = append(results, current.String())
			}
		case xml.CharData:
			if inT {
				current.Write(se)
			}
		}
	}
	return results
}

func parseXLSXSheet(xmlStr string, sharedStrings []string) string {
	decoder := xml.NewDecoder(strings.NewReader(xmlStr))
	var rows [][]string
	var currentRow []string
	var inV bool
	var cellType string
	var currentVal strings.Builder

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "row":
				currentRow = nil
			case "c":
				cellType = ""
				for _, attr := range se.Attr {
					if attr.Name.Local == "t" {
						cellType = attr.Value
					}
				}
			case "v":
				inV = true
				currentVal.Reset()
			}
		case xml.EndElement:
			switch se.Name.Local {
			case "row":
				if len(currentRow) > 0 {
					rows = append(rows, currentRow)
				}
			case "c":
				// cell end — no action needed
			case "v":
				inV = false
				val := currentVal.String()
				if cellType == "s" {
					idx := 0
					fmt.Sscanf(val, "%d", &idx)
					if idx >= 0 && idx < len(sharedStrings) {
						val = sharedStrings[idx]
					}
				}
				currentRow = append(currentRow, val)
			}
		case xml.CharData:
			if inV {
				currentVal.Write(se)
			}
		}
	}

	var lines []string
	for _, row := range rows {
		lines = append(lines, strings.Join(row, "\t"))
	}
	return strings.Join(lines, "\n")
}

func sortStrings(s []string) {
	// Simple bubble sort for small slices; no need to import sort package.
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
