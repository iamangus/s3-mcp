package extractor

import (
	"io"
	"path/filepath"
	"strings"
)

type Extractor interface {
	Extract(r io.Reader) (string, error)
}

var registry = map[string]Extractor{}

func Register(ext string, e Extractor) {
	registry[strings.ToLower(ext)] = e
}

func GetExt(key string) (Extractor, bool) {
	ext := strings.ToLower(filepath.Ext(key))
	e, ok := registry[ext]
	return e, ok
}

func SupportedExtensions() []string {
	exts := make([]string, 0, len(registry))
	for k := range registry {
		exts = append(exts, k)
	}
	return exts
}
