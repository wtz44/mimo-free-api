package toolcall

import (
	"strings"
	"testing"
)

func TestParseStandardFuncCalls(t *testing.T) {
	// Simulate what MiMo outputs
	LT := "\x3c" // <
	GT := "\x3e" // >
	F := "function"
	P := "parameter"
	SL := "\x2f" // /

	text := LT + F + "=WebFetch" + GT + "\n" +
		LT + P + " name=\"url\"" + GT + "https://www.bing.com/search?q=test" + LT + SL + P + GT + "\n" +
		LT + P + " name=\"format\"" + GT + "text" + LT + SL + P + GT + "\n" +
		LT + SL + F + GT

	t.Logf("Input text: %q", text)

	if !HasToolCallSyntax(text) {
		t.Fatal("HasToolCallSyntax returned false for standard format")
	}

	calls := ParseToolCallsFromText(text)
	if len(calls) == 0 {
		t.Fatal("ParseToolCallsFromText returned no calls")
	}

	call := calls[0]
	if call.Name != "WebFetch" {
		t.Errorf("expected name WebFetch, got %s", call.Name)
	}
	if call.Input["url"] != "https://www.bing.com/search?q=test" {
		t.Errorf("expected url param, got %v", call.Input["url"])
	}
	if call.Input["format"] != "text" {
		t.Errorf("expected format param, got %v", call.Input["format"])
	}

	t.Logf("Parsed successfully: name=%s input=%v", call.Name, call.Input)
}

func TestParsePlainFormat(t *testing.T) {
	LT := "\x3c"
	GT := "\x3e"
	SL := "\x2f"

	text := LT + "tool_calls" + GT +
		LT + "invoke name=\"Read\"" + GT +
		LT + "parameter name=\"file_path\"" + GT + "README.md" + LT + SL + "parameter" + GT +
		LT + SL + "invoke" + GT +
		LT + SL + "tool_calls" + GT

	calls := ParseToolCallsFromText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("expected Read, got %s", calls[0].Name)
	}
	t.Logf("Plain format OK: %v", calls[0])
}

func TestParseAltParamFormat(t *testing.T) {
	LT := "\x3c"
	GT := "\x3e"
	F := "function"
	P := "parameter"
	SL := "\x2f"
	EQ := "\x3d"

	text := LT + F + "=webfetch" + GT + "\n" +
		LT + P + EQ + "url" + GT + "https://example.com" + LT + SL + P + GT + "\n" +
		LT + P + EQ + "format" + GT + "markdown" + LT + SL + P + GT + "\n" +
		LT + SL + F + GT

	t.Logf("Input: %q", text)

	if !HasToolCallSyntax(text) {
		t.Fatal("HasToolCallSyntax returned false for eq format")
	}

	calls := ParseToolCallsFromText(text)
	if len(calls) == 0 {
		t.Fatal("ParseToolCallsFromText returned no calls")
	}

	call := calls[0]
	if call.Name != "webfetch" {
		t.Errorf("expected webfetch, got %s", call.Name)
	}
	if call.Input["url"] != "https://example.com" {
		t.Errorf("expected url param, got %v", call.Input["url"])
	}
	if call.Input["format"] != "markdown" {
		t.Errorf("expected format param, got %v", call.Input["format"])
	}

	t.Logf("Alt format OK: name=%s input=%v", call.Name, call.Input)
}

func TestParseFuncCallsBlock(t *testing.T) {
	LT := "\x3c"
	GT := "\x3e"
	SL := "\x2f"

	// Simulate model output: <function_calls><function=name><parameter ...></function></function_calls>
	text := LT + "function_calls" + GT + "\n" +
		LT + "function=read_file" + GT + "\n" +
		LT + "parameter name=\"path\"" + GT + "/home/user/test.txt" + LT + SL + "parameter" + GT + "\n" +
		LT + SL + "function" + GT + "\n" +
		LT + SL + "function_calls" + GT

	t.Logf("Input: %q", text)

	if !HasToolCallSyntax(text) {
		t.Fatal("HasToolCallSyntax returned false for function_calls block")
	}

	calls := ParseToolCallsFromText(text)
	if len(calls) == 0 {
		t.Fatal("ParseToolCallsFromText returned no calls")
	}

	call := calls[0]
	if call.Name != "read_file" {
		t.Errorf("expected read_file, got %s", call.Name)
	}
	if call.Input["path"] != "/home/user/test.txt" {
		t.Errorf("expected path param, got %v", call.Input["path"])
	}
	t.Logf("function_calls block OK: name=%s input=%v", call.Name, call.Input)
}

