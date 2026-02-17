// Package parser provides functions for parsing the line-delimited JSON stream
// produced by Claude's --output-format stream-json output mode.
package parser

import (
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/jamesprial/research-dashboard/internal/model"
)

// ParseStreamLine parses a single line from Claude's stream-json output into
// zero or more typed ParsedEvent values. The counter is used to assign
// sequential Index values to each produced event; it is incremented atomically
// once per emitted event. Filtered events (ping, message_stop, empty lines)
// do not increment the counter. If counter is nil the function still operates
// correctly but Index will always be 0.
func ParseStreamLine(line string, counter *atomic.Int64) []model.ParsedEvent {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	// Attempt to decode the line as a JSON object.
	var data map[string]any
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		// Not valid JSON (or not a JSON object at the top level for the map type).
		// Emit a raw event whose Text is the trimmed line.
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeRaw,
			Text: trimmed,
		})}
	}

	// data may be nil when the JSON value was the literal "null".
	if data == nil {
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeRaw,
		})}
	}

	evtType, _ := stringVal(data, "type")

	switch evtType {
	case "system":
		subtype, _ := stringVal(data, "subtype")
		if subtype == "" {
			subtype = "init"
		}
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeSystem,
			Text: subtype,
			Raw:  data,
		})}

	case "assistant":
		return parseAssistantBlocks(data, counter)

	case "user":
		return parseUserBlocks(data, counter)

	case "result":
		return parseResult(data, counter)

	case "stream_event":
		return parseStreamEvent(data, counter)

	case "":
		// No type field present — treat as raw.
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeRaw,
			Raw:  data,
		})}

	default:
		// Recognised JSON object but unrecognized type value.
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeRaw,
			Raw:  data,
		})}
	}
}

// ---------------------------------------------------------------------------
// Type-specific parsers
// ---------------------------------------------------------------------------

// parseAssistantBlocks parses content blocks from an assistant message event.
func parseAssistantBlocks(data map[string]any, counter *atomic.Int64) []model.ParsedEvent {
	blocks := extractContentBlocks(data)
	if len(blocks) == 0 {
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeAssistant,
		})}
	}

	var events []model.ParsedEvent
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := stringVal(block, "type")
		switch blockType {
		case "text":
			text, _ := stringVal(block, "text")
			events = append(events, emit(counter, model.ParsedEvent{
				Type:    model.EventTypeAssistant,
				Subtype: model.SubtypeText,
				Text:    text,
			}))
		case "tool_use":
			name, _ := stringVal(block, "name")
			var toolInput map[string]any
			if raw, ok := block["input"]; ok {
				toolInput, _ = raw.(map[string]any)
			}
			events = append(events, emit(counter, model.ParsedEvent{
				Type:      model.EventTypeAssistant,
				Subtype:   model.SubtypeToolUse,
				ToolName:  name,
				ToolInput: toolInput,
			}))
		}
		// Unknown block types are silently skipped.
	}

	if len(events) == 0 {
		// Content array was non-empty but contained no recognized block types.
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeAssistant,
		})}
	}
	return events
}

// parseUserBlocks parses content blocks from a user message event.
func parseUserBlocks(data map[string]any, counter *atomic.Int64) []model.ParsedEvent {
	blocks := extractContentBlocks(data)
	if len(blocks) == 0 {
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeUser,
		})}
	}

	var events []model.ParsedEvent
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := stringVal(block, "type")
		if blockType != "tool_result" {
			continue
		}

		toolResult := extractToolResultContent(block)
		isError, _ := boolVal(block, "is_error")

		events = append(events, emit(counter, model.ParsedEvent{
			Type:       model.EventTypeUser,
			Subtype:    model.SubtypeToolResult,
			ToolResult: toolResult,
			IsError:    isError,
		}))
	}

	if len(events) == 0 {
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type: model.EventTypeUser,
		})}
	}
	return events
}

