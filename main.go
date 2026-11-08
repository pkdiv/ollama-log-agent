package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Function struct {
		Index     int            `json:"index,omitempty"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Tools     []map[string]any `json:"tools,omitempty"`
	Stream    bool             `json:"stream"`
	Think     *bool            `json:"think,omitempty"`
	Options   map[string]any   `json:"options,omitempty"`
	KeepAlive string           `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Message         Message `json:"message"`
	Done            bool    `json:"done"`
	DoneReason      string  `json:"done_reason"`
	PromptEvalCount int     `json:"prompt_eval_count"`
	EvalCount       int     `json:"eval_count"`
}

var searchLogsTool = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "search_logs",
		"description": "Search the log file for lines matching a regex. Returns at most limit matching lines with line numbers.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Go regexp matched against each line"},
				"limit":   map[string]any{"type": "integer", "description": "Max number of lines to return"},
			},
			"required": []string{"pattern"},
		},
	},
}

func main() {
	think := false
	messages := []Message{{Role: "system", Content: "You analyze app.log. Use search_logs with a regex; try (?i)error|fatal|panic. Answer only from tool results."}}

	req := ChatRequest{
		Model:   "qwen3:8b",
		Tools:   []map[string]any{searchLogsTool},
		Stream:  false,
		Think:   &think,
		Options: map[string]any{"num_ctx": 8192},
	}

	loop_count := 0

	for loop_count < 10 {
		req.Messages = messages
		resp, err := chat(req)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		messages = append(messages, resp.Message)

		if len(resp.Message.ToolCalls) == 0 {
			fmt.Println(resp.Message.Content)
			return
		}

		for _, toolCall := range resp.Message.ToolCalls {

			switch toolCall.Function.Name {
			case "search_logs":
				pattern, ok := toolCall.Function.Arguments["pattern"].(string)
				if !ok || pattern == "" {
					messages = append(messages, Message{
						Role:     "tool",
						ToolName: toolCall.Function.Name,
						Content:  "missing or invalid pattern argument",
					})
					continue
				}
				limit := 0
				if v, ok := toolCall.Function.Arguments["limit"].(float64); ok {
					limit = int(v)
				}
				result := searchLogs(pattern, limit)
				messages = append(messages, Message{Role: "tool", Content: result, ToolName: toolCall.Function.Name})

			default:
				messages = append(messages, Message{
					Role:     "tool",
					ToolName: toolCall.Function.Name,
					Content:  "unknown tool: " + toolCall.Function.Name,
				})
			}

		}
		loop_count++
	}

	fmt.Println("gave up: too many tool rounds")
}

func chat(req ChatRequest) (ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("error marshalling chat request: %v", err)
	}

	resp, err := http.Post("http://localhost:11434/api/chat", "application/json", bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("error sending chat request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("ollama %s: %s", resp.Status, body)
	}

	var output ChatResponse
	err = json.Unmarshal(body, &output)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return output, nil
}

func searchLogs(pattern string, limit int) string {
	// Clamp limit ourselves — never trust a caller (especially a model) with it.
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Sprintf("invalid pattern %q: %v", pattern, err)
	}

	f, err := os.Open("app.log")
	if err != nil {
		return fmt.Sprintf("failed to open app.log: %v", err)
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%d: %s", lineno, line))
			if len(matches) >= limit {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("error reading app.log: %v", err)
	}

	if len(matches) == 0 {
		return "no matches"
	}
	return strings.Join(matches, "\n")
}
