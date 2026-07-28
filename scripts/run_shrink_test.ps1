# 扩容+缩容(节点退役)全剧本联跑验证
# 流程: 重置环文档/玩家 -> 起 home1/home2/world/gate -> 客户端登录+升级①(home1)
#       -> ringctl add home2 -> 升级②在线迁移(home1->home2)
#       -> ringctl commit + remove home2 -> 升级③反向握手迁回(home2->home1)
#       -> ringctl commit (此后 home2 才可安全停机)

# 0. 重置集群环文档与玩家数据 (节点用 yaml [home1] 重新播种 version 1)
Write-Host "重置 Mongo 环文档与玩家数据..." -ForegroundColor Yellow
docker exec overmind-mongo mongosh overmind --quiet --eval "db.cluster_meta.deleteMany({}); db.players.deleteMany({})" | Out-Null

# 1. 启动两个 Home 节点
Write-Host "正在启动 Home1 / Home2 节点..." -ForegroundColor Green
$home1Proc = Start-Process -FilePath ".\bin\home.exe" -PassThru -NoNewWindow -RedirectStandardOutput "home1_out.log" -RedirectStandardError "home1_err.log"
$home2Proc = Start-Process -FilePath ".\bin\home.exe" -ArgumentList "-config","configs\home2.yaml" -PassThru -NoNewWindow -RedirectStandardOutput "home2_out.log" -RedirectStandardError "home2_err.log"
Start-Sleep -Seconds 2

# 2. 启动 World 与 Gate 节点
Write-Host "正在启动 World / Gate 节点..." -ForegroundColor Green
$worldProc = Start-Process -FilePath ".\bin\world.exe" -PassThru -NoNewWindow -RedirectStandardOutput "world_out.log" -RedirectStandardError "world_err.log"
Start-Sleep -Seconds 1
$gateProc = Start-Process -FilePath ".\bin\gate.exe" -PassThru -NoNewWindow -RedirectStandardOutput "gate_out.log" -RedirectStandardError "gate_err.log"
Start-Sleep -Seconds 2

# 3. 后台启动缩容剧本客户端 (升级① -> 窗口A 10s -> 升级② -> 窗口B 10s -> 升级③)
Write-Host "启动缩容剧本客户端 (test_player_999: home1 -> home2 -> home1)..." -ForegroundColor Cyan
$clientProc = Start-Process -FilePath ".\bin\client.exe" -ArgumentList "shrink" -PassThru -NoNewWindow -RedirectStandardOutput "client_out.log" -RedirectStandardError "client_err.log"

# 4. 窗口 A: 升级①完成后执行扩容改环
Start-Sleep -Seconds 4
Write-Host "`n>>> 执行 ringctl add home2@127.0.0.1 (扩容)..." -ForegroundColor Yellow
.\bin\ringctl.exe add "home2@127.0.0.1"

# 5. 窗口 B: 升级②迁移收敛后, 先 commit 扩容再 remove home2 (缩容)
Start-Sleep -Seconds 10
Write-Host "`n>>> 执行 ringctl commit (扩容收敛)..." -ForegroundColor Yellow
.\bin\ringctl.exe commit
Write-Host ">>> 执行 ringctl remove home2@127.0.0.1 (缩容)..." -ForegroundColor Yellow
.\bin\ringctl.exe remove "home2@127.0.0.1"

# 6. 等升级③反向迁移收敛后, 最终 commit (此后 home2 才允许停机)
Start-Sleep -Seconds 13
Write-Host "`n>>> 执行 ringctl commit (缩容收敛, home2 可安全停机)..." -ForegroundColor Yellow
.\bin\ringctl.exe commit
.\bin\ringctl.exe show

# 7. 清理节点进程
Start-Sleep -Seconds 2
Write-Host "`n清理并停止所有微服务节点..." -ForegroundColor Yellow
Stop-Process -Id $clientProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $home1Proc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $home2Proc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $worldProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $gateProc.Id -Force -ErrorAction SilentlyContinue

# 8. 打印各方日志
Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           CLIENT LOGS                         " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "client_out.log" -ErrorAction SilentlyContinue
Get-Content -Path "client_err.log" -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           GATE NODE LOGS                      " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "gate_out.log" -Tail 60 -ErrorAction SilentlyContinue
Get-Content -Path "gate_err.log" -Tail 20 -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           HOME1 NODE LOGS                     " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "home1_out.log" -Tail 50 -ErrorAction SilentlyContinue
Get-Content -Path "home1_err.log" -Tail 20 -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           HOME2 NODE LOGS                     " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "home2_out.log" -Tail 50 -ErrorAction SilentlyContinue
Get-Content -Path "home2_err.log" -Tail 20 -ErrorAction SilentlyContinue

Write-Host "`n===============================================" -ForegroundColor Magenta
Write-Host "           WORLD NODE LOGS                     " -ForegroundColor Magenta
Write-Host "===============================================" -ForegroundColor Magenta
Get-Content -Path "world_out.log" -Tail 30 -ErrorAction SilentlyContinue
Get-Content -Path "world_err.log" -Tail 20 -ErrorAction SilentlyContinue

# 清理产生的临时日志文件
Remove-Item -Path "*_out.log", "*_err.log" -ErrorAction SilentlyContinue
