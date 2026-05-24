package extractor

import (
	"bytes"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

type pdfExtractor struct{}

func (e *pdfExtractor) Extract(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	reader, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		if text != "" {
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(text)
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

func init() {
	Register(".pdf", &pdfExtractor{})
}
