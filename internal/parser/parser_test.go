package parser_test

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jamesprial/research-dashboard/internal/model"
	"github.com/jamesprial/research-dashboard/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newCounter creates a fresh atomic counter initialized to 0.
func newCounter() *atomic.Int64 {
	return &atomic.Int64{}
}

// assertEventCount is a test helper that checks the number of returned events.
func assertEventCount(t *testing.T, events []model.ParsedEvent, want int) {
	t.Helper()
	if len(events) != want {
		t.Fatalf("got %d events, want %d; events = %+v", len(events), want, events)
	}
}

// assertEventType checks a single event's Type field.
func assertEventType(t *testing.T, evt model.ParsedEvent, want model.EventType) {
	t.Helper()
	if evt.Type != want {
		t.Errorf("event.Type = %q, want %q", evt.Type, want)
	}
}

// assertEventSubtype checks a single event's Subtype field.
func assertEventSubtype(t *testing.T, evt model.ParsedEvent, want model.EventSubtype) {
	t.Helper()
	if evt.Subtype != want {
		t.Errorf("event.Subtype = %q, want %q", evt.Subtype, want)
	}
}

// assertEventText checks a single event's Text field.
func assertEventText(t *testing.T, evt model.ParsedEvent, want string) {
	t.Helper()
	if evt.Text != want {
		t.Errorf("event.Text = %q, want %q", evt.Text, want)
	}
}

// assertEventToolName checks a single event's ToolName field.
func assertEventToolName(t *testing.T, evt model.ParsedEvent, want string) {
	t.Helper()
	if evt.ToolName != want {
		t.Errorf("event.ToolName = %q, want %q", evt.ToolName, want)
	}
}

// assertEventToolResult checks a single event's ToolResult field.
func assertEventToolResult(t *testing.T, evt model.ParsedEvent, want string) {
	t.Helper()
	if evt.ToolResult != want {
		t.Errorf("event.ToolResult = %q, want %q", evt.ToolResult, want)
	}
}

// assertEventIsError checks a single event's IsError field.
func assertEventIsError(t *testing.T, evt model.ParsedEvent, want bool) {
	t.Helper()
	if evt.IsError != want {
		t.Errorf("event.IsError = %v, want %v", evt.IsError, want)
	}
}

// assertEventIndex checks a single event's Index field.
func assertEventIndex(t *testing.T, evt model.ParsedEvent, want int) {
	t.Helper()
	if evt.Index != want {
		t.Errorf("event.Index = %d, want %d", evt.Index, want)
	}
}

// ---------------------------------------------------------------------------
// Test: Empty and whitespace inputs
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_EmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty string", ""},
		{"whitespace only", "  \n  "},
		{"tabs and newlines", "\t\n\t"},
		{"single space", " "},
		{"carriage return", "\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)
			if len(events) != 0 {
				t.Errorf("ParseStreamLine(%q) returned %d events, want 0", tt.line, len(events))
			}
			// Counter should not have been incremented.
			if counter.Load() != 0 {
				t.Errorf("counter = %d, want 0 (no events produced)", counter.Load())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Invalid JSON
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"plain text", "not json at all"},
		{"partial JSON", `{"type":`},
		{"XML-like", `<message>hello</message>`},
		{"number only", "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)
			assertEventCount(t, events, 1)
			assertEventType(t, events[0], model.EventTypeRaw)
			assertEventText(t, events[0], tt.line)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: System events
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_SystemEvent(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantText string
	}{
		{
			name:     "system with subtype init",
			line:     `{"type":"system","subtype":"init"}`,
			wantText: "init",
		},
		{
			name:     "system without subtype defaults to init",
			line:     `{"type":"system"}`,
			wantText: "init",
		},
		{
			name:     "system with different subtype",
			line:     `{"type":"system","subtype":"greeting"}`,
			wantText: "greeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)
			assertEventCount(t, events, 1)
			assertEventType(t, events[0], model.EventTypeSystem)
			assertEventText(t, events[0], tt.wantText)
			// Raw field should be populated with the original parsed JSON.
			if events[0].Raw == nil {
				t.Error("event.Raw should be populated for system events")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Assistant events — text blocks
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_AssistantTextBlock(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeText)
	assertEventText(t, events[0], "hello world")
}

// ---------------------------------------------------------------------------
// Test: Assistant events — tool_use blocks
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_AssistantToolUse(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{"query":"test"}}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeToolUse)
	assertEventToolName(t, events[0], "WebSearch")

	// Verify tool input.
	if events[0].ToolInput == nil {
		t.Fatal("event.ToolInput should not be nil")
	}
	query, ok := events[0].ToolInput["query"]
	if !ok {
		t.Fatal("ToolInput missing key 'query'")
	}
	if query != "test" {
		t.Errorf("ToolInput[query] = %v, want %q", query, "test")
	}
}

// ---------------------------------------------------------------------------
// Test: Assistant events — multiple content blocks
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_AssistantMultipleBlocks(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"thinking..."},{"type":"tool_use","name":"Read","input":{"path":"/tmp"}}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 2)

	// First event: text block.
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeText)
	assertEventText(t, events[0], "thinking...")

	// Second event: tool_use block.
	assertEventType(t, events[1], model.EventTypeAssistant)
	assertEventSubtype(t, events[1], model.SubtypeToolUse)
	assertEventToolName(t, events[1], "Read")
	if events[1].ToolInput == nil {
		t.Fatal("second event ToolInput should not be nil")
	}
	if events[1].ToolInput["path"] != "/tmp" {
		t.Errorf("ToolInput[path] = %v, want %q", events[1].ToolInput["path"], "/tmp")
	}
}

