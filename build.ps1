# 构建脚本:前端 + Go 单二进制
# 用法:./build.ps1 [-Windows] [-Linux]  无参数默认构建 Windows;可同时指定构建两个平台
param(
    [switch]$Linux,
    [switch]$Windows
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot

Write-Host "==> 构建前端..."
Push-Location (Join-Path $root "web")
try {
    if (-not (Test-Path "node_modules")) { npm install }
    npm run build
} finally {
    Pop-Location
}

Write-Host "==> 编译 Go 二进制..."
$env:GOPROXY = "https://goproxy.cn,direct"
$goBin = "$env:ProgramFiles\Go\bin"
if (Test-Path "$goBin\go.exe") { $env:Path = "$goBin;" + $env:Path }

# Windows 资源(syso):gateway.exe 图标与版本信息。
# 仅在 Windows 目标构建前确保存在;Linux 目标不需要。
function Ensure-WindowsRes {
    $syso = Join-Path $root "cmd\gateway\rsrc_windows_amd64.syso"
    $icon = Join-Path $root "assets\icon\app_icon_512.png"
    if (Test-Path $syso) { return }
    if (-not (Test-Path $icon)) {
        Write-Host "警告: 未找到 $icon,gateway.exe 将不带图标。"
        return
    }
    Write-Host "==> 生成 Windows 资源(syso)..."
    go run github.com/tc-hib/go-winres@latest simply `
        --icon $icon `
        --arch amd64 `
        --out (Join-Path $root "rsrc") `
        --file-description "大模型转发网关 LLM Gateway" `
        --product-name "LLM Gateway 大模型转发网关" `
        --original-filename "gateway.exe"
    $gen = Join-Path $root "rsrc_windows_amd64.syso"
    if (Test-Path $gen) { Move-Item $gen $syso -Force }
}

if ($Linux) {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $root "gateway-linux") ./cmd/gateway
    Write-Host "已生成: gateway-linux"
}
if ($Windows -or (-not $Linux -and -not $Windows)) {
    Ensure-WindowsRes
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $root "gateway.exe") ./cmd/gateway
    Write-Host "已生成: gateway.exe"
}

Write-Host "构建完成。运行 ./gateway(.exe) -config config.json 启动。"
