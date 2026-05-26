package document

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/nearai/ironclaw-go/internal/channels"
)

func docAttachment(mime, filename string, data []byte) channels.Attachment {
	return channels.Attachment{
		ID:        "doc_1",
		Kind:      channels.AttachmentKindDocument,
		MIMEType:  mime,
		Filename:  filename,
		SizeBytes: int64(len(data)),
		Data:      data,
	}
}

func TestExtractPlainText(t *testing.T) {
	text, err := ExtractText([]byte("Hello world"), "text/plain", "notes.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hello world" {
		t.Errorf("got %q, want Hello world", text)
	}
}

func TestExtractCSV(t *testing.T) {
	data := []byte("name,age\nAlice,30")
	text, err := ExtractText(data, "text/csv", "data.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "name,age\nAlice,30" {
		t.Errorf("got %q", text)
	}
}

func TestExtractJSON(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	text, err := ExtractText(data, "application/json", "data.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "key") {
		t.Errorf("expected key in output, got %q", text)
	}
}

func TestExtractRTF(t *testing.T) {
	data := []byte(`{\rtf1\ansi\pard
This is a test document.\par
}`)
	text, err := ExtractText(data, "application/rtf", "doc.rtf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "test") {
		t.Errorf("expected 'test' in output, got %q", text)
	}
}

func TestExtractDOCX(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
		<w:body><w:p><w:r><w:t>Hello DOCX</w:t></w:r></w:p></w:body>
	</w:document>`))
	zw.Close()

	text, err := ExtractText(buf.Bytes(), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "test.docx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello DOCX") {
		t.Errorf("expected 'Hello DOCX' in output, got %q", text)
	}
}

func TestExtractPPTX(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("ppt/slides/slide1.xml")
	w.Write([]byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
		<p:sp><p:txBody><a:p><a:r><a:t>Slide One</a:t></a:r></a:p></p:txBody></p:sp>
	</p:sld>`))
	w2, _ := zw.Create("ppt/slides/slide2.xml")
	w2.Write([]byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
		<p:sp><p:txBody><a:p><a:r><a:t>Slide Two</a:t></a:r></a:p></p:txBody></p:sp>
	</p:sld>`))
	zw.Close()

	text, err := ExtractText(buf.Bytes(), "application/vnd.openxmlformats-officedocument.presentationml.presentation", "test.pptx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Slide One") {
		t.Errorf("expected 'Slide One' in output, got %q", text)
	}
	if !strings.Contains(text, "Slide Two") {
		t.Errorf("expected 'Slide Two' in output, got %q", text)
	}
}

func TestExtractXLSX(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("xl/sharedStrings.xml")
	w.Write([]byte(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
		<si><t>Shared</t></si><si><t>Value</t></si>
	</sst>`))
	w2, _ := zw.Create("xl/worksheets/sheet1.xml")
	w2.Write([]byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
		<sheetData>
			<row><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row>
		</sheetData>
	</worksheet>`))
	zw.Close()

	text, err := ExtractText(buf.Bytes(), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "test.xlsx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Shared") {
		t.Errorf("expected 'Shared' in output, got %q", text)
	}
	if !strings.Contains(text, "Value") {
		t.Errorf("expected 'Value' in output, got %q", text)
	}
}

func TestExtractByExtensionFallback(t *testing.T) {
	// Unknown MIME but known extension
	text, err := ExtractText([]byte("fallback text"), "application/octet-stream", "data.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "fallback text" {
		t.Errorf("got %q, want 'fallback text'", text)
	}
}

func TestExtractUnsupported(t *testing.T) {
	_, err := ExtractText([]byte("x"), "application/unknown", "file.bin")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestMiddlewareExtractsPlainText(t *testing.T) {
	mw := NewMiddleware()
	msg := channels.IncomingMessage{
		Content: "check this",
		Attachments: []channels.Attachment{
			docAttachment("text/plain", "notes.txt", []byte("Hello world")),
		},
	}
	mw.Process(&msg)
	if msg.Attachments[0].ExtractedText != "Hello world" {
		t.Errorf("got %q, want Hello world", msg.Attachments[0].ExtractedText)
	}
}

func TestMiddlewareSkipsAlreadyExtracted(t *testing.T) {
	mw := NewMiddleware()
	att := docAttachment("text/plain", "test.txt", []byte("data"))
	att.ExtractedText = "Already done"
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{att},
	}
	mw.Process(&msg)
	if msg.Attachments[0].ExtractedText != "Already done" {
		t.Errorf("got %q, want Already done", msg.Attachments[0].ExtractedText)
	}
}

func TestMiddlewareSkipsAudioAttachments(t *testing.T) {
	mw := NewMiddleware()
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{
			{ID: "a1", Kind: channels.AttachmentKindAudio, MIMEType: "audio/ogg", Data: []byte("audio")},
		},
	}
	mw.Process(&msg)
	if msg.Attachments[0].ExtractedText != "" {
		t.Errorf("expected empty, got %q", msg.Attachments[0].ExtractedText)
	}
}

func TestMiddlewareReportsOversized(t *testing.T) {
	mw := NewMiddleware()
	att := docAttachment("text/plain", "huge.txt", []byte{})
	att.SizeBytes = MaxDocumentSize + 1
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{att},
	}
	mw.Process(&msg)
	text := msg.Attachments[0].ExtractedText
	if !strings.Contains(text, "too large") {
		t.Errorf("expected 'too large' error, got %q", text)
	}
}

func TestMiddlewareTruncatesLongText(t *testing.T) {
	mw := NewMiddleware()
	longText := strings.Repeat("x", MaxExtractedTextLen+1000)
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{
			docAttachment("text/plain", "long.txt", []byte(longText)),
		},
	}
	mw.Process(&msg)
	extracted := msg.Attachments[0].ExtractedText
	if len(extracted) > MaxExtractedTextLen+100 {
		t.Errorf("expected truncation, got len=%d", len(extracted))
	}
	if !strings.HasSuffix(extracted, "[... truncated, document too long ...]") {
		t.Errorf("expected truncation suffix, got suffix: %q", extracted[len(extracted)-50:])
	}
}

func TestMiddlewareNoData(t *testing.T) {
	mw := NewMiddleware()
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{
			docAttachment("text/plain", "empty.txt", []byte{}),
		},
	}
	mw.Process(&msg)
	text := msg.Attachments[0].ExtractedText
	if !strings.Contains(text, "no inline data") {
		t.Errorf("expected 'no inline data' error, got %q", text)
	}
}

func TestMiddlewareUnsupportedFormat(t *testing.T) {
	mw := NewMiddleware()
	msg := channels.IncomingMessage{
		Attachments: []channels.Attachment{
			docAttachment("application/unknown", "file.bin", []byte("data")),
		},
	}
	mw.Process(&msg)
	text := msg.Attachments[0].ExtractedText
	if !strings.Contains(text, "not be supported") {
		t.Errorf("expected unsupported error, got %q", text)
	}
}