// ---------------------------------------------------------------------------
// Test: Assistant events — empty or missing content
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_AssistantEmptyContent(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "empty content array",
			line: `{"type":"assistant","message":{"content":[]}}`,
		},
		{
			name: "no content key",
			line: `{"type":"assistant","message":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)
			// Should produce a fallback event.
			assertEventCount(t, events, 1)
			assertEventType(t, events[0], model.EventTypeAssistant)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: User events — tool_result
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_UserToolResult(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		wantToolResult string
		wantIsError    bool
	}{
		{
			name:           "tool_result string content, no error",
			line:           `{"type":"user","message":{"content":[{"type":"tool_result","content":"file contents here","is_error":false}]}}`,
			wantToolResult: "file contents here",
			wantIsError:    false,
		},
		{
			name:           "tool_result string content, is error",
			line:           `{"type":"user","message":{"content":[{"type":"tool_result","content":"not found","is_error":true}]}}`,
			wantToolResult: "not found",
			wantIsError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)

			assertEventCount(t, events, 1)
			assertEventType(t, events[0], model.EventTypeUser)
			assertEventSubtype(t, events[0], model.SubtypeToolResult)
			assertEventToolResult(t, events[0], tt.wantToolResult)
			assertEventIsError(t, events[0], tt.wantIsError)
		})
	}
}

func Test_ParseStreamLine_UserToolResultListContent(t *testing.T) {
	line := `{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeUser)
	assertEventSubtype(t, events[0], model.SubtypeToolResult)
	// List content should be joined with newlines.
	assertEventToolResult(t, events[0], "line1\nline2")
}

func Test_ParseStreamLine_UserEmptyContent(t *testing.T) {
	line := `{"type":"user","message":{"content":[]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	// Should produce a fallback event.
	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeUser)
}

// ---------------------------------------------------------------------------
// Test: Result events
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_ResultString(t *testing.T) {
	line := `{"type":"result","result":"Final answer","is_error":false}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeResult)
	assertEventText(t, events[0], "Final answer")
	assertEventIsError(t, events[0], false)
}

func Test_ParseStreamLine_ResultDict(t *testing.T) {
	line := `{"type":"result","result":{"text":"done"},"is_error":false}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeResult)
	assertEventText(t, events[0], "done")
	assertEventIsError(t, events[0], false)
}

func Test_ParseStreamLine_ResultError(t *testing.T) {
	line := `{"type":"result","result":"error msg","is_error":true}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeResult)
	assertEventIsError(t, events[0], true)
}

