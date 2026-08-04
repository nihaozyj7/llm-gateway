// Package webassets 将前端构建产物嵌入 Go 二进制。
// 注意:go build 前需先执行 npm run build 生成 dist/。
package webassets

import "embed"

//go:embed all:dist
var FS embed.FS
