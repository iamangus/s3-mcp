package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/angoo/s3-mcp/internal/store"
)

type Server struct {
	store    *store.Store
	s3Client *s3.Client
	bucket   string
}

func New(st *store.Store, s3Client *s3.Client, bucket string) *Server {
	return &Server{
		store:    st,
		s3Client: s3Client,
		bucket:   bucket,
	}
}

func (s *Server) Start(ctx context.Context, port int) error {
	mcpServer := server.NewMCPServer(
		"s3-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
		server.WithLogging(),
	)

	mcpServer.AddTool(
		mcpgo.NewTool("search_documents",
			mcpgo.WithDescription("Semantic search across the contents of the S3 bucket. Returns relevant document chunks with similarity scores."),
			mcpgo.WithString("query",
				mcpgo.Required(),
				mcpgo.Description("The search query to find relevant documents"),
			),
			mcpgo.WithNumber("limit",
				mcpgo.Description("Maximum number of results to return (default: 10)"),
				mcpgo.DefaultNumber(10),
			),
		),
		s.handleSearchDocuments,
	)

	mcpServer.AddTool(
		mcpgo.NewTool("get_document",
			mcpgo.WithDescription("Retrieve the full contents of a document from the S3 bucket by its key."),
			mcpgo.WithString("key",
				mcpgo.Required(),
				mcpgo.Description("The S3 object key (path) of the document to retrieve"),
			),
		),
		s.handleGetDocument,
	)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[mcp] starting StreamableHTTP server on %s", addr)

	httpServer := server.NewStreamableHTTPServer(mcpServer)

	go func() {
		<-ctx.Done()
		log.Printf("[mcp] shutting down")
	}()

	return httpServer.Start(addr)
}

func (s *Server) handleSearchDocuments(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	query, _ := req.RequireString("query")
	limit := int(req.GetFloat("limit", 10))

	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	results, err := s.store.Search(ctx, query, limit)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	type resultItem struct {
		Key        string  `json:"key"`
		Content    string  `json:"content"`
		Similarity float32 `json:"similarity"`
	}

	items := make([]resultItem, len(results))
	for i, r := range results {
		items[i] = resultItem{
			Key:        r.Key,
			Content:    r.Content,
			Similarity: r.Similarity,
		}
	}

	out, _ := json.MarshalIndent(items, "", "  ")
	return mcpgo.NewToolResultText(string(out)), nil
}

func (s *Server) handleGetDocument(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	key, _ := req.RequireString("key")

	output, err := s.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("failed to get document '%s': %v", key, err)), nil
	}
	defer output.Body.Close()

	body, err := io.ReadAll(io.LimitReader(output.Body, 5*1024*1024))
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("failed to read document '%s': %v", key, err)), nil
	}

	contentType := aws.ToString(output.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	displayContent := string(body)
	if !isTextContent(contentType) {
		displayContent = fmt.Sprintf("[binary content, %d bytes, type: %s]", len(body), contentType)
	}

	type docResult struct {
		Key          string `json:"key"`
		Size         int64  `json:"size"`
		LastModified string `json:"last_modified"`
		ContentType  string `json:"content_type"`
		Content      string `json:"content"`
	}

	res := docResult{
		Key:          key,
		Size:         aws.ToInt64(output.ContentLength),
		LastModified: output.LastModified.String(),
		ContentType:  contentType,
		Content:      displayContent,
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	return mcpgo.NewToolResultText(string(out)), nil
}

func isTextContent(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") ||
		contentType == "application/json" ||
		contentType == "application/xml" ||
		contentType == "application/javascript" ||
		contentType == "application/x-yaml"
}
