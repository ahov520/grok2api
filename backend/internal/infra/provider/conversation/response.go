package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponseOptions 保留无法直接交给 Responses 上游执行的下游协议语义。
type ResponseOptions struct {
	AnthropicThinking          bool
	AnthropicWebSearch         bool
	AnthropicWebSearchRequired bool
	AnthropicWebSearchQuery    string
	StopSequences              []string
}

type responseEnvelope struct {
	ID        string         `json:"id"`
	Model     string         `json:"model"`
	Status    string         `json:"status"`
	CreatedAt int64          `json:"created_at"`
	Output    []responseItem `json:"output"`
	Usage     responseUsage  `json:"usage"`
	Error     any            `json:"error"`
}

type responseItem struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Role      string            `json:"role"`
	Status    string            `json:"status"`
	Content   []responseContent `json:"content"`
	Summary   []responseContent `json:"summary"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Arguments string            `json:"arguments"`
	Encrypted string            `json:"encrypted_content"`
	// Action is populated for Build hosted tool items such as web_search_call.
	Action map[string]any `json:"action"`
}

type responseContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Refusal     string `json:"refusal"`
	Annotations []any  `json:"annotations"`
}

type responseUsage struct {
	InputTokens            int64 `json:"input_tokens"`
	OutputTokens           int64 `json:"output_tokens"`
	TotalTokens            int64 `json:"total_tokens"`
	CostInUSDTicks         int64 `json:"cost_in_usd_ticks"`
	NumSourcesUsed         int64 `json:"num_sources_used"`
	NumServerSideToolsUsed int64 `json:"num_server_side_tools_used"`
	InputTokensDetails     struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	ContextDetails struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"context_details"`
}

type parsedResponse struct {
	ID           string
	Model        string
	CreatedAt    int64
	Text         string
	Reasoning    string
	Signature    string
	Refusal      string
	Calls        []responseItem
	WebSearch    []webSearchCall
	Annotations  []any
	Usage        responseUsage
	Status       string
	StopSequence string
}

// ConvertResponseJSON 将 Responses 非流式结果转换为 Chat Completions 或 Anthropic Messages。
func ConvertResponseJSON(body []byte, operation string) ([]byte, error) {
	return ConvertResponseJSONWithOptions(body, operation, ResponseOptions{})
}

// ConvertResponseJSONWithOptions 按下游协议选项恢复 thinking、搜索与 stop sequence。
func ConvertResponseJSONWithOptions(body []byte, operation string, options ResponseOptions) ([]byte, error) {
	if operation == OperationResponses {
		return body, nil
	}
	if errorValue, ok := decodeResponseError(body); ok {
		if operation == OperationMessages {
			return anthropicErrorJSON(errorValue), nil
		}
		return openAIErrorJSON(errorValue), nil
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("解析 Responses 响应: %w", err)
	}
	parsed := parseResponse(envelope)
	if operation == OperationMessages || operation == OperationChat {
		parsed.Text, parsed.StopSequence = applyStopSequences(parsed.Text, options.StopSequences)
	}
	if operation == OperationMessages {
		if !options.AnthropicWebSearch {
			parsed.WebSearch = nil
		}
	}
	var result any
	if operation == OperationMessages {
		result = messagesResponse(parsed, options)
	} else {
		result = chatResponse(parsed)
	}
	return json.Marshal(result)
}

