package adapter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ilcm96/gh-copilot-proxy/internal/httpx"
)

// toolCallState preserves tool call tracking state during streaming.
// toolCallState 는 스트리밍 중 추적되는 도구 호출 상태를 보존합니다.
type toolCallState struct {
	ID                string
	Name              string
	Arguments         strings.Builder
	ContentBlockIndex int
}

// sseConverter provides a state machine that converts OpenAI SSE streams into Anthropic events.
// sseConverter 는 OpenAI SSE 스트림을 Anthropic 이벤트로 변환하는 상태 기계를 제공합니다.
type sseConverter struct {
	previousChunk string
	messageStart  bool
	stopReason    map[string]any

	currentContentBlockIndex int
	thinkingStart            bool
	contentIndex             int
	contentChunks            int
	textContentStart         bool
	toolCallChunks           int

	toolCallsByIndex              map[int]*toolCallState
	toolCallIndexToContentBlockID map[int]int
}

// newSSEConverter returns an instance with initialized SSE conversion state.
// newSSEConverter 는 SSE 변환 상태를 초기화한 인스턴스를 반환합니다.
func newSSEConverter() *sseConverter {
	return &sseConverter{
		currentContentBlockIndex:      -1,
		toolCallsByIndex:              make(map[int]*toolCallState),
		toolCallIndexToContentBlockID: make(map[int]int),
	}
}

