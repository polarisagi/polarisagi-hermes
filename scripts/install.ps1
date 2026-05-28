Write-Host "🌌 正在安装/更新 Polaris Hermes (Windows)..." -ForegroundColor Cyan

# 检查管理员权限
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "❌ 权限不足！请右键点击 PowerShell 并选择“以管理员身份运行”(Run as Administrator) 来执行此脚本。" -ForegroundColor Red
    exit 1
}

$Repo = "polarisagi/polarisagi-hermes"
$BinName = "polarisagi-hermes.exe"
$InstallDir = "C:\ProgramData\PolarisGateway"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

$Arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$DownloadUrl = "https://github.com/$Repo/releases/latest/download/polarisagi-hermes-windows-$Arch.exe"

$TaskName = "PolarisGatewayService"

# 停止可能正在运行的服务和进程，以便覆盖文件
if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
    Write-Host "🛑 正在停止运行中的后台服务..." -ForegroundColor Cyan
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

$Process = Get-Process -Name "polarisagi-hermes" -ErrorAction SilentlyContinue
if ($Process) {
    Write-Host "🛑 正在结束旧进程..." -ForegroundColor Cyan
    Stop-Process -Name "polarisagi-hermes" -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

Write-Host "⬇️ 正在从 GitHub 下载最新版本: $DownloadUrl"
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile "$InstallDir\$BinName" -UseBasicParsing
} catch {
    Write-Host "❌ 下载失败。请检查网络或确认仓库是否已发布 Release。" -ForegroundColor Red
    exit 1
}

Write-Host "⚙️ 正在注册 Windows 计划任务以实现开机后台静默自启..."
$Action = New-ScheduledTaskAction -Execute "$InstallDir\$BinName" -WorkingDirectory $InstallDir
$Trigger = New-ScheduledTaskTrigger -AtStartup
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -DontStopOnIdleEnd
$Principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

Register-ScheduledTask -Action $Action -Trigger $Trigger -Settings $Settings -Principal $Principal -TaskName $TaskName -Force | Out-Null

Write-Host "▶️ 正在启动服务..."
Start-ScheduledTask -TaskName $TaskName

Write-Host "🎉 安装完成！Polaris Hermes 已在后台服务运行。" -ForegroundColor Green
Write-Host "请打开浏览器访问: http://127.0.0.1:27777/dashboard 进入控制台" -ForegroundColor Yellow