// parseResult parses a result-type event.
func parseResult(data map[string]any, counter *atomic.Int64) []model.ParsedEvent {
	text := extractResultText(data)
	isError, _ := boolVal(data, "is_error")

	return []model.ParsedEvent{emit(counter, model.ParsedEvent{
		Type:    model.EventTypeResult,
		Text:    text,
		IsError: isError,
		Raw:     data,
	})}
}

// parseStreamEvent parses a stream_event-type envelope.
func parseStreamEvent(data map[string]any, counter *atomic.Int64) []model.ParsedEvent {
	eventRaw, ok := data["event"]
	if !ok {
		return nil
	}
	event, ok := eventRaw.(map[string]any)
	if !ok {
		return nil
	}

	eventType, _ := stringVal(event, "type")

	switch eventType {
	case "ping", "message_stop":
		// Filtered — no events emitted, counter not incremented.
		return nil

	case "content_block_delta":
		delta, ok := event["delta"].(map[string]any)
		if !ok {
			return nil
		}
		deltaType, _ := stringVal(delta, "type")
		if deltaType != "text_delta" {
			return nil
		}
		text, _ := stringVal(delta, "text")
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type:    model.EventTypeAssistant,
			Subtype: model.SubtypeTextDelta,
			Text:    text,
		})}

	case "content_block_start":
		block, ok := event["content_block"].(map[string]any)
		if !ok {
			return nil
		}
		blockType, _ := stringVal(block, "type")
		if blockType != "tool_use" {
			return nil
		}
		name, _ := stringVal(block, "name")
		return []model.ParsedEvent{emit(counter, model.ParsedEvent{
			Type:     model.EventTypeAssistant,
			Subtype:  model.SubtypeToolStart,
			ToolName: name,
		})}

	default:
		// All other stream event types are filtered.
		return nil
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// emit assigns the next sequential index to an event using the counter.
// If counter is nil, the event index defaults to 0.
func emit(counter *atomic.Int64, evt model.ParsedEvent) model.ParsedEvent {
	if counter != nil {
		evt.Index = int(counter.Add(1) - 1)
	}
	return evt
}

// extractContentBlocks navigates data["message"]["content"] and returns the
// slice of raw content block values, or nil if the path does not exist.
func extractContentBlocks(data map[string]any) []any {
	messageRaw, ok := data["message"]
	if !ok {
		return nil
	}
	message, ok := messageRaw.(map[string]any)
	if !ok {
		return nil
	}
	contentRaw, ok := message["content"]
	if !ok {
		return nil
	}
	content, ok := contentRaw.([]any)
	if !ok {
		return nil
	}
	return content
}

// extractToolResultContent resolves the "content" field of a tool_result block.
// It handles two cases:
//   - string: returned directly.
//   - []any of {"type":"text","text":"..."} dicts: texts are joined with "\n".
func extractToolResultContent(block map[string]any) string {
	contentRaw, ok := block["content"]
	if !ok {
		return ""
	}

	// Case 1: plain string.
	if s, ok := contentRaw.(string); ok {
		return s
	}

	// Case 2: list of text dicts.
	list, ok := contentRaw.([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, item := range list {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemType, _ := stringVal(itemMap, "type"); itemType != "text" {
			continue
		}
		if text, ok := itemMap["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// extractResultText retrieves the result text from a result event's data map.
// The "result" field may be a string or a map with a "text" key.
func extractResultText(data map[string]any) string {
	resultRaw, ok := data["result"]
	if !ok {
		return ""
	}

	// Case 1: plain string.
	if s, ok := resultRaw.(string); ok {
		return s
	}

	// Case 2: map with "text" key.
	if m, ok := resultRaw.(map[string]any); ok {
		if text, ok := m["text"].(string); ok {
			return text
		}
	}

	return ""
}

// stringVal safely retrieves a string value from a map by key.
// Returns ("", false) if the key is absent or the value is not a string.
func stringVal(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// boolVal safely retrieves a bool value from a map by key.
// Returns (false, false) if the key is absent or the value is not a bool.
func boolVal(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
