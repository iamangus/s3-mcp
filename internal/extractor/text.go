package extractor

import (
	"io"
)

type textExtractor struct{}

func (e *textExtractor) Extract(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var textExts = []string{
	".txt", ".md", ".rst", ".log",
	".csv", ".tsv",
	".json", ".jsonl", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf",
	".xml", ".svg",
	".sh", ".bash", ".zsh",
	".py", ".js", ".ts", ".jsx", ".tsx", ".go", ".rs", ".java", ".c", ".cpp",
	".h", ".hpp", ".rb", ".php", ".swift", ".kt", ".scala", ".r", ".sql", ".lua",
	".env", ".gitignore",
	"", ".dockerignore",
}

func init() {
	t := &textExtractor{}
	for _, ext := range textExts {
		Register(ext, t)
	}
}
