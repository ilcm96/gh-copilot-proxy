package adapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ConvertResponseOpenAIToAnthropic transforms a Copilot response into an Anthropic message structure.
// ConvertResponseOpenAIToAnthropic 는 Copilot 응답을 Anthropic 메시지 구조로 변환합니다.
func ConvertResponseOpenAIToAnthropic(body map[string]any) (map[string]any, error) {
	choices := toSlice(body["choices"])
	if len(choices) == 0 {
		return nil, errors.New("no choices in response")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, errors.New("invalid choice format")
	}

	var content []map[string]any
	if message, ok := choice["message"].(map[string]any); ok {
		annotations := toSlice(message["annotations"])
		if len(annotations) > 0 {
			id := fmt.Sprintf("srvtoolu_%s", uuid.NewString())
			content = append(content, map[string]any{
				"type": "server_tool_use",
				"id":   id,
				"name": "web_search",
				"input": map[string]any{
					"query": "",
				},
			})
			var results []map[string]any
			for _, raw := range annotations {
				annot, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				info := map[string]any{
					"type":  "web_search_result",
					"url":   toString(nestedMapValue(annot, "url_citation", "url")),
					"title": toString(nestedMapValue(annot, "url_citation", "title")),
				}
				results = append(results, info)
			}
			content = append(content, map[string]any{
				"type":        "web_search_tool_result",
				"tool_use_id": id,
				"content":     results,
			})
		}
		if text := toString(message["content"]); text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		if toolCalls := toSlice(message["tool_calls"]); len(toolCalls) > 0 {
			for _, raw := range toolCalls {
				call, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				function, _ := call["function"].(map[string]any)
				argsVal := "{}"
				if function != nil {
					args := function["arguments"]
					switch v := args.(type) {
					case string:
						if v != "" {
							argsVal = v
						}
					case map[string]any, []any:
						b, err := json.Marshal(v)
						if err == nil {
							argsVal = string(b)
						}
					}
				}
				content = append(content, map[string]any{
					"type": "tool_use",
					"id":   toString(call["id"]),
					"name": toString(nestedMapValue(function, "name")),
					"input": map[string]any{
						"arguments": argsVal,
					},
				})
			}
		}
	}

	usage := map[string]int{}
	if rawUsage, ok := body["usage"].(map[string]any); ok {
		usage["input_tokens"] = toInt(rawUsage["prompt_tokens"])
		usage["output_tokens"] = toInt(rawUsage["completion_tokens"])
		usage["cache_read_input_tokens"] = toInt(rawUsage["cache_read_input_tokens"])
	}

	finishReason := toString(choice["finish_reason"])
	if mapped, ok := anthropicStopResponseMap[finishReason]; ok {
		finishReason = mapped
	} else {
		finishReason = "end_turn"
	}

	result := map[string]any{
		"id":            toString(body["id"]),
		"type":          "message",
		"role":          "assistant",
		"model":         toString(body["model"]),
		"content":       content,
		"stop_reason":   finishReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":            usage["input_tokens"],
			"output_tokens":           usage["output_tokens"],
			"cache_read_input_tokens": usage["cache_read_input_tokens"],
		},
	}
	return result, nil
}
