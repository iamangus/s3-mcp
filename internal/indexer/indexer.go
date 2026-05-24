package indexer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/angoo/s3-mcp/internal/chunker"
	"github.com/angoo/s3-mcp/internal/extractor"
	"github.com/angoo/s3-mcp/internal/store"
)

type Config struct {
	Bucket        string
	Prefix        string
	ChunkSize     int
	ChunkOverlap  int
	MaxFileSizeMB int
	Concurrency   int
}

type Indexer struct {
	cfg      Config
	s3Client *s3.Client
	st       *store.Store
	chunker  *chunker.Chunker
}

func New(cfg Config, s3Client *s3.Client, st *store.Store) *Indexer {
	return &Indexer{
		cfg:      cfg,
		s3Client: s3Client,
		st:       st,
		chunker:  chunker.New(cfg.ChunkSize, cfg.ChunkOverlap),
	}
}

func (idx *Indexer) Run(ctx context.Context) (int, int, error) {
	var totalObjects, indexedObjects int
	var mu sync.Mutex
	sem := make(chan struct{}, idx.cfg.Concurrency)
	var wg sync.WaitGroup

	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(idx.cfg.Bucket),
			Prefix:            strPtr(idx.cfg.Prefix),
			ContinuationToken: continuationToken,
		}

		resp, err := idx.s3Client.ListObjectsV2(ctx, input)
		if err != nil {
			return totalObjects, indexedObjects, fmt.Errorf("list objects: %w", err)
		}

		for range resp.Contents {
			totalObjects++
		}

		for _, obj := range resp.Contents {
			sem <- struct{}{}
			wg.Add(1)

			go func(key string, size int64) {
				defer wg.Done()
				defer func() { <-sem }()

				if !idx.shouldProcess(key, size) {
					return
				}

				if err := idx.processObject(ctx, key); err != nil {
					log.Printf("[indexer] error processing %s: %v", key, err)
					return
				}

				mu.Lock()
				indexedObjects++
				mu.Unlock()
			}(aws.ToString(obj.Key), aws.ToInt64(obj.Size))
		}

		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	wg.Wait()

	return totalObjects, indexedObjects, nil
}

func (idx *Indexer) processObject(ctx context.Context, key string) error {
	output, err := idx.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(idx.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer output.Body.Close()

	ext, ok := extractor.GetExt(key)
	if !ok {
		return nil
	}

	text, err := ext.Extract(output.Body)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	chunks := idx.chunker.Split(text)
	if len(chunks) == 0 {
		return nil
	}

	cr := make([]store.ChunkResult, len(chunks))
	for i, c := range chunks {
		cr[i] = store.ChunkResult{Content: c.Content, ChunkIndex: c.ChunkIndex}
	}
	if err := idx.st.AddMulti(ctx, key, cr); err != nil {
		return fmt.Errorf("store add batch: %w", err)
	}

	return nil
}

func (idx *Indexer) shouldProcess(key string, size int64) bool {
	if strings.HasSuffix(key, "/") {
		return false
	}

	if size > int64(idx.cfg.MaxFileSizeMB)*1024*1024 {
		return false
	}

	return true
}

func (idx *Indexer) ProcessSingle(ctx context.Context, key string) error {
	if idx.cfg.Prefix != "" && !strings.HasPrefix(key, idx.cfg.Prefix) {
		return nil
	}
	return idx.processObject(ctx, key)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