func Test_ParseStreamLine_ResultWithStats(t *testing.T) {
	line := `{"type":"result","result":"done","total_cost_usd":1.5,"duration_ms":5000,"duration_api_ms":4000,"num_turns":10,"session_id":"abc","usage":{"input":100}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeResult)
	assertEventText(t, events[0], "done")

	// Raw should contain all the original fields including stats.
	if events[0].Raw == nil {
		t.Fatal("event.Raw should not be nil for result events with stats")
	}

	// Verify stats are accessible via Raw.
	statsChecks := map[string]any{
		"total_cost_usd":  1.5,
		"duration_ms":     float64(5000),
		"duration_api_ms": float64(4000),
		"num_turns":       float64(10),
		"session_id":      "abc",
	}
	for key, wantVal := range statsChecks {
		gotVal, ok := events[0].Raw[key]
		if !ok {
			t.Errorf("Raw missing key %q", key)
			continue
		}
		// Compare via JSON round-trip for numeric tolerance.
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(wantVal)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("Raw[%q] = %v, want %v", key, gotVal, wantVal)
		}
	}

	// Check usage map exists.
	usage, ok := events[0].Raw["usage"]
	if !ok {
		t.Error("Raw missing key 'usage'")
	} else {
		usageMap, ok := usage.(map[string]any)
		if !ok {
			t.Errorf("usage is %T, want map[string]any", usage)
		} else {
			if _, ok := usageMap["input"]; !ok {
				t.Error("usage map missing key 'input'")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Stream events
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_StreamEventFiltered(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "ping event filtered",
			line: `{"type":"stream_event","event":{"type":"ping"}}`,
		},
		{
			name: "message_stop event filtered",
			line: `{"type":"stream_event","event":{"type":"message_stop"}}`,
		},
		{
			name: "non-tool content_block_start filtered",
			line: `{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"text"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)
			if len(events) != 0 {
				t.Errorf("ParseStreamLine(%q) returned %d events, want 0 (filtered)", tt.line, len(events))
			}
		})
	}
}

func Test_ParseStreamLine_StreamEventTextDelta(t *testing.T) {
	line := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk"}}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeTextDelta)
	assertEventText(t, events[0], "chunk")
}

func Test_ParseStreamLine_StreamEventToolStart(t *testing.T) {
	line := `{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"tool_use","name":"Bash"}}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeToolStart)
	assertEventToolName(t, events[0], "Bash")
}

// ---------------------------------------------------------------------------
// Test: Unrecognized type
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_UnrecognizedType(t *testing.T) {
	line := `{"type":"unknown_thing","data":123}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeRaw)
}

// ---------------------------------------------------------------------------
// Test: Counter behavior
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_CounterSequential(t *testing.T) {
	counter := newCounter()

	// Call 1: line that produces 1 event.
	line1 := `{"type":"system","subtype":"init"}`
	events1 := parser.ParseStreamLine(line1, counter)
	assertEventCount(t, events1, 1)
	assertEventIndex(t, events1[0], 0)

	// Call 2: line that produces 2 events.
	line2 := `{"type":"assistant","message":{"content":[{"type":"text","text":"a"},{"type":"tool_use","name":"Read","input":{"path":"/"}}]}}`
	events2 := parser.ParseStreamLine(line2, counter)
	assertEventCount(t, events2, 2)
	assertEventIndex(t, events2[0], 1)
	assertEventIndex(t, events2[1], 2)

	// Call 3: line that produces 1 event.
	line3 := `{"type":"result","result":"done","is_error":false}`
	events3 := parser.ParseStreamLine(line3, counter)
	assertEventCount(t, events3, 1)
	assertEventIndex(t, events3[0], 3)

	// Final counter value should be 4.
	if counter.Load() != 4 {
		t.Errorf("counter = %d, want 4", counter.Load())
	}
}

func Test_ParseStreamLine_CounterNotIncrementedForFiltered(t *testing.T) {
	counter := newCounter()

	// Filtered event should not increment the counter.
	events := parser.ParseStreamLine(`{"type":"stream_event","event":{"type":"ping"}}`, counter)
	if len(events) != 0 {
		t.Errorf("expected 0 events for ping, got %d", len(events))
	}
	if counter.Load() != 0 {
		t.Errorf("counter should remain 0 after filtered event, got %d", counter.Load())
	}

	// Now produce a real event — it should get index 0.
	events = parser.ParseStreamLine(`{"type":"system","subtype":"init"}`, counter)
	assertEventCount(t, events, 1)
	assertEventIndex(t, events[0], 0)
}

