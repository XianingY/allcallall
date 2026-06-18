package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
)

type readToolExecutor interface {
	ExecuteReadOnlyTool(ctx context.Context, organizationID, userID uint64, toolName, inputJSON string) (string, error)
}

type mcpServer struct {
	organizationID uint64
	userID         uint64
	executor       readToolExecutor
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func main() {
	_ = godotenv.Load()
	log := zerolog.New(os.Stderr).With().Timestamp().Str("component", "mcp_tool_server").Logger()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	db, closeDB, err := appruntime.OpenMigratedDB(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	orgID, err := requiredUintEnv("MCP_ORGANIZATION_ID")
	if err != nil {
		log.Fatal().Err(err).Msg("invalid MCP_ORGANIZATION_ID")
	}
	userID, err := requiredUintEnv("MCP_USER_ID")
	if err != nil {
		log.Fatal().Err(err).Msg("invalid MCP_USER_ID")
	}

	outbox := events.NewStore(db)
	agentSvc := agent.NewService(db, metrics.NewCounterStore())
	agentSvc.WithOutbox(outbox)
	knowledgeSvc := knowledge.NewService(db).WithOutbox(outbox)
	agentSvc.WithKnowledgeRetriever(knowledgeSvc)
	if planner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER")); err == nil {
		agentSvc.WithPlanner(planner)
		if embedder, ok := planner.(knowledge.EmbeddingProvider); ok {
			knowledgeSvc.WithEmbeddingProvider(embedder)
		}
	}
	if chunkIndexer, _, err := appruntime.ChunkIndexerFromEnv(); err == nil && chunkIndexer != nil {
		agentSvc.WithChunkIndexer(chunkIndexer)
		knowledgeSvc.WithChunkIndexer(chunkIndexer)
	}

	server := mcpServer{organizationID: orgID, userID: userID, executor: agentSvc}
	if err := server.serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal().Err(err).Msg("mcp server stopped")
	}
}

func (s mcpServer) serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := writeRPCResponse(writer, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if req.ID == nil {
			continue
		}
		response := s.handle(ctx, req)
		if err := writeRPCResponse(writer, response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s mcpServer) handle(ctx context.Context, req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "allcallall-tool-server", "version": "0.1.0"},
		}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": mcpReadOnlyTools()}}
	case "tools/call":
		result, err := s.handleToolCall(ctx, req.Params)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
	default:
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

func (s mcpServer) handleToolCall(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params toolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	descriptor, ok := agent.ToolDescriptorByName(params.Name)
	if !ok || descriptor.Kind != agent.ToolKindReadOnly {
		return nil, fmt.Errorf("tool %s is not exposed by this MCP server", params.Name)
	}
	input, err := json.Marshal(params.Arguments)
	if err != nil {
		return nil, err
	}
	output, err := s.executor.ExecuteReadOnlyTool(ctx, s.organizationID, s.userID, params.Name, string(input))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": output}},
		"isError": false,
	}, nil
}

func mcpReadOnlyTools() []map[string]any {
	out := make([]map[string]any, 0)
	for _, descriptor := range agent.RegisteredTools() {
		if descriptor.Kind != agent.ToolKindReadOnly {
			continue
		}
		out = append(out, map[string]any{
			"name":        descriptor.Name,
			"description": descriptor.Description,
			"inputSchema": descriptor.InputSchema,
		})
	}
	return out
}

func writeRPCResponse(writer *bufio.Writer, response rpcResponse) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if _, err := writer.Write(raw); err != nil {
		return err
	}
	if err := writer.WriteByte('\n'); err != nil {
		return err
	}
	return writer.Flush()
}

func requiredUintEnv(name string) (uint64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}
