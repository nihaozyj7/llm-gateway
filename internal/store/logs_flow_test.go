package store

import (
	"testing"
	"time"

	"gateway/internal/model"
)

// TestPendingLogLifecycle 验证实时日志状态流转:
// InsertPendingLog(等待中)→ UpdateLogStatus(传输中)→ UpdateLogFinal(最终状态)更新同一条记录。
func TestPendingLogLifecycle(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()

	// 1. 请求到达:插入「等待中」日志
	id, err := st.InsertPendingLog(&model.RequestLog{
		RequestTime:    now,
		RequestID:      "req_flow_001",
		APIKeyName:     "测试密钥",
		Model:          "gpt-x",
		SourceIP:       "127.0.0.1",
		PayloadRequest: `{"model":"gpt-x"}`,
		IsStream:       true,
	})
	if err != nil {
		t.Fatalf("InsertPendingLog: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertPendingLog id = %d, want > 0", id)
	}

	got, err := st.GetLog(id)
	if err != nil || got == nil {
		t.Fatalf("GetLog pending: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("初始状态 = %q, want %q", got.Status, "pending")
	}
	if got.Model != "gpt-x" || got.APIKeyName != "测试密钥" || got.SourceIP != "127.0.0.1" {
		t.Errorf("pending 日志应携带请求阶段信息: %+v", got)
	}
	if !got.IsStream {
		t.Error("IsStream 应落库")
	}

	// 2. 流式首包:状态 等待中 → 传输中
	if err := st.UpdateLogStatus(id, "streaming"); err != nil {
		t.Fatalf("UpdateLogStatus: %v", err)
	}
	got, _ = st.GetLog(id)
	if got.Status != "streaming" {
		t.Errorf("状态 = %q, want %q", got.Status, "streaming")
	}

	// 3. 请求结束:更新同一条记录为最终状态(含 token/耗时/渠道等)
	if err := st.UpdateLogFinal(&model.RequestLog{
		ID:               id,
		RequestTime:      now,
		RequestID:        "req_flow_001",
		APIKeyName:       "测试密钥",
		ChannelID:        3,
		ChannelName:      "chan-a",
		Model:            "gpt-x",
		UpstreamModel:    "gpt-x-0414",
		Status:           "success",
		IsStream:         true,
		LatencyMs:        1234,
		FirstResponseMs:  300,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CacheReadTokens:  20,
		Cost:             0.001,
		Error:            "",
		SourceIP:         "127.0.0.1",
		PayloadRequest:   `{"model":"gpt-x"}`,
		PayloadResponse:  `{"usage":{}}`,
		ChannelTrail:     `[{"channel_id":3,"channel_name":"chan-a","ok":true}]`,
	}); err != nil {
		t.Fatalf("UpdateLogFinal: %v", err)
	}

	got, _ = st.GetLog(id)
	if got.Status != "success" {
		t.Errorf("最终状态 = %q, want %q", got.Status, "success")
	}
	if got.ChannelName != "chan-a" || got.UpstreamModel != "gpt-x-0414" {
		t.Errorf("最终渠道/上游模型未更新: %+v", got)
	}
	if got.LatencyMs != 1234 || got.FirstResponseMs != 300 || got.TotalTokens != 150 {
		t.Errorf("最终耗时/token 未更新: %+v", got)
	}
	if got.ChannelTrail == "" {
		t.Error("ChannelTrail 应更新")
	}

	// 总数应保持 1 条(同一行被更新,而非新插入)
	logs, total, err := st.ListLogs(nil, "", "", "", "", 0, 10)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Errorf("日志总数 = %d, want 1(应更新同一条记录)", total)
	}
	if logs[0].ID != id {
		t.Errorf("应更新 id=%d 的行,实际首行 id=%d", id, logs[0].ID)
	}
}

// TestFinishStaleLogs 遗留进行中日志(等待中/传输中)在网关启动时应标记为失败。
func TestFinishStaleLogs(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()

	// 一条 pending、一条 streaming、一条已完成的 success
	id1, _ := st.InsertPendingLog(&model.RequestLog{RequestTime: now, RequestID: "r1", Model: "m"})
	id2, _ := st.InsertPendingLog(&model.RequestLog{RequestTime: now, RequestID: "r2", Model: "m"})
	_ = st.UpdateLogStatus(id2, "streaming")
	_, _ = st.InsertLog(&model.RequestLog{
		RequestTime: now, RequestID: "r3", Model: "m", Status: "success", LatencyMs: 5,
	})

	if err := st.FinishStaleLogs(); err != nil {
		t.Fatalf("FinishStaleLogs: %v", err)
	}

	g1, _ := st.GetLog(id1)
	if g1.Status != "fail" || g1.Error == "" {
		t.Errorf("pending 遗留应标 fail,实际 status=%q error=%q", g1.Status, g1.Error)
	}
	g2, _ := st.GetLog(id2)
	if g2.Status != "fail" {
		t.Errorf("streaming 遗留应标 fail,实际 status=%q", g2.Status)
	}
	logs, total, _ := st.ListLogs(nil, "", "", "", "", 0, 10)
	if total != 3 {
		t.Errorf("总数 = %d, want 3", total)
	}
	_ = logs
}
