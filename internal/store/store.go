package store

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/philippgille/chromem-go"
)

type DocRef struct {
	Key        string
	ChunkIndex int
	Content    string
	Similarity float32
}

type Store struct {
	mu         sync.RWMutex
	db         *chromem.DB
	collection *chromem.Collection
	keyToIDs   map[string][]string
	count      int
}

func New(ctx context.Context, openAIKey, openAIBaseURL, embeddingModel string, concurrency int) (*Store, error) {
	var ef chromem.EmbeddingFunc
	if openAIBaseURL != "" && openAIBaseURL != chromem.BaseURLOpenAI {
		ef = chromem.NewEmbeddingFuncOpenAICompat(openAIBaseURL, openAIKey, embeddingModel, nil)
	} else {
		ef = chromem.NewEmbeddingFuncOpenAI(openAIKey, chromem.EmbeddingModelOpenAI(embeddingModel))
	}

	db := chromem.NewDB()
	col, err := db.CreateCollection("s3-docs", nil, ef)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	return &Store{
		db:         db,
		collection: col,
		keyToIDs:   make(map[string][]string),
	}, nil
}

func (s *Store) Add(ctx context.Context, key, content string, chunkIndex, totalChunks int) error {
	docID := fmt.Sprintf("%s#%d", key, chunkIndex)
	doc := chromem.Document{
		ID:       docID,
		Content:  content,
		Metadata: map[string]string{"key": key, "chunk_index": fmt.Sprintf("%d", chunkIndex)},
	}

	if err := s.collection.AddDocument(ctx, doc); err != nil {
		return fmt.Errorf("add document: %w", err)
	}

	s.mu.Lock()
	s.keyToIDs[key] = append(s.keyToIDs[key], docID)
	s.count++
	s.mu.Unlock()

	log.Printf("[store] added chunk %s (%d/%d) for key %s", docID, chunkIndex+1, totalChunks, key)
	return nil
}

func (s *Store) Remove(ctx context.Context, key string) error {
	s.mu.RLock()
	ids, ok := s.keyToIDs[key]
	s.mu.RUnlock()

	if !ok || len(ids) == 0 {
		return nil
	}

	if err := s.collection.Delete(ctx, nil, nil, ids...); err != nil {
		return fmt.Errorf("delete documents: %w", err)
	}

	s.mu.Lock()
	delete(s.keyToIDs, key)
	s.count -= len(ids)
	s.mu.Unlock()

	log.Printf("[store] removed key %s (%d chunks)", key, len(ids))
	return nil
}

func (s *Store) Replace(ctx context.Context, key string, chunks []ChunkResult) error {
	if err := s.Remove(ctx, key); err != nil {
		return fmt.Errorf("remove old: %w", err)
	}
	return s.AddMulti(ctx, key, chunks)
}

type ChunkResult struct {
	Content    string
	ChunkIndex int
}

func (s *Store) AddMulti(ctx context.Context, key string, chunks []ChunkResult) error {
	docs := make([]chromem.Document, len(chunks))
	ids := make([]string, len(chunks))

	for i, c := range chunks {
		docID := fmt.Sprintf("%s#%d", key, c.ChunkIndex)
		ids[i] = docID
		docs[i] = chromem.Document{
			ID:       docID,
			Content:  c.Content,
			Metadata: map[string]string{"key": key, "chunk_index": fmt.Sprintf("%d", c.ChunkIndex)},
		}
	}

	if err := s.collection.AddDocuments(ctx, docs, s.dbConcurrency()); err != nil {
		return fmt.Errorf("add documents batch: %w", err)
	}

	s.mu.Lock()
	s.keyToIDs[key] = ids
	s.count += len(ids)
	s.mu.Unlock()

	return nil
}

func (s *Store) dbConcurrency() int {
	return 2
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]DocRef, error) {
	results, err := s.collection.Query(ctx, query, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	refs := make([]DocRef, len(results))
	for i, r := range results {
		refs[i] = DocRef{
			Content:    r.Content,
			Similarity: r.Similarity,
		}

		if r.Metadata != nil {
			refs[i].Key = r.Metadata["key"]
		}
	}

	return refs, nil
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

func (s *Store) KeyCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.keyToIDs)
}
