#!/usr/bin/env bash
set -e

REPO_ROOT="/home/coder/Testing/obsidian-red-signer-main"
GO_PROJECT_DIR="$REPO_ROOT/cmd/red-signer"
PLUGIN_DIR="/home/coder/Library/API/.obsidian/plugins/obsidian-red-signer"  # changed to match plugin ID

GOOS=linux
GOARCH=amd64
BINARY_NAME="signer-linux-x64"

echo "🔨 Building Go binary for $GOOS/$GOARCH..."
cd "$GO_PROJECT_DIR"
GOOS=$GOOS GOARCH=$GOARCH go build -o "$BINARY_NAME" main.go
chmod +x "$BINARY_NAME"

echo "📦 Copying to $PLUGIN_DIR"
mkdir -p "$PLUGIN_DIR"
cp "$BINARY_NAME" "$PLUGIN_DIR/"
chmod +x "$PLUGIN_DIR/$BINARY_NAME"   

cd "$REPO_ROOT"
echo "📝 Compiling TypeScript..."
npx tsc --project src/tsconfig.json

cp src/main.js "$PLUGIN_DIR/"
cp manifest.json "$PLUGIN_DIR/" 2>/dev/null || true

echo "✅ Reload complete. Please restart Obsidian or reload plugins."