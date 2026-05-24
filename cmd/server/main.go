package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	appconfig "github.com/angoo/s3-mcp/internal/config"
	"github.com/angoo/s3-mcp/internal/indexer"
	mcpserver "github.com/angoo/s3-mcp/internal/mcp"
	sqspoller "github.com/angoo/s3-mcp/internal/sqs"
	"github.com/angoo/s3-mcp/internal/store"
)

func main() {
	cfg, err := appconfig.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logDiagnostics()

	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3EndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.S3EndpointURL)
		}
	})

	log.Printf("starting s3-mcp server")
	log.Printf("  bucket: %s", cfg.S3Bucket)
	log.Printf("  region: %s", cfg.AWSRegion)
	if cfg.S3EndpointURL != "" {
		log.Printf("  s3 endpoint: %s", cfg.S3EndpointURL)
	}
	log.Printf("  embedding: %s", cfg.EmbeddingModel)
	log.Printf("  port: %d", cfg.MCPPort)
	log.Printf("  chunk_size: %d, overlap: %d", cfg.ChunkSize, cfg.ChunkOverlap)
	log.Printf("  concurrency: %d", cfg.Concurrency)
	if cfg.S3Prefix != "" {
		log.Printf("  prefix: %s", cfg.S3Prefix)
	}
	if cfg.SQSQueueURL != "" {
		log.Printf("  sqs: %s", cfg.SQSQueueURL)
	}

	log.Printf("initializing vector store...")
	st, err := store.New(ctx, cfg.OpenAIKey, cfg.OpenAIBaseURL, cfg.EmbeddingModel, cfg.Concurrency)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	idx := indexer.New(indexer.Config{
		Bucket:        cfg.S3Bucket,
		Prefix:        cfg.S3Prefix,
		ChunkSize:     cfg.ChunkSize,
		ChunkOverlap:  cfg.ChunkOverlap,
		MaxFileSizeMB: cfg.MaxFileSizeMB,
		Concurrency:   cfg.Concurrency,
	}, s3Client, st)

	log.Printf("indexing S3 bucket contents...")
	total, indexed, err := idx.Run(ctx)
	if err != nil {
		log.Fatalf("indexing: %v", err)
	}
	log.Printf("indexing complete: %d/%d objects indexed, %d chunks total", indexed, total, st.Count())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	mcpSrv := mcpserver.New(st, s3Client, cfg.S3Bucket)
	go func() {
		if err := mcpSrv.Start(ctx, cfg.MCPPort); err != nil {
			log.Printf("mcp server error: %v", err)
			cancel()
		}
	}()

	if cfg.SQSQueueURL != "" {
		sqsClient := sqs.NewFromConfig(awsCfg)
		sqsHandler := &handler{indexer: idx, store: st, prefix: cfg.S3Prefix}
		poller := sqspoller.New(sqsClient, cfg.SQSQueueURL, sqsHandler)
		go func() {
			if err := poller.Run(ctx); err != nil {
				log.Printf("sqs poller error: %v", err)
			}
		}()
	}

	log.Printf("server ready")

	<-sigCh
	log.Printf("shutting down...")
	cancel()
	log.Printf("shutdown complete")
}

func loadAWSConfig(ctx context.Context, cfg *appconfig.Config) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.AWSRegion),
	}
	if cfg.S3NoSignRequest {
		opts = append(opts, config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

type handler struct {
	indexer *indexer.Indexer
	store   *store.Store
	prefix  string
}

func (h *handler) OnObjectCreated(ctx context.Context, key string) error {
	if h.prefix != "" && !strings.HasPrefix(key, h.prefix) {
		return nil
	}
	return h.indexer.ProcessSingle(ctx, key)
}

func (h *handler) OnObjectRemoved(ctx context.Context, key string) error {
	if h.prefix != "" && !strings.HasPrefix(key, h.prefix) {
		return nil
	}
	return h.store.Remove(ctx, key)
}

func logDiagnostics() {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err == nil {
		log.Printf("[diag] RLIMIT_NOFILE: soft=%d hard=%d", limit.Cur, limit.Max)
	} else {
		log.Printf("[diag] RLIMIT_NOFILE: <unavailable: %v>", err)
	}

	for _, p := range []string{
		"/proc/sys/fs/inotify/max_user_watches",
		"/proc/sys/fs/inotify/max_user_instances",
		"/proc/sys/fs/inotify/max_queued_events",
	} {
		if data, err := os.ReadFile(p); err == nil {
			log.Printf("[diag] %s = %s", p, strings.TrimSpace(string(data)))
		}
	}

	log.Printf("[diag] GOMAXPROCS=%d, num goroutines=%d", runtime.GOMAXPROCS(0), runtime.NumGoroutine())
}
