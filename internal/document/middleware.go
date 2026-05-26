package document

import (
	"fmt"

	"github.com/nearai/ironclaw-go/internal/channels"
)

// Middleware processes document attachments on incoming messages.
type Middleware struct{}

// NewMiddleware creates a new document extraction middleware.
func NewMiddleware() *Middleware {
	return &Middleware{}
}

// Process extracts text from document attachments on the incoming message.
func (m *Middleware) Process(msg *channels.IncomingMessage) {
	var extractions []struct {
		index int
		text  string
	}

	for i, att := range msg.Attachments {
		if att.Kind != channels.AttachmentKindDocument {
			continue
		}
		if att.ExtractedText != "" {
			continue
		}

		// Check size limit
		if att.SizeBytes > MaxDocumentSize {
			mb := float64(att.SizeBytes) / (1024.0 * 1024.0)
			maxMB := float64(MaxDocumentSize) / (1024.0 * 1024.0)
			extractions = append(extractions, struct {
				index int
				text  string
			}{
				i,
				fmt.Sprintf("[Document too large for text extraction: %.1f MB exceeds %.0f MB limit. Please send a smaller file or copy-paste the relevant text.]", mb, maxMB),
			})
			continue
		}

		if len(att.Data) == 0 {
			extractions = append(extractions, struct {
				index int
				text  string
			}{
				i,
				"[Document has no inline data. Please try sending the file again.]",
			})
			continue
		}

		if int64(len(att.Data)) > MaxDocumentSize {
			mb := float64(len(att.Data)) / (1024.0 * 1024.0)
			maxMB := float64(MaxDocumentSize) / (1024.0 * 1024.0)
			extractions = append(extractions, struct {
				index int
				text  string
			}{
				i,
				fmt.Sprintf("[Document too large for text extraction: %.1f MB exceeds %.0f MB limit. Please send a smaller file or copy-paste the relevant text.]", mb, maxMB),
			})
			continue
		}

		text, err := ExtractText(att.Data, att.MIMEType, att.Filename)
		if err != nil {
			name := att.Filename
			if name == "" {
				name = "document"
			}
			extractions = append(extractions, struct {
				index int
				text  string
			}{
				i,
				fmt.Sprintf("[Failed to extract text from '%s' (%s): %s. The file format may not be supported.]", name, att.MIMEType, err),
			})
			continue
		}

		// Truncate at a rune boundary to avoid cutting multi-byte UTF-8
		if len(text) > MaxExtractedTextLen {
			boundary := 0
			for idx := range text {
				if idx > MaxExtractedTextLen {
					break
				}
				boundary = idx
			}
			text = text[:boundary] + "\n\n[... truncated, document too long ...]"
		}

		extractions = append(extractions, struct {
			index int
			text  string
		}{i, text})
	}

	for _, e := range extractions {
		msg.Attachments[e.index].ExtractedText = e.text
	}
}
