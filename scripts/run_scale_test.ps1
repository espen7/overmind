# 双 home 在线扩容剧本联跑验证
# 流程: 重置环文档 -> 起 home1/home2/world/gate -> 客户端登录+升级(home1)
#       -> ringctl add home2 -> 轮询热切 -> 第二次升级触发在线迁移(home2) -> commit

# 0. 重置集群环文档 (让节点用 yaml [home1] 重新播种 version 1)
#    同时重置玩家数据, 保证 test_player_999 走新建初始化 (Gold 500, Level 1)
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

# 3. 后台启动扩容剧本客户端 (登录 home1 -> 升级 -> 等 10s 窗口 -> 再升级)
Write-Host "启动扩容剧本客户端 (test_player_999, 扩容后应迁移至 home2)..." -ForegroundColor Cyan
$clientProc = Start-Process -FilePath ".\bin\client.exe" -ArgumentList "scale" -PassThru -NoNewWindow -RedirectStandardOutput "client_out.log" -RedirectStandardError "client_err.log"

# 4. 等第一次升级完成后, 在客户端等待窗口内执行改环
Start-Sleep -Seconds 4
Write-Host "`n>>> 执行 ringctl add home2@127.0.0.1 (扩容)..." -ForegroundColor Yellow
.\bin\ringctl.exe add "home2@127.0.0.1"

# 5. 等待客户端完成第二次升级与心跳 (轮询热切 <=3s + 客户端窗口 10s)
Write-Host ">>> 等待各节点热切环与客户端触发在线迁移..." -ForegroundColor Yellow
Start-Sleep -Seconds 13

# 6. 迁移收敛后 commit 清空 prev_nodes
Write-Host "`n>>> 执行 ringctl commit..." -ForegroundColor Yellow
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
Get-Content -Path "gate_out.log" -Tail 50 -ErrorAction SilentlyContinue
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
