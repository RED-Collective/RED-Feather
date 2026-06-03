# RED-Feather Plugin Installer (lightweight method) for Windows
$REPO = "StandardCodebase/obsidian-red-signer"
$PLUGIN_NAME = "red-feather"

Write-Host "🔍 Locating your Obsidian vault..." -ForegroundColor Cyan
$VAULT_PATH = Read-Host "Enter the full path to your Obsidian vault (e.g., C:\Users\You\Documents\Vault)"

if (-not (Test-Path $VAULT_PATH))
{
    Write-Host "❌ Vault not found: $VAULT_PATH" -ForegroundColor Red
    exit 1
}

$PLUGIN_DIR = Join-Path $VAULT_PATH ".obsidian\plugins\$PLUGIN_NAME"
New-Item -ItemType Directory -Force -Path $PLUGIN_DIR | Out-Null

Write-Host "📥 Fetching latest release info from GitHub..." -ForegroundColor Cyan
$RELEASE_JSON = (Invoke-WebRequest -Uri "https://api.github.com/repos/$REPO/releases/latest" -UseBasicParsing).Content | ConvertFrom-Json

# Find asset URLs
$MANIFEST_URL = ($RELEASE_JSON.assets | Where-Object { $_.name -eq "manifest.json" }).browser_download_url
$MAINJS_URL = ($RELEASE_JSON.assets | Where-Object { $_.name -eq "main.js" }).browser_download_url
$BIN_NAME = "red-feather-windows-x64.exe"
$BIN_URL = ($RELEASE_JSON.assets | Where-Object { $_.name -eq $BIN_NAME }).browser_download_url

if (-not $MANIFEST_URL -or -not $MAINJS_URL -or -not $BIN_URL)
{
    Write-Host "❌ Failed to find required assets in the latest release." -ForegroundColor Red
    exit 1
}

Write-Host "📄 Downloading manifest.json..."
Invoke-WebRequest -Uri $MANIFEST_URL -OutFile (Join-Path $PLUGIN_DIR "manifest.json") -UseBasicParsing

Write-Host "📄 Downloading main.js..."
Invoke-WebRequest -Uri $MAINJS_URL -OutFile (Join-Path $PLUGIN_DIR "main.js") -UseBasicParsing

Write-Host "⚙️ Downloading $BIN_NAME..."
Invoke-WebRequest -Uri $BIN_URL -OutFile (Join-Path $PLUGIN_DIR $BIN_NAME) -UseBasicParsing

Write-Host "✅ Installation complete!" -ForegroundColor Green
Write-Host "📂 Plugin installed at: $PLUGIN_DIR"
Write-Host ""
Write-Host "Next steps:"
Write-Host "1. Restart Obsidian (or reload community plugins)"
Write-Host "2. Enable 'RED-Feather' in Settings → Community plugins"
