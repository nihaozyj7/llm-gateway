package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

func newSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// streamStarted 判断响应是否已写入(流式降级用)
func streamStarted(w http.ResponseWriter) bool {
	// net/http 中可用 responseWriter 内部状态判断;这里通过接口探测
	type sniffed interface{ Written() bool }
	if sw, ok := w.(sniffed); ok {
		return sw.Written()
	}
	return false
}
