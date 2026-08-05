# 图标与 Windows 资源生成

## 文件说明

| 文件 | 用途 |
|------|------|
| `scripts/gen_icon.py` | 生成图标源文件(渐变分流箭头:多渠道聚合转发) |
| `assets/icon/app_icon_512.png` | 512×512 主图标(网页 favicon PNG + exe 图标源) |
| `assets/icon/favicon.ico` | 多尺寸 ICO(16/32/48/64/128/256,网页 favicon) |
| `web/public/favicon.ico` | 复制到前端 `dist/` 的浏览器标签页图标 |
| `web/public/app-icon.png` | 复制到前端 `dist/` 的 PNG 图标 |
| `cmd/gateway/rsrc_windows_amd64.syso` | go-winres 生成的 Windows 资源对象,`go build` 自动链接进 `gateway.exe` |

## 重新生成

```powershell
# 1. 重新绘制图标(修改 scripts/gen_icon.py 后)
python scripts/gen_icon.py

# 2. 同步前端 public 资源
Copy-Item assets/icon/favicon.ico web/public/favicon.ico
Copy-Item assets/icon/app_icon_512.png web/public/app-icon.png

# 3. 重新生成 Windows 资源对象(仅 Windows amd64;Linux 构建不受影响,自动忽略)
go run github.com/tc-hib/go-winres@latest simply `
  --icon assets/icon/app_icon_512.png `
  --arch amd64 `
  --out rsrc `
  --file-description "大模型转发网关 LLM Gateway" `
  --product-name "LLM Gateway 大模型转发网关" `
  --original-filename "gateway.exe"
# 把生成的 rsrc_windows_amd64.syso 移到 cmd/gateway/ 目录
```

## 说明

- syso 文件放在 `cmd/gateway/`(main 包目录)是 Go 的约定:构建时自动链接该目录下匹配 `GOOS/GOARCH` 的 `*.syso`。
- `build.ps1` 已集成 syso 自动生成(见脚本中 "Windows 资源" 段),普通构建无需手动执行上面第 3 步。
- 图标语义:左侧三条输入箭头(青/靛蓝/紫,代表多渠道)汇聚为右侧一条渐变输出箭头(蓝→绿,代表网关聚合转发)。