func TestParsePercentToolCalls(t *testing.T) {
	LT := "\x3c"
	GT := "\x3e"
	SL := "\x2f"

	// Test pure percent format (no XML present)
	purePercent := "% WebFetch https://example.com\n% WebFetch https://example.org"

	calls := ParseToolCallsFromText(purePercent)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls from percent format, got %d", len(calls))
	}
	if calls[0].Input["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", calls[0].Input["url"])
	}
	if calls[1].Input["url"] != "https://example.org" {
		t.Errorf("expected url=https://example.org, got %v", calls[1].Input["url"])
	}

	// Test mixed: percent lines + DSML (DSML should take priority)
	mixed := "% WebFetch https://example.com\n% WebFetch https://example.org\n" +
		LT + "\uff5cDSML\uff5ctool_calls" + GT + "\n" +
		"  " + LT + "\uff5cDSML\uff5cinvoke name=\"webfetch\"" + GT + "\n" +
		"    " + LT + "\uff5cDSML\uff5cparameter name=\"url\"" + GT + "<![CDATA[https://real.com]]>" + LT + SL + "\uff5cDSML\uff5cparameter" + GT + "\n" +
		"  " + LT + SL + "\uff5cDSML\uff5cinvoke" + GT + "\n" +
		LT + SL + "\uff5cDSML\uff5ctool_calls" + GT

	calls = ParseToolCallsFromText(mixed)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call (DSML takes priority), got %d", len(calls))
	}
	if calls[0].Name != "webfetch" {
		t.Errorf("expected webfetch, got %s", calls[0].Name)
	}
	if calls[0].Input["url"] != "https://real.com" {
		t.Errorf("expected url=https://real.com, got %v", calls[0].Input["url"])
	}

	// Test StripToolCallSyntax removes percent lines
	stripped := StripToolCallSyntax(purePercent)
	if strings.Contains(stripped, "%") {
		t.Errorf("StripToolCallSyntax should remove percent lines, got: %s", stripped)
	}

	// Test HasToolCallSyntax detects percent format
	if !HasToolCallSyntax(purePercent) {
		t.Error("HasToolCallSyntax should detect percent format")
	}

	t.Logf("percent format OK: %d calls parsed, stripped=%q", len(calls), stripped)
}

