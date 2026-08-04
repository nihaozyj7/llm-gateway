package route

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestBuildUpstreamURL(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://api.openai.com/v1", "/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		{"https://api.deepseek.com", "/v1/chat/completions", "https://api.deepseek.com/chat/completions"},
		{"https://api.example.com/v1/", "/v1/models", "https://api.example.com/v1/models"},
		{"http://localhost:9000", "/v1/models", "http://localhost:9000/models"},
		{"https://api.example.com/v1", "/v1", "https://api.example.com/v1"},
		{"https://api.example.com/v1", "/v1/", "https://api.example.com/v1"},
		{"https://api.example.com/v1/", "/v1/", "https://api.example.com/v1"},
	}
	for _, c := range cases {
		got, err := BuildUpstreamURL(c.base, c.path)
		if err != nil {
			t.Fatalf("BuildUpstreamURL(%q,%q) error: %v", c.base, c.path, err)
		}
		if got != c.want {
			t.Errorf("BuildUpstreamURL(%q,%q)=%q, want %q", c.base, c.path, got, c.want)
		}
	}
	// 非法路径与非法 baseURL 应返回错误
	if _, err := BuildUpstreamURL("https://api.example.com/v1", "/v1evil"); err == nil {
		t.Error("expected error for non-/v1/ path")
	}
	if _, err := BuildUpstreamURL("not-a-url", "/v1/models"); err == nil {
		t.Error("expected error for base without scheme/host")
	}
}

func TestApplyModelMapping(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[]}`)
	got := ApplyModelMapping(body, "gpt-4o-custom")
	want := `{"messages":[],"model":"gpt-4o-custom"}`
	// 仅校验 model 字段被替换
	var g, w map[string]any
	_ = json.Unmarshal(got, &g)
	_ = json.Unmarshal([]byte(want), &w)
	if !reflect.DeepEqual(g["model"], w["model"]) {
		t.Errorf("model=%v, want %v", g["model"], w["model"])
	}
	// 无映射时原样返回
	if !reflect.DeepEqual(ApplyModelMapping(body, ""), body) {
		t.Error("empty mapping should return body unchanged")
	}
}

func TestParseUsage(t *testing.T) {
	body := []byte(`{"id":"x","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	u := parseUsage(body)
	if u == nil || u.PromptTokens != 10 || u.CompletionTokens != 20 || u.TotalTokens != 30 {
		t.Fatalf("parseUsage = %+v", u)
	}
	// total 缺失时补全
	body2 := []byte(`{"usage":{"prompt_tokens":5,"completion_tokens":7}}`)
	u2 := parseUsage(body2)
	if u2 == nil || u2.TotalTokens != 12 {
		t.Fatalf("parseUsage missing total = %+v", u2)
	}
	// 无 usage
	if parseUsage([]byte(`{"id":"x"}`)) != nil {
		t.Error("expected nil for no usage")
	}
}

func TestParseStreamUsage(t *testing.T) {
	line := `data: {"id":"c","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`
	u := parseStreamUsage(line)
	if u == nil || u.TotalTokens != 7 {
		t.Fatalf("parseStreamUsage = %+v", u)
	}
	if parseStreamUsage("data: [DONE]") != nil {
		t.Error("[DONE] should return nil")
	}
	if parseStreamUsage(`data: {"choices":[{"delta":{"content":"hi"}}]}`) != nil {
		t.Error("chunk without usage should return nil")
	}
}

func TestCost(t *testing.T) {
	pIn := 3.0    // 元/百万 token
	pOut := 6.0   // 元/百万 token
	pCache := 0.5 // 元/百万 token
	got := Cost(1_000_000, 0, 500_000, &pIn, &pCache, &pOut)
	// 1e6/1e6*3 + 5e5/1e6*6 = 3 + 3
	if got != 6.0 {
		t.Errorf("Cost = %v, want 6", got)
	}
	// 缓存:100 万输入中 40 万命中缓存 → 60万*3/1e6 + 40万*0.5/1e6 = 1.8 + 0.2
	got = Cost(1_000_000, 400_000, 0, &pIn, &pCache, nil)
	if math.Abs(got-2.0) > 1e-9 {
		t.Errorf("Cost cached = %v, want 2.0", got)
	}
	// 未配置缓存单价:缓存部分按输入价计 = 全部输入按输入价
	got = Cost(1_000_000, 400_000, 0, &pIn, nil, nil)
	if got != 3.0 {
		t.Errorf("Cost nil cache = %v, want 3.0", got)
	}
	// 缓存超出输入:按输入量截断
	got = Cost(1_000_000, 2_000_000, 0, &pIn, &pCache, nil)
	if got != 0.5 {
		t.Errorf("Cost cache clamp = %v, want 0.5", got)
	}
	if Cost(1_000_000, 0, 500_000, nil, nil, nil) != 0 {
		t.Error("nil prices should cost 0")
	}
}

func TestIsChannelFailure(t *testing.T) {
	cases := []struct {
		status int
		err    bool
		want   bool
	}{
		{200, false, false},
		{400, false, false}, // 业务错误
		{401, false, false},
		{429, false, true}, // 限流=渠道失败
		{500, false, true},
		{503, false, true},
		{0, true, true}, // 网络错误
	}
	for _, c := range cases {
		if got := isChannelFailure(c.status, boolErr(c.err)); got != c.want {
			t.Errorf("isChannelFailure(%d,%v)=%v, want %v", c.status, c.err, got, c.want)
		}
	}
}

func boolErr(b bool) error {
	if b {
		return &netErr{}
	}
	return nil
}

type netErr struct{}

func (e *netErr) Error() string { return "network error" }

func TestExtractModelID(t *testing.T) {
	if got := ExtractModelID([]byte(`{"model":"abc","stream":true}`)); got != "abc" {
		t.Errorf("ExtractModelID = %q", got)
	}
	if ExtractModelID([]byte(`{"x":1}`)) != "" {
		t.Error("expected empty")
	}
}

func TestIsStreamRequest(t *testing.T) {
	if !IsStreamRequest([]byte(`{"model":"a","stream":true}`)) {
		t.Error("stream:true should be stream")
	}
	if IsStreamRequest([]byte(`{"model":"a"}`)) {
		t.Error("no stream should be non-stream")
	}
}
