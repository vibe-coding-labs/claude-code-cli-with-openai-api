package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

// convertClaudeRequest drives the shipped Claude parse and OpenAI build entry points.
func convertClaudeRequest(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	internal, err := NewClaudeConverter().ParseRequest([]byte(body))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	out, err := NewOpenAIConverter(nil).BuildRequest(internal)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal openai request: %v", err)
	}
	return got
}

func messagesOf(t *testing.T, got map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, _ := got["messages"].([]interface{})
	var msgs []map[string]interface{}
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

// TestClaudeCode_LocalSessionShapes uses reconstructed fixtures.
// The two local Claude project transcript directories for this repo contain
// these content shapes and no others: string user text, assistant text,
// assistant thinking (thinking + signature), assistant tool_use, user
// tool_result as a string, and user tool_result as an array of text blocks.
// Values here are fake; they are not transcript excerpts.
func TestClaudeCode_LocalSessionShapes(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":64,
		"messages":[
			{"role":"user","content":"please edit the file"},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"look at the function","signature":"sig-placeholder"},
				{"type":"text","text":"I will edit it"},
				{"type":"tool_use","id":"toolu_01","name":"Edit","input":{"file_path":"a.go"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_01","content":"edited"}
			]},
			{"role":"assistant","content":[{"type":"thinking","thinking":"check the result","signature":"sig-2"}]},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_02","name":"Read","input":{"file_path":"b.go"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_02","content":[{"type":"text","text":"line 1"},{"type":"text","text":"line 2"}]}]}
		]
	}`)
	msgs := messagesOf(t, got)
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		roles = append(roles, m["role"].(string))
	}
	// The thinking-only turn has no tool call, so it stays an assistant message.
	// The tool_use that follows a string result must keep call-then-result order
	// for the paired call that came before it.
	var paired bool
	for i := 0; i+1 < len(msgs); i++ {
		if msgs[i]["role"] != "assistant" {
			continue
		}
		calls, _ := msgs[i]["tool_calls"].([]interface{})
		if len(calls) != 1 {
			continue
		}
		call := calls[0].(map[string]interface{})
		if call["id"] != "toolu_01" {
			continue
		}
		fn := call["function"].(map[string]interface{})
		if fn["name"] != "Edit" {
			t.Errorf("tool name = %v", fn["name"])
		}
		if msgs[i+1]["role"] != "tool" || msgs[i+1]["tool_call_id"] != "toolu_01" || msgs[i+1]["content"] != "edited" {
			t.Errorf("tool result after Edit = %#v", msgs[i+1])
		} else {
			paired = true
		}
		rc, _ := msgs[i]["reasoning_content"].(string)
		if !strings.Contains(rc, "look at the function") {
			t.Errorf("reasoning_content = %q", rc)
		}
		content, _ := msgs[i]["content"].(string)
		if !strings.Contains(content, "I will edit it") {
			t.Errorf("assistant content = %q", content)
		}
	}
	if !paired {
		t.Fatalf("Edit tool_use was not followed by its tool result: %#v", msgs)
	}
	var arrayResult string
	var thinkingOnly string
	for _, m := range msgs {
		if m["role"] == "tool" && m["tool_call_id"] == "toolu_02" {
			arrayResult, _ = m["content"].(string)
		}
		if m["role"] == "assistant" {
			rc, _ := m["reasoning_content"].(string)
			if strings.Contains(rc, "check the result") {
				thinkingOnly = rc
			}
		}
	}
	if arrayResult != "line 1line 2" {
		t.Errorf("array tool_result content = %q, want both text blocks", arrayResult)
	}
	if thinkingOnly == "" {
		t.Errorf("thinking-only assistant lost its thinking text; roles=%v", roles)
	}
}

func TestClaudeCode_ThinkingTextReachesReasoningContent(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":128,
		"messages":[{"role":"assistant","content":[
			{"type":"thinking","thinking":"plan the edit","signature":"sig"},
			{"type":"text","text":"done"}
		]}]
	}`)
	var assistant map[string]interface{}
	for _, m := range messagesOf(t, got) {
		if m["role"] == "assistant" {
			assistant = m
		}
	}
	if assistant == nil {
		t.Fatal("assistant message missing")
	}
	if rc, _ := assistant["reasoning_content"].(string); !strings.Contains(rc, "plan the edit") {
		t.Errorf("reasoning_content = %q, want the thinking text", rc)
	}
	if content, _ := assistant["content"].(string); !strings.Contains(content, "done") {
		t.Errorf("content = %q, want the text block", content)
	}
}

