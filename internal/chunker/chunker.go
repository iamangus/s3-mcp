package chunker

import (
	"strings"
)

type Result struct {
	Content     string
	ChunkIndex  int
	TotalChunks int
}

type Chunker struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

func New(chunkSize, chunkOverlap int) *Chunker {
	return &Chunker{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		Separators:   []string{"\n\n", "\n", ". ", "? ", "! ", ", ", " ", ""},
	}
}

func (c *Chunker) Split(text string) []Result {
	if len(text) <= c.ChunkSize {
		return []Result{{Content: text, ChunkIndex: 0, TotalChunks: 1}}
	}

	chunks := c.splitRecursive(text, c.Separators)
	total := len(chunks)
	results := make([]Result, total)
	for i, chunk := range chunks {
		results[i] = Result{
			Content:     chunk,
			ChunkIndex:  i,
			TotalChunks: total,
		}
	}
	return results
}

func (c *Chunker) splitRecursive(text string, separators []string) []string {
	if len(text) <= c.ChunkSize {
		if text == "" {
			return nil
		}
		return []string{text}
	}

	if len(separators) == 0 {
		return c.splitHard(text)
	}

	sep := separators[0]
	remainingSeps := separators[1:]

	parts := strings.Split(text, sep)
	var chunks []string
	var current string

	for _, part := range parts {
		if len(current)+len(part) <= c.ChunkSize {
			if current == "" {
				current = part
			} else {
				current += sep + part
			}
		} else {
			if current != "" {
				subs := c.splitRecursive(current, remainingSeps)
				chunks = append(chunks, subs...)
			}
			current = part
		}
	}

	if current != "" {
		if len(current) <= c.ChunkSize {
			chunks = append(chunks, current)
		} else {
			subs := c.splitRecursive(current, remainingSeps)
			chunks = append(chunks, subs...)
		}
	}

	if c.ChunkOverlap > 0 && len(chunks) > 1 {
		var merged []string
		merged = append(merged, chunks[0])
		for i := 1; i < len(chunks); i++ {
			prev := chunks[i-1]
			if len(prev) > c.ChunkOverlap {
				overlapPortion := prev[len(prev)-c.ChunkOverlap:]
				chunks[i] = overlapPortion + chunks[i]
			}
			merged = append(merged, chunks[i])
		}
		chunks = merged
	}

	return chunks
}

func (c *Chunker) splitHard(text string) []string {
	var chunks []string
	runes := []rune(text)

	for i := 0; i < len(runes); i += c.ChunkSize {
		end := i + c.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