func parseResponse(value responseEnvelope) parsedResponse {
	parsed := parsedResponse{ID: value.ID, Model: value.Model, CreatedAt: value.CreatedAt, Usage: value.Usage, Status: value.Status}
	if parsed.CreatedAt == 0 {
		parsed.CreatedAt = time.Now().Unix()
	}
	var annotations []map[string]any
	for _, item := range value.Output {
		switch item.Type {
		case "message":
			annotations = append(annotations, extractMessageAnnotations(item)...)
			for _, content := range item.Content {
				parsed.Annotations = append(parsed.Annotations, content.Annotations...)
				switch content.Type {
				case "output_text":
					parsed.Text += content.Text
				case "refusal":
					parsed.Refusal += content.Refusal
				}
			}
		case "reasoning":
			reasoning := ""
			for _, content := range item.Content {
				if content.Type == "reasoning_text" {
					reasoning += content.Text
				}
			}
			if reasoning == "" {
				for _, summary := range item.Summary {
					reasoning += summary.Text
				}
			}
			parsed.Reasoning += reasoning
			if item.Encrypted != "" {
				parsed.Signature = item.Encrypted
			}
		case "function_call":
			parsed.Calls = append(parsed.Calls, item)
		case "web_search_call":
			// Cap candidates early so pathological upstream envelopes cannot
			// retain unbounded intermediate search state before dedupe.
			if len(parsed.WebSearch) >= maxWebSearchCalls {
				continue
			}
			if call, ok := parseWebSearchCallItem(item); ok {
				parsed.WebSearch = append(parsed.WebSearch, call)
			}
		}
	}
	if len(parsed.WebSearch) > 0 {
		parsed.WebSearch = dedupeWebSearchCalls(parsed.WebSearch)
		parsed.WebSearch = mergeAnnotationTitles(parsed.WebSearch, annotations)
	}
	return parsed
}

func decodeResponseError(body []byte) (any, bool) {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return nil, false
	}
	if value, exists := root["error"]; exists && value != nil {
		return value, true
	}
	if response, ok := root["response"].(map[string]any); ok {
		if value, exists := response["error"]; exists && value != nil {
			return value, true
		}
	}
	if typeName, _ := root["type"].(string); typeName == "error" || typeName == "response.failed" {
		return root, true
	}
	return nil, false
}

func responseErrorDetails(value any) (map[string]any, string) {
	message := "Upstream request failed"
	if object, ok := value.(map[string]any); ok {
		if text, ok := object["message"].(string); ok && strings.TrimSpace(text) != "" {
			message = strings.TrimSpace(text)
		}
		return object, message
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		message = strings.TrimSpace(text)
	}
	return nil, message
}

func openAIErrorJSON(value any) []byte {
	data, _ := json.Marshal(openAIErrorPayload(value))
	return data
}

func openAIErrorPayload(value any) map[string]any {
	object, message := responseErrorDetails(value)
	code := "upstream_error"
	errorType := "server_error"
	if object != nil {
		if candidate, ok := object["code"].(string); ok && strings.TrimSpace(candidate) != "" {
			code = strings.TrimSpace(candidate)
		}
	}
	normalized := strings.ToLower(message + " " + code)
	switch {
	case strings.Contains(normalized, "capacity"), strings.Contains(normalized, "high demand"), strings.Contains(normalized, "overloaded"):
		code = "model_capacity_exceeded"
	case strings.Contains(normalized, "rate limit"), strings.Contains(normalized, "rate_limit"), strings.Contains(normalized, "too many requests"):
		code = "rate_limit_exceeded"
		errorType = "rate_limit_error"
	}
	return map[string]any{"error": map[string]any{
		"message": message, "type": errorType, "code": code, "param": nil,
	}}
}

// chatStreamErrorPayload 归一化 chat 流式错误。默认类型 api_error（与上游契约一致）；
// 容量/限流错误额外携带 code 便于客户端识别与重试。
func chatStreamErrorPayload(value any) map[string]any {
	object, message := responseErrorDetails(value)
	result := map[string]any{"message": message, "type": "api_error"}
	if object != nil {
		if candidate, ok := object["type"].(string); ok && strings.TrimSpace(candidate) != "" {
			result["type"] = strings.TrimSpace(candidate)
		}
	}
	code := "upstream_error"
	if object != nil {
		if candidate, ok := object["code"].(string); ok && strings.TrimSpace(candidate) != "" {
			code = strings.TrimSpace(candidate)
		}
	}
	normalized := strings.ToLower(message + " " + code)
	switch {
	case strings.Contains(normalized, "capacity"), strings.Contains(normalized, "high demand"), strings.Contains(normalized, "overloaded"):
		code = "model_capacity_exceeded"
		result["type"] = "api_error"
	case strings.Contains(normalized, "rate limit"), strings.Contains(normalized, "rate_limit"), strings.Contains(normalized, "too many requests"):
		code = "rate_limit_exceeded"
		result["type"] = "rate_limit_error"
	}
	if code != "upstream_error" {
		result["code"] = code
	}
	return result
}
