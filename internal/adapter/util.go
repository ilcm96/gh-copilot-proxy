package adapter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

var anthropicStopResponseMap = map[string]string{
	"stop":           "end_turn",
	"length":         "max_tokens",
	"tool_calls":     "tool_use",
	"content_filter": "stop_sequence",
}

// stringifyJSON serializes a value into a JSON string or falls back to a formatted string.
// stringifyJSON 는 값을 JSON 문자열로 직렬화하거나 포맷 가능한 문자열로 대체합니다.
func stringifyJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

// nestedMapValue finds the value at a given key path within nested maps or slices.
// nestedMapValue 는 중첩된 맵/슬라이스에서 지정된 키 경로의 값을 찾습니다.
func nestedMapValue(v any, keys ...any) any {
	current := v
	for _, key := range keys {
		switch k := key.(type) {
		case string:
			m, ok := current.(map[string]any)
			if !ok {
				return nil
			}
			current = m[k]
		case int:
			arr, ok := current.([]any)
			if !ok || k < 0 || k >= len(arr) {
				return nil
			}
			current = arr[k]
		default:
			return nil
		}
	}
	return current
}

// toSlice safely converts an interface value to a slice.
// toSlice 는 인터페이스 값을 슬라이스로 안전하게 변환합니다.
func toSlice(v any) []any {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

// toString converts various types into their string representation.
// toString 는 다양한 타입을 문자열 표현으로 변환합니다.
func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case fmt.Stringer:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case nil:
		return ""
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

// toInt converts a value that can be interpreted as an integer into an int.
// toInt 는 정수로 해석 가능한 값을 int 로 변환합니다.
func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			f, _ := t.Float64()
			return int(f)
		}
		return int(i)
	case string:
		if t == "" {
			return 0
		}
		if i, err := strconv.Atoi(t); err == nil {
			return i
		}
	}
	return 0
}