// TransformOpenAIResponseToAnthropic converts a Copilot response into an Anthropic-compatible format.
// TransformOpenAIResponseToAnthropic 는 Copilot 응답을 Anthropic 호환 형식으로 변환합니다.
func TransformOpenAIResponseToAnthropic(w http.ResponseWriter, resp *http.Response) error {
	httpx.CopyHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(resp.StatusCode)
		converter := newSSEConverter()
		return converter.pipe(w, resp.Body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	converted, err := ConvertResponseOpenAIToAnthropic(payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(converted)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(data)
	return err
}

// pipe reads an OpenAI SSE stream and forwards it as Anthropic events.
// pipe 는 OpenAI SSE 스트림을 읽어 Anthropic 이벤트로 전송합니다.
func (c *sseConverter) pipe(w http.ResponseWriter, reader io.Reader) error {
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)
	scanner.Split(splitDoubleNewline)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		events, err := c.processLine(line)
		if err != nil {
			return err
		}
		for _, event := range events {
			if len(event) == 0 {
				continue
			}
			if _, err := w.Write(event); err != nil {
				return err
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// processLine converts a single SSE data chunk into Anthropic events.
// processLine 는 단일 SSE 데이터 청크를 Anthropic 이벤트들로 변환합니다.
func (c *sseConverter) processLine(line string) ([][]byte, error) {
	var events [][]byte
	if line == "" {
		return events, nil
	}
	if strings.HasPrefix(line, "data: ") {
		line = line[6:]
	} else if c.previousChunk != "" {
		line = c.previousChunk + line
		c.previousChunk = ""
		log.Printf("continuing previous chunk: %s", line)
	}
	if line == "[DONE]" {
		if c.stopReason == nil {
			c.stopReason = map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   "end_turn",
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			}
		}
		data, err := marshalEventPayload(c.stopReason)
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("message_delta", data))
		stopPayload, err := marshalEventPayload(map[string]any{"type": "message_stop"})
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("message_stop", stopPayload))
		c.stopReason = nil
		return events, nil
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(line), &body); err != nil {
		c.previousChunk += line
		return events, nil
	}

	if errPayload, ok := body["error"].(map[string]any); ok {
		payload := map[string]any{
			"type": "error",
			"message": map[string]any{
				"type":    "api_error",
				"message": stringifyJSON(errPayload),
			},
		}
		data, err := marshalEventPayload(payload)
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("error", data))
		return events, nil
	}

	if !c.messageStart {
		c.messageStart = true
		messageID := fmt.Sprintf("%d", time.Now().UnixMilli())
		payload := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"message_id":    messageID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         toString(body["model"]),
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		}
		data, err := marshalEventPayload(payload)
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("message_start", data))
	}

	if usage, ok := body["usage"].(map[string]any); ok {
		if c.stopReason == nil {
			c.stopReason = map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   "end_turn",
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"input_tokens":            toInt(usage["prompt_tokens"]),
					"output_tokens":           toInt(usage["completion_tokens"]),
					"cache_read_input_tokens": toInt(usage["cache_read_input_tokens"]),
				},
			}
		} else {
			currentUsage, _ := c.stopReason["usage"].(map[string]any)
			if currentUsage == nil {
				currentUsage = map[string]any{}
				c.stopReason["usage"] = currentUsage
			}
			currentUsage["input_tokens"] = toInt(currentUsage["input_tokens"]) + toInt(usage["prompt_tokens"])
			currentUsage["output_tokens"] = toInt(currentUsage["output_tokens"]) + toInt(usage["completion_tokens"])
			currentUsage["cache_read_input_tokens"] = toInt(currentUsage["cache_read_input_tokens"]) + toInt(usage["cache_read_input_tokens"])
		}
	}

	choices := toSlice(body["choices"])
	if len(choices) == 0 {
		return events, nil
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return events, nil
	}

	delta, _ := choice["delta"].(map[string]any)

	if thinking, ok := delta["thinking"].(map[string]any); ok {
		if c.currentContentBlockIndex >= 0 {
			stopPayload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_stop",
				"index": c.currentContentBlockIndex,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_stop", stopPayload))
			c.currentContentBlockIndex = -1
		}
		if !c.thinkingStart {
			c.thinkingStart = true
			c.currentContentBlockIndex = c.contentIndex
			startPayload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_delta",
				"index": c.contentIndex,
				"content_block": map[string]any{
					"type":     "thinking",
					"thinking": "",
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_start", startPayload))
		}
		if signature := toString(thinking["signature"]); signature != "" {
			payload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_delta",
				"index": c.currentContentBlockIndex,
				"delta": map[string]any{
					"type":      "thinking_delta",
					"signature": signature,
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_delta", payload))
		}
		if reasoning := toString(thinking["reasoning"]); reasoning != "" {
			payload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_delta",
				"index": c.currentContentBlockIndex,
				"delta": map[string]any{
					"type": "thinking_delta",
					"text": reasoning,
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_delta", payload))
		}
		return events, nil
	}

	if c.thinkingStart {
		stopPayload, err := marshalEventPayload(map[string]any{
			"type":  "content_block_stop",
			"index": c.currentContentBlockIndex,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("content_block_stop", stopPayload))
		c.currentContentBlockIndex = -1
		c.thinkingStart = false
		c.contentIndex++
	}

	if deltaText := toString(nestedMapValue(delta, "content", 0, "text")); deltaText != "" {
		if !c.textContentStart {
			payload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_start",
				"index": c.contentIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_start", payload))
			c.textContentStart = true
			c.currentContentBlockIndex = c.contentIndex
		}
		payload, err := marshalEventPayload(map[string]any{
			"type":  "content_block_delta",
			"index": c.currentContentBlockIndex,
			"delta": map[string]any{
				"type": "text_delta",
				"text": deltaText,
			},
		})
		if err != nil {
			return nil, err
		}
		events = append(events, buildEvent("content_block_delta", payload))
		c.contentChunks++
	}

	if len(toSlice(nestedMapValue(delta, "content"))) > 0 && toString(nestedMapValue(delta, "content", 0, "id")) != "" {
		if !c.textContentStart {
			payload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_start",
				"index": c.contentIndex,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    "",
					"name":  "",
					"input": map[string]any{},
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_start", payload))
			c.currentContentBlockIndex = c.contentIndex
		}
	}

	if toolCalls := toSlice(nestedMapValue(delta, "tool_calls")); len(toolCalls) > 0 {
		for index, raw := range toolCalls {
			toolCall, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := c.toolCallsByIndex[index]; !ok {
				newIndex := c.contentIndex + c.toolCallChunks + 1
				c.toolCallChunks++
				c.toolCallIndexToContentBlockID[index] = newIndex
				toolID := toString(toolCall["id"])
				toolName := toString(nestedMapValue(toolCall, "function", "name"))
				if toolName == "" {
					toolName = fmt.Sprintf("tool_%d", index)
				}
				state := &toolCallState{ID: toolID, Name: toolName, ContentBlockIndex: newIndex}
				c.toolCallsByIndex[index] = state
				startPayload, err := marshalEventPayload(map[string]any{
					"type":  "content_block_start",
					"index": newIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    state.ID,
						"name":  state.Name,
						"input": map[string]any{},
					},
				})
				if err != nil {
					return nil, err
				}
				events = append(events, buildEvent("content_block_start", startPayload))
				c.currentContentBlockIndex = newIndex
			}

			if fn, ok := toolCall["function"].(map[string]any); ok {
				if id := toString(toolCall["id"]); id != "" {
					if state := c.toolCallsByIndex[index]; state != nil {
						state.ID = id
						if name := toString(nestedMapValue(fn, "name")); name != "" {
							state.Name = name
						}
					}
				}
				if args, ok := fn["arguments"].(string); ok {
					blockIndex, known := c.toolCallIndexToContentBlockID[index]
					if !known {
						continue
					}
					payload, err := marshalEventPayload(map[string]any{
						"type":  "content_block_delta",
						"index": blockIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": sanitizeArgument(args),
						},
					})
					if err != nil {
						return nil, err
					}
					events = append(events, buildEvent("content_block_delta", payload))
				}
			}
		}
	}

	if finishReason := toString(choice["finish_reason"]); finishReason != "" {
		if c.currentContentBlockIndex >= 0 {
			stopPayload, err := marshalEventPayload(map[string]any{
				"type":  "content_block_stop",
				"index": c.currentContentBlockIndex,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, buildEvent("content_block_stop", stopPayload))
			c.currentContentBlockIndex = -1
		}
		mapped := anthropicStopResponseMap[finishReason]
		if mapped == "" {
			mapped = "end_turn"
		}
		if usage, ok := body["usage"].(map[string]any); ok {
			c.stopReason = map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   mapped,
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"input_tokens":            toInt(usage["prompt_tokens"]),
					"output_tokens":           toInt(usage["completion_tokens"]),
					"cache_read_input_tokens": toInt(usage["cache_read_input_tokens"]),
				},
			}
		}
	}

	return events, nil
}

