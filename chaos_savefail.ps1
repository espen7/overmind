# 一次性故障注入验证: SaveService 失败回投重试 (Mongo 中断 -> 恢复 -> 增量最终落地)
$ErrorActionPreference = "Continue"

Write-Host "[Chaos] 启动三节点..." -ForegroundColor Green
$homeProc = Start-Process -FilePath ".\bin\home.exe" -PassThru -NoNewWindow -RedirectStandardOutput "home_out.log" -RedirectStandardError "home_err.log"
Start-Sleep -Seconds 2
$worldProc = Start-Process -FilePath ".\bin\world.exe" -PassThru -NoNewWindow -RedirectStandardOutput "world_out.log" -RedirectStandardError "world_err.log"
Start-Sleep -Seconds 2
$gateProc = Start-Process -FilePath ".\bin\gate.exe" -PassThru -NoNewWindow -RedirectStandardOutput "gate_out.log" -RedirectStandardError "gate_err.log"
Start-Sleep -Seconds 2

Write-Host "[Chaos] 后台启动客户端 (登录+升级产生脏数据)..." -ForegroundColor Cyan
$clientProc = Start-Process -FilePath ".\bin\client.exe" -PassThru -NoNewWindow -RedirectStandardOutput "client_out.log" -RedirectStandardError "client_err.log"

# 升级发生在客户端启动后约 1-2 秒, 5 秒异步落盘 tick 尚未触发
Start-Sleep -Seconds 3
Write-Host "[Chaos] === 注入故障: docker stop overmind-mongo ===" -ForegroundColor Red
docker stop overmind-mongo | Out-Null

Write-Host "[Chaos] Mongo 已停, 等待 15 秒 (期间落盘应失败并回投退避重试)..." -ForegroundColor Yellow
Start-Sleep -Seconds 15

Write-Host "[Chaos] === 恢复: docker start overmind-mongo ===" -ForegroundColor Green
docker start overmind-mongo | Out-Null

Write-Host "[Chaos] 等待 30 秒让退避重试命中..." -ForegroundColor Yellow
Start-Sleep -Seconds 30

Write-Host "`n[Chaos] === 查库验证: 玩家文档最终状态 ===" -ForegroundColor Magenta
docker exec overmind-mongo mongosh overmind --quiet --eval "printjson(db.players.findOne({_id:'test_player_999'}))"

Write-Host "`n[Chaos] 清理节点进程..." -ForegroundColor Yellow
Stop-Process -Id $homeProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $worldProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $gateProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $clientProc.Id -Force -ErrorAction SilentlyContinue

Write-Host "`n=============================================== " -ForegroundColor Magenta
Write-Host "  HOME 节点 SaveService 相关日志 (重试证据链)" -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "home_out.log" -ErrorAction SilentlyContinue | Select-String "SaveService|落盘|PlayerActor|退位"

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "  HOME 节点完整错误输出" -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "home_err.log" -Tail 20 -ErrorAction SilentlyContinue

Remove-Item -Path "*_out.log", "*_err.log" -ErrorAction SilentlyContinue
