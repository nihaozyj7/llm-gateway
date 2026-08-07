# 重建 SQLite 数据库文件:停网关 → 备份 → VACUUM INTO 重建 → 替换。
# 适用场景:清除日志后数据库文件仍过大(历史数据被外部程序追加过垃圾数据、
# freelist 未回收等,SQLite 的 DELETE/VACUUM 无法缩小的尾部内容)。
#
# 用法(需管理员权限,因为网关部署在 Program Files 下):
#   powershell -ExecutionPolicy Bypass -File scripts\rebuild-gateway-db.ps1
# 可选指定数据库路径:
#   powershell ... -DbPath 'C:\Program Files\Apps\本地大模型网关\.data\gateway.db'
param(
    [string]$DbPath = 'C:\Program Files\Apps\本地大模型网关\.data\gateway.db'
)

$ErrorActionPreference = 'Stop'
$dir = Split-Path $DbPath

Write-Host "==> 目标数据库: $DbPath"

# 1. 停止运行中的网关进程(SQLite 文件被进程独占,必须先停)
$procs = @(Get-Process -Name gateway -ErrorAction SilentlyContinue)
if ($procs.Count -gt 0) {
    Write-Host "==> 停止网关进程 (PID: $($procs.Id -join ',')) ..."
    foreach ($p in $procs) { Stop-Process -Id $p.Id -Force }
    Start-Sleep -Seconds 2
} else {
    Write-Host '==> 未发现运行中的网关进程。'
}

# 2. 确认文件已解锁(Windows 释放句柄可能有短暂延迟)
$unlocked = $false
for ($i = 0; $i -lt 10; $i++) {
    try {
        $fs = [System.IO.File]::Open($DbPath, 'Open', 'ReadWrite', 'None')
        $fs.Close()
        $unlocked = $true
        break
    } catch {
        Start-Sleep -Seconds 1
    }
}
if (-not $unlocked) {
    Write-Host '!! 数据库文件仍被占用,无法重建。请确认网关已完全退出后重试。' -ForegroundColor Red
    exit 1
}

# 3. 备份
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$bak = "$DbPath.bak-$stamp"
Copy-Item $DbPath $bak
Write-Host "==> 已备份: $bak ($((Get-Item $bak).Length) bytes)"

# 4. VACUUM INTO 重建(只读打开,导出逻辑数据;排除 freelist 与任何尾部垃圾)
$tmp = Join-Path $env:TEMP ("gateway-rebuilt-" + [guid]::NewGuid().ToString('N') + '.db')
$pyFile = Join-Path $env:TEMP ("rebuild-" + [guid]::NewGuid().ToString('N') + '.py')
$py = @"
import sqlite3, sys
src = 'file:%s?mode=ro' % sys.argv[1].replace('\\', '/')
dst = sys.argv[2].replace('\\', '/')
conn = sqlite3.connect(src, uri=True)
conn.execute("VACUUM INTO '%s'" % dst)
conn.close()
"@
Set-Content -Path $pyFile -Value $py -Encoding UTF8
try {
    python $pyFile $DbPath $tmp
} finally {
    Remove-Item $pyFile -ErrorAction SilentlyContinue
}
Write-Host "==> 重建完成: $tmp ($((Get-Item $tmp).Length) bytes)"

# 5. 替换(必须同时删除旧 -wal/-shm,避免与新库不一致)
Remove-Item "$DbPath-wal", "$DbPath-shm" -ErrorAction SilentlyContinue
Copy-Item $tmp $DbPath -Force
Remove-Item $tmp

Write-Host ''
Write-Host '==> 数据库重建完成!现在可以用 start.bat 或 gateway.exe -config config.json 重新启动网关。' -ForegroundColor Green
