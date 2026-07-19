package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestElasticsearchIndexMappingsUseIKAnalyzers(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		fields []string
		init   func(context.Context, *ElasticsearchIndexer) error
	}{
		{
			name:   "messages",
			path:   "/allcallall_messages",
			fields: []string{"body", "sender_display_name"},
			init: func(ctx context.Context, indexer *ElasticsearchIndexer) error {
				return indexer.InitMessageIndex(ctx)
			},
		},
		{
			name:   "context chunks",
			path:   "/" + contextChunkIndexName,
			fields: []string{"content", "keywords", "source_title"},
			init: func(ctx context.Context, indexer *ElasticsearchIndexer) error {
				return indexer.InitChunkIndex(ctx)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPut || request.URL.Path != test.path {
					t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
				}
				var payload struct {
					Mappings struct {
						Properties map[string]map[string]any `json:"properties"`
					} `json:"mappings"`
				}
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatalf("decode index mapping: %v", err)
				}
				for _, fieldName := range test.fields {
					field := payload.Mappings.Properties[fieldName]
					if field["analyzer"] != ikIndexAnalyzer || field["search_analyzer"] != ikSearchAnalyzer {
						t.Fatalf("field %q does not use IK analyzers: %#v", fieldName, field)
					}
				}
				writer.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			indexer := newTestElasticsearchIndexer(t, server.URL)
			if err := test.init(context.Background(), indexer); err != nil {
				t.Fatalf("initialize index: %v", err)
			}
		})
	}
}

func TestElasticsearchIndexInitializationRejectsUnexpectedBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"type":"illegal_argument_exception","reason":"unknown analyzer"}}`))
	}))
	defer server.Close()

	indexer := newTestElasticsearchIndexer(t, server.URL)
	err := indexer.InitChunkIndex(context.Background())
	if err == nil || !strings.Contains(err.Error(), "illegal_argument_exception") {
		t.Fatalf("expected analyzer configuration error, got %v", err)
	}
}

func TestElasticsearchExistingIndexRequiresCompatibleIKMapping(t *testing.T) {
	for _, test := range []struct {
		name     string
		analyzer string
		search   string
		wantErr  bool
	}{
		{name: "compatible", analyzer: ikIndexAnalyzer, search: ikSearchAnalyzer},
		{name: "incompatible", analyzer: "standard", search: "standard", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodPut:
					writer.WriteHeader(http.StatusBadRequest)
					_, _ = writer.Write([]byte(`{"error":{"type":"resource_already_exists_exception"}}`))
				case http.MethodGet:
					_ = json.NewEncoder(writer).Encode(map[string]any{
						"allcallall_messages": map[string]any{
							"mappings": map[string]any{
								"properties": map[string]any{
									"body": map[string]any{
										"analyzer":        test.analyzer,
										"search_analyzer": test.search,
									},
									"sender_display_name": map[string]any{
										"analyzer":        test.analyzer,
										"search_analyzer": test.search,
									},
								},
							},
						},
					})
				default:
					t.Fatalf("unexpected method: %s", request.Method)
				}
			}))
			defer server.Close()

			indexer := newTestElasticsearchIndexer(t, server.URL)
			err := indexer.InitMessageIndex(context.Background())
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "reindex is required") {
					t.Fatalf("expected reindex error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("compatible mapping rejected: %v", err)
			}
		})
	}
}

func newTestElasticsearchIndexer(t *testing.T, serverURL string) *ElasticsearchIndexer {
	t.Helper()
	indexer, err := NewElasticsearchIndexer(ElasticsearchConfig{URL: serverURL})
	if err != nil {
		t.Fatalf("new elasticsearch indexer: %v", err)
	}
	return indexer
}

func TestElasticsearchIKChineseSearchContract(t *testing.T) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ALLCALLALL_TEST_ELASTICSEARCH_URL")), "/")
	if baseURL == "" {
		t.Skip("ALLCALLALL_TEST_ELASTICSEARCH_URL is not configured")
	}
	indexName := strings.ToLower("allcallall_messages_ik_" + time.Now().Format("20060102_150405_000000000"))
	indexer, err := NewElasticsearchIndexer(ElasticsearchConfig{URL: baseURL, Index: indexName})
	if err != nil {
		t.Fatalf("new elasticsearch indexer: %v", err)
	}
	t.Cleanup(func() {
		request, requestErr := http.NewRequest(http.MethodDelete, baseURL+"/"+indexName, nil)
		if requestErr == nil {
			response, responseErr := http.DefaultClient.Do(request)
			if responseErr == nil {
				_ = response.Body.Close()
			}
		}
	})

	ctx := context.Background()
	if err := indexer.InitMessageIndex(ctx); err != nil {
		t.Fatalf("initialize IK message index: %v", err)
	}
	if err := indexer.InitMessageIndex(ctx); err != nil {
		t.Fatalf("verify existing IK message index: %v", err)
	}
	if err := indexer.IndexMessage(ctx, MessageDocument{
		ID:             "message:1",
		OrganizationID: 1,
		ConversationID: 2,
		MessageID:      1,
		Body:           "供应商准入需要完成安全审批流程和法务复核。",
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("index Chinese message: %v", err)
	}
	refreshRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/"+indexName+"/_refresh",
		nil,
	)
	if err != nil {
		t.Fatalf("create refresh request: %v", err)
	}
	refreshResponse, err := http.DefaultClient.Do(refreshRequest)
	if err != nil {
		t.Fatalf("refresh message index: %v", err)
	}
	_ = refreshResponse.Body.Close()
	if refreshResponse.StatusCode < 200 || refreshResponse.StatusCode >= 300 {
		t.Fatalf("refresh message index: status=%d", refreshResponse.StatusCode)
	}

	results, err := indexer.SearchMessages(ctx, MessageSearchQuery{
		OrganizationID: 1,
		Query:          "审批流程",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("search Chinese message: %v", err)
	}
	if len(results) != 1 || results[0].MessageID != 1 {
		t.Fatalf("unexpected Chinese search results: %+v", results)
	}
}
