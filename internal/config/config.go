package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AWSRegion       string
	S3EndpointURL   string
	S3Bucket        string
	S3Prefix        string
	S3NoSignRequest bool
	SQSQueueURL     string
	OpenAIKey       string
	OpenAIBaseURL   string
	EmbeddingModel  string
	MCPPort         int
	ChunkSize       int
	ChunkOverlap    int
	MaxFileSizeMB   int
	Concurrency     int
}

func Load() (*Config, error) {
	cfg := &Config{
		MCPPort:        envInt("MCP_PORT", 8080),
		ChunkSize:      envInt("CHUNK_SIZE", 1000),
		ChunkOverlap:   envInt("CHUNK_OVERLAP", 200),
		MaxFileSizeMB:  envInt("MAX_FILE_SIZE_MB", 10),
		Concurrency:    envInt("CONCURRENCY", 10),
		EmbeddingModel: env("EMBEDDING_MODEL", "text-embedding-3-small"),
		OpenAIBaseURL:  os.Getenv("OPENAI_BASE_URL"),
	}

	cfg.AWSRegion = env("AWS_REGION", "auto")
	cfg.S3EndpointURL = os.Getenv("S3_ENDPOINT_URL")
	cfg.S3Bucket = os.Getenv("S3_BUCKET")
	cfg.S3Prefix = os.Getenv("S3_PREFIX")
	cfg.S3NoSignRequest = envBool("S3_NO_SIGN_REQUEST", false)
	cfg.SQSQueueURL = os.Getenv("SQS_QUEUE_URL")
	cfg.OpenAIKey = os.Getenv("OPENAI_API_KEY")

	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required")
	}
	if cfg.OpenAIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true"
	}
	return fallback
}
