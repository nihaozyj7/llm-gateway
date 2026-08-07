# AGENTS.md — 大模型转发网关 (LLM Gateway)

OpenAI 兼容的轻量级多渠道 LLM 转发网关:密钥轮询回退代理(同渠道重试 + 按优先级降级 + 连续失败冷却),透传上游,不做协议转换/聚合。Go 单二进制(前端 embed)+ SQLite,零外部依赖。

## Project
- 栈:Go 1.26(仅依赖 `x/crypto`、`modernc.org/sqlite`,免 CGO)+ Vue3/Vite/Tailwind/ECharts 管理界面
- 入口:`cmd/gateway/main.go`(装配 store → router → handlers,embed `web/dist` 前端)
- 数据:`config.json` + SQLite(`.data/gateway.db`,WAL)
- 版本号:`cmd/gateway/main.go` 中 `var version = "0.8.0"`

## Commands
- 完整构建(前端 + 二进制,Windows 默认):`./build.ps1`;加 `-Linux` 交叉编译 Linux,`-Windows -Linux` 两者都要
- 手动构建:`cd web && npm install && npm run build` → `go build -o gateway.exe ./cmd/gateway`
- 测试:`go test ./...`(已验证通过;有测试:route、server、store)
- 静态检查:`go vet ./...`
- 运行:`./gateway.exe -config config.json`(或 `go run ./cmd/gateway`);`start.bat` 双击启动
- 前端开发:`cd web && npm run dev`(Vite,热更新;改完需 `npm run build` 重新 embed 后编译 Go)

## Architecture
- `internal/config` — 配置加载/默认值/持久化(`config.json`;首次启动自动生成并保存 `session_secret`)
- `internal/model` — 纯数据模型:`Channel`(`Type`:openai/anthropic/responses,决定出站鉴权头与路径透传形态)/ `Model`(同名全局唯一)/ `ChannelModel`(渠道内模型名映射 + 模型级优先级)/ `APIKey` / `RequestLog` / `StatRow`
- `internal/store` — SQLite 访问层(modernc.org/sqlite,DSN 带 WAL + busy_timeout + foreign_keys;`migrate()` 顺序建表/迁移;渠道/模型/日志/统计/API key CRUD;渠道失败计数与 cooldown 状态)
- `internal/route` — 核心引擎:`Router.Handle`(非流式)/ `HandleStream`(流式,仅首包前失败可重试降级)。`PickChannels` 按模型级优先级取候选;失败先同渠道重试一次(`RetrySameChannel`),再降级下一渠道;连续失败达 `CooldownThreshold` 置 cooldown;`doOnce`/`doStreamOnce` 向上游透传,`ApplyModelMapping` 替换模型名
- `internal/server` — HTTP 层:`GatewayHandler`(`/v1/*`,Bearer API key 鉴权,SHA-256 哈希查库,`/v1/*` 开 CORS 但 `/api/admin/*` 不开)、`AdminHandler`(`/api/admin/*` 仅本机,无登录)、请求日志记录(logging.go,含 `retry_success` 等状态)
- `web/` — 管理界面(Vue3 + Vite + Tailwind + ECharts),`dist` 经 `web/embed.go` 内嵌;SPA history 路由由 `spaFallback` 处理

## Conventions
- 注释用中文;公开标识符带中文 doc 注释
- 时间戳一律 Unix 毫秒(INTEGER);价格单位:元/百万 token;token 用 int64
- 渠道错误语义:`Result.ChannelFail` = 可重试/降级(网络、5xx);`BizError` = 业务错误(4xx),直接返回不重试不降级
- API key 存储用 SHA-256 哈希(`hashKey`);不打印请求/响应体明文日志
- 数据库变更走 `store.go` 的 `migrate()` 追加 SQL,不在外部改库
- 仅本机可访问的约束(管理 API)不能开 CORS;`/v1/*` 的 CORS 是特性,勿移除
- 新增依赖谨慎:项目刻意保持"标准库 + sqlite + x/crypto"的零依赖形态

## Notes
- (待补充:部署/故障排查经验、渠道 API 兼容性注意点等)
