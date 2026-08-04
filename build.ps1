# 构建脚本:前端 + Go 单二进制
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

if ($Linux) {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $root "gateway-linux") ./cmd/gateway
    Write-Host "已生成: gateway-linux"
} else {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $root "gateway.exe") ./cmd/gateway
    Write-Host "已生成: gateway.exe"
}

Write-Host "构建完成。运行 ./gateway(.exe) -config config.json 启动。"
