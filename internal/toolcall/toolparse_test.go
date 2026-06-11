package toolcall

import "testing"

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