func Test_ParseStreamLine_CounterNotIncrementedForEmpty(t *testing.T) {
	counter := newCounter()

	// Empty line should not increment the counter.
	events := parser.ParseStreamLine("", counter)
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty line, got %d", len(events))
	}
	if counter.Load() != 0 {
		t.Errorf("counter should remain 0 after empty line, got %d", counter.Load())
	}
}

// ---------------------------------------------------------------------------
// Test: Edge cases
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_VeryLongLine(t *testing.T) {
	// Build a 10KB text payload.
	longText := strings.Repeat("a", 10*1024)
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + longText + `"}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeText)
	if len(events[0].Text) != 10*1024 {
		t.Errorf("expected text length %d, got %d", 10*1024, len(events[0].Text))
	}
}

func Test_ParseStreamLine_NullJSONValue(t *testing.T) {
	// The literal string "null" is valid JSON but not a JSON object.
	// It should be treated as a raw event.
	counter := newCounter()
	events := parser.ParseStreamLine("null", counter)
	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeRaw)
}

func Test_ParseStreamLine_UnicodeContent(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello \u4e16\u754c \ud83c\udf0d"}]}}`
	counter := newCounter()
	events := parser.ParseStreamLine(line, counter)

	assertEventCount(t, events, 1)
	assertEventType(t, events[0], model.EventTypeAssistant)
	assertEventSubtype(t, events[0], model.SubtypeText)
	// The text should contain decoded unicode characters.
	if !strings.Contains(events[0].Text, "\u4e16\u754c") {
		t.Errorf("event.Text does not contain expected unicode characters, got %q", events[0].Text)
	}
}

func Test_ParseStreamLine_NilCounter(t *testing.T) {
	// Passing a nil counter should not panic. This tests nil safety.
	// The function may either handle it gracefully or the test documents
	// that a non-nil counter is required.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ParseStreamLine panicked with nil counter: %v", r)
		}
	}()

	events := parser.ParseStreamLine(`{"type":"system","subtype":"init"}`, nil)
	// We only care that it does not panic; the result is implementation-defined.
	_ = events
}

// ---------------------------------------------------------------------------
// Test: Comprehensive scenario table (table-driven)
// ---------------------------------------------------------------------------

