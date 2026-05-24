package extractor

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

type docxExtractor struct{}

func (e *docxExtractor) Extract(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return extractDocxXML(rc)
	}

	return "", nil
}

func extractDocxXML(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var buf strings.Builder
	inT := false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return strings.TrimSpace(buf.String()), nil
		}
		if err != nil {
			return "", err
		}

		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "t" {
				inT = true
			} else if el.Name.Local == "p" && buf.Len() > 0 {
				buf.WriteByte('\n')
			}
		case xml.EndElement:
			if el.Name.Local == "t" {
				inT = false
			}
		case xml.CharData:
			if inT {
				buf.Write(el)
			}
		}
	}
}

func init() {
	Register(".docx", &docxExtractor{})
}
