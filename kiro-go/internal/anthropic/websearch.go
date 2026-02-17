package anthropic

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/common"
	"kiro-go/internal/kiro"

	"github.com/google/uuid"
)

// ── MCP Types ──

type mcpRequest struct {
	ID      string    `json:"id"`
	JSONRPC string    `json:"jsonrpc"`
	Method  string    `json:"method"`
	Params  mcpParams `json:"params"`
}

type mcpParams struct {
	Name      string       `json:"name"`
	Arguments mcpArguments `json:"arguments"`
}

type mcpArguments struct {
	Query string `json:"query"`
}

type mcpResponse struct {
	Error   *mcpError  `json:"error,omitempty"`
	ID      string     `json:"id"`
	JSONRPC string     `json:"jsonrpc"`
	Result  *mcpResult `json:"result,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type webSearchResults struct {
	Results      []webSearchResult `json:"results"`
	TotalResults int               `json:"totalResults"`
	Query        string            `json:"query"`
	Error        string            `json:"error"`
}

type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// HasWebSearchTool 检查请求是否为纯 WebSearch 请求
func HasWebSearchTool(req *MessagesRequest) bool {
	if len(req.Tools) != 1 {
		return false
	}
	var tool map[string]interface{}
	if json.Unmarshal(req.Tools[0], &tool) != nil {
		return false
	}
	name, _ := tool["name"].(string)
	return name == "web_search"
}

// ExtractSearchQuery 从消息中提取搜索查询
func ExtractSearchQuery(req *MessagesRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}
	msg := req.Messages[0]

	// 尝试解析为字符串
	var text string
	if json.Unmarshal(msg.Content, &text) == nil {
		return stripSearchPrefix(text)
	}

	// 尝试解析为数组
	var blocks []map[string]interface{}
	if json.Unmarshal(msg.Content, &blocks) == nil && len(blocks) > 0 {
		if blocks[0]["type"] == "text" {
			if t, ok := blocks[0]["text"].(string); ok {
				return stripSearchPrefix(t)
			}
		}
	}
	return ""
}

func stripSearchPrefix(text string) string {
	const prefix = "Perform a web search for the query: "
	if strings.HasPrefix(text, prefix) {
		return text[len(prefix):]
	}
	return text
}

// HandleWebSearchRequest 处理 WebSearch 请求
func HandleWebSearchRequest(w http.ResponseWriter, req *MessagesRequest, provider *kiro.Provider) {
	query := ExtractSearchQuery(req)
	if query == "" {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "无法从消息中提取搜索查询")
		return
	}

	// 创建 MCP 请求
	toolUseID := "srvtoolu_" + uuid.New().String()[:24]
	mcpReq := createMCPRequest(query)

	// 调用 MCP API
	var searchResults *webSearchResults
	mcpBody, _ := json.Marshal(mcpReq)
	resp, err := provider.CallMCP(mcpBody)
	if err == nil {
		defer resp.Body.Close()
		var mcpResp mcpResponse
		if json.NewDecoder(resp.Body).Decode(&mcpResp) == nil && mcpResp.Result != nil {
			if len(mcpResp.Result.Content) > 0 && mcpResp.Result.Content[0].Type == "text" {
				var results webSearchResults
				if json.Unmarshal([]byte(mcpResp.Result.Content[0].Text), &results) == nil {
					searchResults = &results
				}
			}
		}
	}

	// 生成 SSE 响应
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	msgID := "msg_" + uuid.New().String()[:24]

	writeSSE := func(event string, data interface{}) {
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
		flusher.Flush()
	}

	// 1. message_start
	writeSSE("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": msgID, "type": "message", "role": "assistant", "model": req.Model,
			"content": []interface{}{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 0, "output_tokens": 0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
		},
	})

	// 2. content_block_start (server_tool_use)
	writeSSE("content_block_start", map[string]interface{}{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]interface{}{"id": toolUseID, "type": "server_tool_use", "name": "web_search", "input": map[string]interface{}{}},
	})

	// 3. content_block_delta (input_json_delta)
	inputJSON, _ := json.Marshal(map[string]string{"query": query})
	writeSSE("content_block_delta", map[string]interface{}{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": string(inputJSON)},
	})

	// 4. content_block_stop (server_tool_use)
	writeSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": 0})

	// 5. content_block_start (web_search_tool_result)
	var searchContent []map[string]interface{}
	if searchResults != nil {
		for _, r := range searchResults.Results {
			searchContent = append(searchContent, map[string]interface{}{
				"type": "web_search_result", "title": r.Title, "url": r.URL,
				"encrypted_content": r.Snippet, "page_age": nil,
			})
		}
	}
	writeSSE("content_block_start", map[string]interface{}{
		"type": "content_block_start", "index": 1,
		"content_block": map[string]interface{}{"type": "web_search_tool_result", "tool_use_id": toolUseID, "content": searchContent},
	})

	// 6. content_block_stop (web_search_tool_result)
	writeSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": 1})

	// 7. content_block_start (text)
	writeSSE("content_block_start", map[string]interface{}{
		"type": "content_block_start", "index": 2,
		"content_block": map[string]interface{}{"type": "text", "text": ""},
	})

	// 8. 生成搜索摘要并分块发送
	summary := generateSearchSummary(query, searchResults)
	runes := []rune(summary)
	chunkSize := 100
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		writeSSE("content_block_delta", map[string]interface{}{
			"type": "content_block_delta", "index": 2,
			"delta": map[string]interface{}{"type": "text_delta", "text": string(runes[i:end])},
		})
	}

	// 9. content_block_stop (text)
	writeSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": 2})

	// 10. message_delta
	outputTokens := (len(summary) + 3) / 4
	writeSSE("message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": outputTokens},
	})

	// 11. message_stop
	writeSSE("message_stop", map[string]interface{}{"type": "message_stop"})
}

func createMCPRequest(query string) mcpRequest {
	random22 := randomAlphaNum(22)
	timestamp := time.Now().UnixMilli()
	random8 := randomLowerAlphaNum(8)

	return mcpRequest{
		ID:      fmt.Sprintf("web_search_tooluse_%s_%d_%s", random22, timestamp, random8),
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: mcpParams{
			Name:      "web_search",
			Arguments: mcpArguments{Query: query},
		},
	}
}

func generateSearchSummary(query string, results *webSearchResults) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Here are the search results for \"%s\":\n\n", query)

	if results != nil && len(results.Results) > 0 {
		for i, r := range results.Results {
			fmt.Fprintf(&sb, "%d. **%s**\n", i+1, r.Title)
			if r.Snippet != "" {
				snippet := r.Snippet
				runes := []rune(snippet)
				if len(runes) > 200 {
					snippet = string(runes[:200]) + "..."
				}
				fmt.Fprintf(&sb, "   %s\n", snippet)
			}
			fmt.Fprintf(&sb, "   Source: %s\n\n", r.URL)
		}
	} else {
		sb.WriteString("No results found.\n")
	}

	sb.WriteString("\nPlease note that these are web search results and may not be fully accurate or up-to-date.")
	return sb.String()
}

const alphaNum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const lowerAlphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomAlphaNum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphaNum[rand.Intn(len(alphaNum))]
	}
	return string(b)
}

func randomLowerAlphaNum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = lowerAlphaNum[rand.Intn(len(lowerAlphaNum))]
	}
	return string(b)
}
