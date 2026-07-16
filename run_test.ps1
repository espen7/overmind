# 1. 启动 Home 节点
Write-Host "正在启动 Home 节点..." -ForegroundColor Green
$homeProc = Start-Process -FilePath ".\bin\home.exe" -PassThru -NoNewWindow -RedirectStandardOutput "home_out.log" -RedirectStandardError "home_err.log"
Start-Sleep -Seconds 2

# 2. 启动 World 节点
Write-Host "正在启动 World 节点..." -ForegroundColor Green
$worldProc = Start-Process -FilePath ".\bin\world.exe" -PassThru -NoNewWindow -RedirectStandardOutput "world_out.log" -RedirectStandardError "world_err.log"
Start-Sleep -Seconds 2

# 3. 启动 Gate 节点
Write-Host "正在启动 Gate 节点..." -ForegroundColor Green
$gateProc = Start-Process -FilePath ".\bin\gate.exe" -PassThru -NoNewWindow -RedirectStandardOutput "gate_out.log" -RedirectStandardError "gate_err.log"
Start-Sleep -Seconds 2

# 4. 执行测试客户端
Write-Host "开始执行 Mock 客户端..." -ForegroundColor Cyan
.\bin\client.exe

# 5. 等待 2 秒捕获退出清理日志
Start-Sleep -Seconds 2

# 6. 清理节点进程
Write-Host "清理并停止所有微服务节点..." -ForegroundColor Yellow
Stop-Process -Id $homeProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $worldProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $gateProc.Id -Force -ErrorAction SilentlyContinue

# 7. 打印并核对各节点的运行日志
Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           GATE NODE LOGS                      " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "gate_out.log" -Tail 40 -ErrorAction SilentlyContinue
Get-Content -Path "gate_err.log" -Tail 20 -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           HOME NODE LOGS                      " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "home_out.log" -Tail 40 -ErrorAction SilentlyContinue
Get-Content -Path "home_err.log" -Tail 20 -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           WORLD NODE LOGS                     " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "world_out.log" -Tail 40 -ErrorAction SilentlyContinue
Get-Content -Path "world_err.log" -Tail 20 -ErrorAction SilentlyContinue

# 清理产生的临时日志文件
Remove-Item -Path "*_out.log", "*_err.log" -ErrorAction SilentlyContinue
