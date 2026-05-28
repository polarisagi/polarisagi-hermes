# 检查管理员权限
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "❌ 权限不足！请右键点击 PowerShell 并选择“以管理员身份运行”(Run as Administrator) 来执行此脚本。" -ForegroundColor Red
    exit 1
}

Write-Host "🗑️ 正在卸载 Polaris Hermes (Windows)..." -ForegroundColor Cyan

$TaskName = "PolarisGatewayService"
$InstallDir = "C:\ProgramData\PolarisGateway"
$BinName = "polarisagi-hermes.exe"

# 检查任务是否存在
$TaskExists = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($TaskExists) {
    Write-Host "⚙️ 正在停止并移除后台服务计划任务..."
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
}

# 删除二进制文件目录 (C:\ProgramData\PolarisGateway)
if (Test-Path $InstallDir) {
    Write-Host "🗑️ 正在删除程序文件: $InstallDir"
    Remove-Item -Path $InstallDir -Recurse -Force
}

Write-Host ""
Write-Host "⚠️ 注意: 您的数据库和配置数据仍保留在 $env:USERPROFILE\.polarisagi-hermes 目录中。" -ForegroundColor Yellow
Write-Host "如果您想彻底清理所有数据（这会删除所有配置和账单记录），请手动执行：" -ForegroundColor Yellow
Write-Host "Remove-Item -Path $env:USERPROFILE\.polarisagi-hermes -Recurse -Force" -ForegroundColor Yellow
Write-Host ""
Write-Host "✅ 卸载完成！" -ForegroundColor Green