func Test_ParseStreamLine_ScenarioTable(t *testing.T) {
	type eventCheck struct {
		evtType    model.EventType
		subtype    model.EventSubtype
		text       string
		toolName   string
		toolResult string
		isError    bool
		checkRaw   bool // if true, verify Raw is non-nil
	}

	tests := []struct {
		name       string
		line       string
		wantCount  int
		wantEvents []eventCheck
	}{
		{
			name:      "empty line",
			line:      "",
			wantCount: 0,
		},
		{
			name:      "whitespace only",
			line:      "  \n  ",
			wantCount: 0,
		},
		{
			name:      "invalid JSON",
			line:      "not json at all",
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeRaw, text: "not json at all"},
			},
		},
		{
			name:      "system event with subtype",
			line:      `{"type":"system","subtype":"init"}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeSystem, text: "init", checkRaw: true},
			},
		},
		{
			name:      "system event no subtype",
			line:      `{"type":"system"}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeSystem, text: "init", checkRaw: true},
			},
		},
		{
			name:      "assistant text block",
			line:      `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeText, text: "hello world"},
			},
		},
		{
			name:      "assistant tool_use",
			line:      `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{"query":"test"}}]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeToolUse, toolName: "WebSearch"},
			},
		},
		{
			name:      "assistant multiple blocks",
			line:      `{"type":"assistant","message":{"content":[{"type":"text","text":"thinking..."},{"type":"tool_use","name":"Read","input":{"path":"/tmp"}}]}}`,
			wantCount: 2,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeText, text: "thinking..."},
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeToolUse, toolName: "Read"},
			},
		},
		{
			name:      "assistant empty content array",
			line:      `{"type":"assistant","message":{"content":[]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant},
			},
		},
		{
			name:      "assistant no content key",
			line:      `{"type":"assistant","message":{}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant},
			},
		},
		{
			name:      "user tool_result string",
			line:      `{"type":"user","message":{"content":[{"type":"tool_result","content":"file contents here","is_error":false}]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeUser, subtype: model.SubtypeToolResult, toolResult: "file contents here", isError: false},
			},
		},
		{
			name:      "user tool_result error",
			line:      `{"type":"user","message":{"content":[{"type":"tool_result","content":"not found","is_error":true}]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeUser, subtype: model.SubtypeToolResult, toolResult: "not found", isError: true},
			},
		},
		{
			name:      "user tool_result list content",
			line:      `{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeUser, subtype: model.SubtypeToolResult, toolResult: "line1\nline2"},
			},
		},
		{
			name:      "user empty content",
			line:      `{"type":"user","message":{"content":[]}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeUser},
			},
		},
		{
			name:      "result string",
			line:      `{"type":"result","result":"Final answer","is_error":false}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeResult, text: "Final answer", isError: false},
			},
		},
		{
			name:      "result dict",
			line:      `{"type":"result","result":{"text":"done"},"is_error":false}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeResult, text: "done", isError: false},
			},
		},
		{
			name:      "result error",
			line:      `{"type":"result","result":"error msg","is_error":true}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeResult, text: "error msg", isError: true},
			},
		},
		{
			name:      "result with stats",
			line:      `{"type":"result","result":"done","total_cost_usd":1.5,"duration_ms":5000,"duration_api_ms":4000,"num_turns":10,"session_id":"abc","usage":{"input":100}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeResult, text: "done", checkRaw: true},
			},
		},
		{
			name:      "stream_event ping filtered",
			line:      `{"type":"stream_event","event":{"type":"ping"}}`,
			wantCount: 0,
		},
		{
			name:      "stream_event message_stop filtered",
			line:      `{"type":"stream_event","event":{"type":"message_stop"}}`,
			wantCount: 0,
		},
		{
			name:      "stream_event text_delta",
			line:      `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk"}}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeTextDelta, text: "chunk"},
			},
		},
		{
			name:      "stream_event tool_start",
			line:      `{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"tool_use","name":"Bash"}}}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeAssistant, subtype: model.SubtypeToolStart, toolName: "Bash"},
			},
		},
		{
			name:      "stream_event non-tool block start filtered",
			line:      `{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"text"}}}`,
			wantCount: 0,
		},
		{
			name:      "unrecognized type",
			line:      `{"type":"unknown_thing","data":123}`,
			wantCount: 1,
			wantEvents: []eventCheck{
				{evtType: model.EventTypeRaw},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := newCounter()
			events := parser.ParseStreamLine(tt.line, counter)

			assertEventCount(t, events, tt.wantCount)

			for i, wantEvt := range tt.wantEvents {
				if i >= len(events) {
					break
				}
				evt := events[i]
				assertEventType(t, evt, wantEvt.evtType)
				if wantEvt.subtype != model.SubtypeEmpty {
					assertEventSubtype(t, evt, wantEvt.subtype)
				}
				if wantEvt.text != "" {
					assertEventText(t, evt, wantEvt.text)
				}
				if wantEvt.toolName != "" {
					assertEventToolName(t, evt, wantEvt.toolName)
				}
				if wantEvt.toolResult != "" {
					assertEventToolResult(t, evt, wantEvt.toolResult)
				}
				if wantEvt.isError {
					assertEventIsError(t, evt, true)
				}
				if wantEvt.checkRaw && evt.Raw == nil {
					t.Errorf("event[%d].Raw should be non-nil", i)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func Benchmark_ParseStreamLine_SystemEvent(b *testing.B) {
	line := `{"type":"system","subtype":"init"}`
	counter := newCounter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseStreamLine(line, counter)
	}
}

func Benchmark_ParseStreamLine_AssistantText(b *testing.B) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}`
	counter := newCounter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseStreamLine(line, counter)
	}
}

func Benchmark_ParseStreamLine_StreamEventTextDelta(b *testing.B) {
	line := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"chunk"}}}`
	counter := newCounter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseStreamLine(line, counter)
	}
}

func Benchmark_ParseStreamLine_LongLine(b *testing.B) {
	longText := strings.Repeat("x", 10*1024)
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + longText + `"}]}}`
	counter := newCounter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseStreamLine(line, counter)
	}
}
