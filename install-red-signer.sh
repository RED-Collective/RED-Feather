#!/bin/bash
# Red Signer Plugin Installer (lightweight method)

REPO="StandardCodebase/obsidian-red-signer"
PLUGIN_NAME="obsidian-red-signer"

echo "🔍 Locating your Obsidian vault..."
read -p "Enter the full path to your Obsidian vault (e.g., /home/user/Documents/Vault): " VAULT_PATH

if [ ! -d "$VAULT_PATH" ]; then
    echo "❌ Vault not found: $VAULT_PATH"
    exit 1
fi

PLUGIN_DIR="$VAULT_PATH/.obsidian/plugins/$PLUGIN_NAME"
mkdir -p "$PLUGIN_DIR"

echo "📥 Fetching latest release info from GitHub..."
RELEASE_JSON=$(curl -s "https://api.github.com/repos/$REPO/releases/latest")

# Extract asset URLs
MANIFEST_URL=$(echo "$RELEASE_JSON" | grep -o 'https://.*manifest.json' | head -1)
MAINJS_URL=$(echo "$RELEASE_JSON" | grep -o 'https://.*main.js' | head -1)

# Determine correct binary
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
    linux)
        case "$ARCH" in
            x86_64) BIN_NAME="signer-linux-x64" ;;
            aarch64) BIN_NAME="signer-linux-arm64" ;;
            *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
        esac
        ;;
    darwin)
        case "$ARCH" in
            x86_64) BIN_NAME="signer-macos-x64" ;;
            arm64) BIN_NAME="signer-macos-arm64" ;;
            *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
        esac
        ;;
    *)
        echo "❌ Unsupported OS: $OS"
        exit 1
        ;;
esac

BIN_URL=$(echo "$RELEASE_JSON" | grep -o "https://.*$BIN_NAME" | head -1)

if [ -z "$MANIFEST_URL" ] || [ -z "$MAINJS_URL" ] || [ -z "$BIN_URL" ]; then
    echo "❌ Failed to find required assets in the latest release."
    exit 1
fi

echo "📄 Downloading manifest.json..."
curl -L -o "$PLUGIN_DIR/manifest.json" "$MANIFEST_URL"

echo "📄 Downloading main.js..."
curl -L -o "$PLUGIN_DIR/main.js" "$MAINJS_URL"

echo "⚙️ Downloading $BIN_NAME..."
curl -L -o "$PLUGIN_DIR/$BIN_NAME" "$BIN_URL"

# Make binary executable (Unix only)
chmod +x "$PLUGIN_DIR/$BIN_NAME"

echo "✅ Installation complete!"
echo "📂 Plugin installed at: $PLUGIN_DIR"
echo ""
echo "Next steps:"
echo "1. Restart Obsidian (or reload community plugins)"
echo "2. Enable 'Red Signer' in Settings → Community plugins"
