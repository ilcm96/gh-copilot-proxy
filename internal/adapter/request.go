package adapter

import (
	"encoding/json"
	"strings"
)

// ConvertRequestAnthropicToOpenAI converts an Anthropic-style message into the OpenAI request shape.
// ConvertRequestAnthropicToOpenAI 는 Anthropic 스타일 메시지를 OpenAI 요청 형식으로 변환합니다.
func ConvertRequestAnthropicToOpenAI(body map[string]any) map[string]any {
	var messages []map[string]any

	if system, ok := body["system"]; ok {
		switch v := system.(type) {
		case string:
			if v != "" {
				messages = append(messages, map[string]any{"role": "system", "content": v})
			}
		case []any:
			var textParts []map[string]any
			for _, item := range v {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				text := toString(itemMap["text"])
				if text != "" {
					textParts = append(textParts, map[string]any{"type": "text", "text": text})
				}
			}
			if len(textParts) > 0 {
				messages = append(messages, map[string]any{"role": "system", "content": textParts})
			}
		}
	}

	for _, raw := range toSlice(body["messages"]) {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := toString(msg["role"])
		if role != "user" && role != "assistant" {
			continue
		}
		content := msg["content"]
		switch v := content.(type) {
		case string:
			messages = append(messages, map[string]any{"role": role, "content": v})
		case []any:
			if role == "user" {
				toolParts := make([]any, 0)
				for _, part := range v {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}
					if toString(partMap["type"]) == "tool_result" && toString(partMap["tool_use_id"]) != "" {
						toolParts = append(toolParts, partMap)
					}
				}
				for _, tool := range toolParts {
					toolMap, ok := tool.(map[string]any)
					if !ok {
						continue
					}
					contentValue := toolMap["content"]
					var payload string
					switch c := contentValue.(type) {
					case string:
						payload = c
					case []any, map[string]any:
						b, err := json.Marshal(c)
						if err == nil {
							payload = string(b)
						}
					}
					if payload == "" {
						payload = "{}"
					}
					messages = append(messages, map[string]any{
						"role":         "tool",
						"content":      payload,
						"tool_call_id": toString(toolMap["tool_use_id"]),
					})
				}

				var openaiContent []any
				for _, part := range v {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}
					partType := toString(partMap["type"])
					switch partType {
					case "text":
						text := toString(partMap["text"])
						if text != "" {
							openaiContent = append(openaiContent, map[string]any{"type": "text", "text": text})
						}
					case "image":
						source, ok := partMap["source"].(map[string]any)
						if !ok {
							continue
						}
						url := toString(source["url"])
						if url == "" {
							if data := toString(source["data"]); data != "" {
								url = data
							}
						}
						if url == "" {
							continue
						}
						contentMap := map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": url},
						}
						if mediaType := toString(source["media_type"]); mediaType != "" {
							contentMap["media_type"] = mediaType
						}
						openaiContent = append(openaiContent, contentMap)
					}
				}
				if len(openaiContent) > 0 {
					messages = append(messages, map[string]any{"role": "user", "content": openaiContent})
				}
			} else if role == "assistant" {
				assistantMessage := map[string]any{"role": "assistant"}
				var textParts []string
				if len(v) > 0 {
					for _, part := range v {
						partMap, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if toString(partMap["type"]) == "text" {
							text := toString(partMap["text"])
							if text != "" {
								textParts = append(textParts, text)
							}
						}
					}
				}
				if len(textParts) > 0 {
					assistantMessage["content"] = strings.Join(textParts, "\n")
				}

				var toolCalls []map[string]any
				for _, part := range v {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}
					if toString(partMap["type"]) == "tool_use" {
						fnName := toString(partMap["name"])
						if fnName == "" {
							continue
						}
						args := partMap["input"]
						if args == nil {
							args = map[string]any{}
						}
						argsJSON, err := json.Marshal(args)
						if err != nil {
							argsJSON = []byte("{}")
						}
						toolCalls = append(toolCalls, map[string]any{
							"id":   toString(partMap["id"]),
							"type": "function",
							"function": map[string]any{
								"name":      fnName,
								"arguments": string(argsJSON),
							},
						})
					}
				}
				if len(toolCalls) > 0 {
					assistantMessage["tool_calls"] = toolCalls
				}
				if _, ok := assistantMessage["content"]; ok || len(toolCalls) > 0 {
					messages = append(messages, assistantMessage)
				}
			}
		}
	}

	var tools []map[string]any
	for _, raw := range toSlice(body["tools"]) {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := toString(tool["name"])
		if name == "" {
			continue
		}
		function := map[string]any{
			"name":        name,
			"description": toString(tool["description"]),
			"parameters":  tool["input_schema"],
		}
		tools = append(tools, map[string]any{
			"type":     "function",
			"function": function,
		})
	}

	result := map[string]any{
		"messages":    messages,
		"model":       body["model"],
		"max_tokens":  body["max_tokens"],
		"temperature": body["temperature"],
		"stream":      body["stream"],
	}
	if len(tools) > 0 {
		result["tools"] = tools
	}
	if toolChoice, ok := body["tool_choice"]; ok {
		result["tool_choice"] = toolChoice
	}
	return result
}