func TestClaudeCode_ServerToolUseAndResult(t *testing.T) {
	// Official shape: the call and the web_search_result array share one assistant message.
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":128,
		"messages":[{"role":"assistant","content":[
			{"type":"server_tool_use","id":"srvtoolu_01","name":"web_search","input":{"query":"x"}},
			{"type":"web_search_tool_result","tool_use_id":"srvtoolu_01","content":[
				{"type":"web_search_result","title":"First","url":"https://a.example","encrypted_content":"opaque","page_age":"2 days ago"},
				{"type":"web_search_result","title":"Second","url":"https://b.example","encrypted_content":"opaque"}
			]}
		]}]
	}`)
	msgs := messagesOf(t, got)
	if len(msgs) < 2 {
		t.Fatalf("messages = %#v, want assistant tool_calls then a tool message", msgs)
	}
	if msgs[0]["role"] != "assistant" {
		t.Fatalf("first message role = %v, want assistant before the tool result", msgs[0]["role"])
	}
	calls, _ := msgs[0]["tool_calls"].([]interface{})
	if len(calls) != 1 {
		t.Fatalf("tool_calls = %#v", msgs[0]["tool_calls"])
	}
	call := calls[0].(map[string]interface{})
	fn := call["function"].(map[string]interface{})
	if call["id"] != "srvtoolu_01" || fn["name"] != "web_search" {
		t.Errorf("tool call = %#v", call)
	}
	if msgs[1]["role"] != "tool" || msgs[1]["tool_call_id"] != "srvtoolu_01" {
		t.Fatalf("second message = %#v, want tool result for srvtoolu_01", msgs[1])
	}
	content, _ := msgs[1]["content"].(string)
	if !strings.Contains(content, "First") || !strings.Contains(content, "https://a.example") || !strings.Contains(content, "Second") || !strings.Contains(content, "https://b.example") {
		t.Errorf("tool content = %q, want both titles and urls", content)
	}
	if strings.Contains(content, "https://a.exampleSecond") {
		t.Errorf("tool content = %q, second title is glued to the first url", content)
	}
	t.Logf("web_search tool content=%q", content)
}

func TestClaudeCode_CodeExecutionAndEditorResults(t *testing.T) {
	t.Run("bash object", func(t *testing.T) {
		got := convertClaudeRequest(t, `{
			"model":"claude-sonnet-4-6","max_tokens":32,
			"messages":[
				{"role":"assistant","content":[{"type":"server_tool_use","id":"srvtoolu_bash","name":"bash_code_execution","input":{"command":"ls"}}]},
				{"role":"user","content":[{"type":"bash_code_execution_tool_result","tool_use_id":"srvtoolu_bash","content":{"type":"bash_code_execution_result","stdout":"file.txt\n","stderr":"warn\n","return_code":0}}]}
			]
		}`)
		var tool map[string]interface{}
		for _, m := range messagesOf(t, got) {
			if m["role"] == "tool" && m["tool_call_id"] == "srvtoolu_bash" {
				tool = m
			}
		}
		if tool == nil {
			t.Fatal("bash result was not a tool message")
		}
		content, _ := tool["content"].(string)
		if !strings.Contains(content, "file.txt") || !strings.Contains(content, "warn") || !strings.Contains(content, "return_code=0") {
			t.Errorf("tool content = %q, want stdout, stderr, and return_code", content)
		}
	})

	t.Run("text_editor_code_execution_tool_result", func(t *testing.T) {
		got := convertClaudeRequest(t, `{
			"model":"claude-sonnet-4-6","max_tokens":32,
			"messages":[{"role":"assistant","content":[
				{"type":"server_tool_use","id":"srvtoolu_ed","name":"text_editor_code_execution","input":{"command":"view","path":"a.go"}},
				{"type":"text_editor_code_execution_tool_result","tool_use_id":"srvtoolu_ed","content":{"type":"text_editor_code_execution_result","content":"package main"}}
			]}]
		}`)
		msgs := messagesOf(t, got)
		if len(msgs) < 2 || msgs[0]["role"] != "assistant" || msgs[1]["role"] != "tool" {
			t.Fatalf("messages = %#v, want assistant then tool", msgs)
		}
		content, _ := msgs[1]["content"].(string)
		if !strings.Contains(content, "package main") {
			t.Errorf("tool content = %q, want the editor result body", content)
		}
		for _, m := range msgs {
			if m["role"] == "user" {
				t.Errorf("editor result was left as a user message: %#v", m)
			}
		}
	})

	t.Run("web_fetch_tool_result", func(t *testing.T) {
		got := convertClaudeRequest(t, `{
			"model":"claude-sonnet-4-6","max_tokens":32,
			"messages":[{"role":"assistant","content":[
				{"type":"server_tool_use","id":"srvtoolu_fetch","name":"web_fetch","input":{"url":"https://example.com/doc"}},
				{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_fetch","content":{"type":"web_fetch_result","url":"https://example.com/article","content":{"type":"document","source":{"type":"text","media_type":"text/plain","data":"full article text"}}}}
			]}]
		}`)
		var tool map[string]interface{}
		for _, m := range messagesOf(t, got) {
			if m["role"] == "tool" && m["tool_call_id"] == "srvtoolu_fetch" {
				tool = m
			}
		}
		if tool == nil {
			t.Fatal("web_fetch result was dropped")
		}
		content, _ := tool["content"].(string)
		if !strings.Contains(content, "https://example.com/article") || !strings.Contains(content, "full article text") {
			t.Errorf("tool content = %q, want the url and document.source.data", content)
		}
		t.Logf("web_fetch tool content=%q", content)
	})
}

func TestClaudeCode_CompactionBecomesText(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":32,
		"messages":[{"role":"user","content":[
			{"type":"compaction","content":"summary of prior turn"},
			{"type":"text","text":"continue"}
		]}]
	}`)
	var user map[string]interface{}
	for _, m := range messagesOf(t, got) {
		if m["role"] == "user" {
			user = m
		}
	}
	content, _ := user["content"].(string)
	if !strings.Contains(content, "summary of prior turn") || !strings.Contains(content, "continue") {
		t.Errorf("content = %q, want compaction summary and the following text", content)
	}
}