func TestParseNestedDSMLParams(t *testing.T) {
	// Simulate the exact output mimo-v2.5-pro generates for the "question" tool
	// with nested parameters (questions > item > header/options/item)
	LT := "\x3c"
	GT := "\x3e"
	SL := "\x2f"
	FW := "\uff5c" // fullwidth ｜

	text := LT + FW + "DSML" + FW + "tool_calls" + GT + "\n" +
		"  " + LT + FW + "DSML" + FW + "invoke name=\"question\"" + GT + "\n" +
		"    " + LT + FW + "DSML" + FW + "parameter name=\"questions\"" + GT + "\n" +
		"      " + LT + FW + "DSML" + FW + "parameter name=\"item\"" + GT + "\n" +
		"        " + LT + FW + "DSML" + FW + "parameter name=\"header\"" + GT + "迷宫风格" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"        " + LT + FW + "DSML" + FW + "parameter name=\"options\"" + GT + "\n" +
		"          " + LT + FW + "DSML" + FW + "parameter name=\"item\"" + GT + "\n" +
		"            " + LT + FW + "DSML" + FW + "parameter name=\"description\"" + GT + "精致木质感" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"            " + LT + FW + "DSML" + FW + "parameter name=\"label\"" + GT + "木质3D" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"          " + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"          " + LT + FW + "DSML" + FW + "parameter name=\"item\"" + GT + "\n" +
		"            " + LT + FW + "DSML" + FW + "parameter name=\"description\"" + GT + "霓虹灯风格" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"            " + LT + FW + "DSML" + FW + "parameter name=\"label\"" + GT + "霓虹科幻" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"          " + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"        " + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"        " + LT + FW + "DSML" + FW + "parameter name=\"question\"" + GT + "你希望迷宫是什么风格？" + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"      " + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"    " + LT + SL + FW + "DSML" + FW + "parameter" + GT + "\n" +
		"  " + LT + SL + FW + "DSML" + FW + "invoke" + GT + "\n" +
		LT + SL + FW + "DSML" + FW + "tool_calls" + GT

	t.Logf("Input text length: %d", len(text))

	if !HasToolCallSyntax(text) {
		t.Fatal("HasToolCallSyntax returned false for nested DSML format")
	}

	calls := ParseToolCallsFromText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.Name != "question" {
		t.Errorf("expected name=question, got %s", call.Name)
	}

	t.Logf("Parsed input: %+v", call.Input)

	// Structure: questions -> item -> {header, options, question}
	questions, ok := call.Input["questions"].(map[string]any)
	if !ok {
		t.Fatalf("expected questions to be map, got %T: %v", call.Input["questions"], call.Input["questions"])
	}

	item, ok := questions["item"].(map[string]any)
	if !ok {
		t.Fatalf("expected questions.item to be map, got %T: %v", questions["item"], questions["item"])
	}

	// Verify header
	if item["header"] != "迷宫风格" {
		t.Errorf("expected header=迷宫风格, got %v", item["header"])
	}

	// Verify question text
	if item["question"] != "你希望迷宫是什么风格？" {
		t.Errorf("expected question text, got %v", item["question"])
	}

	// Verify options is a map with item array
	options, ok := item["options"].(map[string]any)
	if !ok {
		t.Fatalf("expected options to be map, got %T: %v", item["options"], item["options"])
	}

	items, ok := options["item"].([]any)
	if !ok {
		t.Fatalf("expected options.item to be array, got %T: %v", options["item"], options["item"])
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items in options, got %d", len(items))
	}

	item0, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected item[0] to be map, got %T", items[0])
	}
	if item0["description"] != "精致木质感" {
		t.Errorf("expected item[0].description=精致木质感, got %v", item0["description"])
	}
	if item0["label"] != "木质3D" {
		t.Errorf("expected item[0].label=木质3D, got %v", item0["label"])
	}

	t.Logf("Nested DSML parsing OK: questions.item has %d keys, options has %d items", len(item), len(items))
}

func TestParseNestedToolCalls(t *testing.T) {
	// Model outputs nested format: <tool_call> containing <function=X>
	LT := "\x3c"
	GT := "\x3e"
	F := "function"
	P := "parameter"
	SL := "\x2f"

	input := LT + "tool_call" + GT + "\n" +
		LT + F + "=webfetch" + GT + "\n" +
		LT + P + " name=\"url\"" + GT + "https://www.bing.com/search?q=claude+fable+5" + LT + SL + P + GT + "\n" +
		LT + P + " name=\"format\"" + GT + "markdown" + LT + SL + P + GT + "\n" +
		LT + SL + F + GT + "\n" +
		LT + SL + "tool_call" + GT

	t.Logf("Input: %q", input)

	calls := ParseToolCallsFromText(input)
	if len(calls) == 0 {
		t.Fatal("ParseToolCallsFromText returned no calls for nested format")
	}

	call := calls[0]
	if call.Name != "webfetch" {
		t.Errorf("expected webfetch, got %s", call.Name)
	}
	if call.Input["url"] != "https://www.bing.com/search?q=claude+fable+5" {
		t.Errorf("expected url, got %v", call.Input["url"])
	}
	if call.Input["format"] != "markdown" {
		t.Errorf("expected format=markdown, got %v", call.Input["format"])
	}

	t.Logf("nested format OK: name=%s input=%v", call.Name, call.Input)
}