// splitDoubleNewline splits an SSE stream using double newlines as separators.
// splitDoubleNewline 는 SSE 스트림을 두 줄 공백 단위로 분리하는 스캐너 함수입니다.
func splitDoubleNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
		return i + 2, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// marshalEventPayload serializes an event payload into JSON bytes.
// marshalEventPayload 는 이벤트 페이로드를 JSON 바이트로 직렬화합니다.
func marshalEventPayload(payload map[string]any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// buildEvent assembles SSE event headers together with their data payload.
// buildEvent 는 SSE 이벤트 헤더와 데이터를 조립합니다.
func buildEvent(event string, data []byte) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("event: ")
	buffer.WriteString(event)
	buffer.WriteString("\n")
	buffer.WriteString("data: ")
	buffer.Write(data)
	buffer.WriteString("\n\n")
	return buffer.Bytes()
}

// sanitizeArgument attempts to safely fix malformed JSON fragments.
// sanitizeArgument 는 잘못된 JSON 조각을 최대한 안전하게 수정합니다.
func sanitizeArgument(arg string) string {
	if json.Valid([]byte(arg)) {
		return arg
	}
	re := regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`)
	fixed := re.ReplaceAllString(arg, "")
	fixed = strings.ReplaceAll(fixed, "\\", "\\\\")
	fixed = strings.ReplaceAll(fixed, "\"", "\\\"")
	return fixed
}