func TestClaudeCode_DisableParallelToolUse(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":32,
		"tools":[{"name":"bash","description":"run","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"auto","disable_parallel_tool_use":true},
		"messages":[{"role":"user","content":"hi"}]
	}`)
	if got["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto", got["tool_choice"])
	}
	parallel, ok := got["parallel_tool_calls"].(bool)
	if !ok || parallel {
		t.Errorf("parallel_tool_calls = %v, want false", got["parallel_tool_calls"])
	}
}

func TestClaudeCode_ImageURLSource(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":32,
		"messages":[{"role":"user","content":[
			{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}},
			{"type":"text","text":"what is this"}
		]}]
	}`)
	var user map[string]interface{}
	for _, m := range messagesOf(t, got) {
		if m["role"] == "user" {
			user = m
		}
	}
	parts, _ := user["content"].([]interface{})
	var sawURL bool
	for _, p := range parts {
		part, _ := p.(map[string]interface{})
		if part["type"] != "image_url" {
			continue
		}
		img, _ := part["image_url"].(map[string]interface{})
		if img["url"] == "https://example.com/a.png" {
			sawURL = true
		}
	}
	if !sawURL {
		t.Errorf("image url source was dropped: %#v", user["content"])
	}
}

func TestClaudeCode_DocumentBlockDoesNotPanic(t *testing.T) {
	got := convertClaudeRequest(t, `{
		"model":"claude-sonnet-4-6","max_tokens":32,
		"messages":[{"role":"user","content":[
			{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"QQ=="}},
			{"type":"text","text":"summarize"}
		]}]
	}`)
	var user map[string]interface{}
	for _, m := range messagesOf(t, got) {
		if m["role"] == "user" {
			user = m
		}
	}
	content := user["content"]
	blob, _ := json.Marshal(content)
	if !strings.Contains(string(blob), "summarize") {
		t.Errorf("sibling text was lost: %s", blob)
	}
}
