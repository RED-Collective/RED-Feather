"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
const obsidian_1 = require("obsidian");
const child_process_1 = require("child_process");
const path = __importStar(require("path"));
const fs = __importStar(require("fs"));
const util_1 = require("util");
const crypto_1 = require("crypto");
// FIX: Use execFile instead of exec to prevent command injection
const execFilePromise = (0, util_1.promisify)(child_process_1.execFile);
// --- Modal for signing with public key display ---
class SignModal extends obsidian_1.Modal {
    constructor(app, plugin, file) {
        super(app);
        this.publicKey = null;
        this.plugin = plugin;
        this.file = file;
    }
    async onOpen() {
        this.publicKey = await this.plugin.getPublicKey();
        const { contentEl } = this;
        contentEl.empty();
        contentEl.createEl("h2", { text: "Red Signer" });
        if (!this.file) {
            contentEl.createEl("p", {
                text: "No markdown file is currently active.",
            });
            return;
        }
        contentEl.createEl("h3", { text: `Current file: ${this.file.name}` });
        contentEl.createEl("h4", { text: "Your Public Key:" });
        const keyContainer = contentEl.createDiv({
            cls: "red-signer-key-container",
        });
        if (this.publicKey) {
            const keyText = keyContainer.createEl("code", { text: this.publicKey });
            keyText.style.wordBreak = "break-all";
            keyText.style.display = "block";
            keyText.style.margin = "0.5em 0";
            keyText.style.padding = "0.5em";
            keyText.style.backgroundColor = "#f0f0f0";
            keyText.style.borderRadius = "4px";
            const copyBtn = keyContainer.createEl("button", {
                text: "Copy to Clipboard",
            });
            copyBtn.onclick = async () => {
                await navigator.clipboard.writeText(this.publicKey);
                new obsidian_1.Notice("Public key copied!");
            };
        }
        else {
            keyContainer.createEl("p", {
                text: "No public key found. Sign a note first to generate one.",
            });
        }
        const signBtn = contentEl.createEl("button", {
            text: "✍️ Sign this note",
            cls: "mod-cta",
        });
        signBtn.style.marginTop = "1em";
        signBtn.onclick = async () => {
            signBtn.disabled = true;
            signBtn.setText("Signing...");
            await this.plugin.signFile(this.file);
            this.close();
        };
        const closeBtn = contentEl.createEl("button", { text: "Close" });
        closeBtn.style.marginLeft = "0.5em";
        closeBtn.onclick = () => this.close();
    }
    onClose() {
        const { contentEl } = this;
        contentEl.empty();
    }
}
// --- Main Plugin Class ---
class RedSignerPlugin extends obsidian_1.Plugin {
    constructor() {
        super(...arguments);
        this.binaryPath = "";
        this.pluginDir = "";
        this.vaultRoot = "";
        this.statusBarItem = null;
    }
    async onload() {
        this.vaultRoot = this.app.vault.adapter.getBasePath();
        if (!this.vaultRoot) {
            new obsidian_1.Notice("❌ This plugin only works on desktop Obsidian.");
            return;
        }
        // DYNAMIC FOLDER RESOLUTION
        const manifestDir = this.manifest.dir || "";
        this.pluginDir = path.join(this.vaultRoot, manifestDir);
        let binaryName;
        switch (process.platform) {
            case "win32":
                binaryName = "signer-windows-x64.exe";
                break;
            case "darwin":
                binaryName = "signer-macos-x64";
                break;
            case "linux":
                binaryName = "signer-linux-x64";
                break;
            default:
                binaryName = "signer";
        }
        this.binaryPath = path.join(this.pluginDir, binaryName);
        if (process.platform !== "win32" && fs.existsSync(this.binaryPath)) {
            try {
                const stats = fs.statSync(this.binaryPath);
                if (!(stats.mode & 0o111)) {
                    fs.chmodSync(this.binaryPath, 0o755);
                    console.log(`Set executable permission on ${this.binaryPath}`);
                }
            }
            catch (err) {
                console.warn(`Could not set executable permission: ${err}`);
            }
        }
        if (!fs.existsSync(this.binaryPath)) {
            new obsidian_1.Notice(`❌ Signer binary missing at ${this.binaryPath}`, 0);
            console.error(`Missing: ${this.binaryPath}`);
        }
        // --- Status Bar Indicator ---
        this.statusBarItem = this.addStatusBarItem();
        this.statusBarItem.addClass("red-signer-status");
        this.updateStatusForActiveFile();
        // --- Events to refresh status ---
        this.registerEvent(this.app.vault.on("modify", (file) => {
            const activeFile = this.app.workspace.getActiveFile();
            if (activeFile && file === activeFile) {
                this.updateStatusForActiveFile();
            }
        }));
        this.registerEvent(this.app.workspace.on("active-leaf-change", () => {
            this.updateStatusForActiveFile();
        }));
        // --- Ribbon icon ---
        this.addRibbonIcon("signature", "Red Signer: Sign current note", async () => {
            const file = this.app.workspace.getActiveFile();
            if (file && file.extension === "md") {
                new SignModal(this.app, this, file).open();
            }
            else {
                new obsidian_1.Notice("Please open a markdown note first.");
            }
        });
        // --- Editor menu (direct sign) ---
        this.registerEvent(this.app.workspace.on("editor-menu", (menu, _editor, view) => {
            const file = view.file;
            if (file && file.extension === "md") {
                menu.addItem((item) => {
                    item
                        .setTitle("Sign this note directly")
                        .setIcon("checkmark")
                        .onClick(async () => {
                        await this.signFile(file);
                    });
                });
            }
        }));
        // --- Commands ---
        this.addCommand({
            id: "sign-current-note",
            name: "Sign current note",
            checkCallback: (checking) => {
                const file = this.app.workspace.getActiveFile();
                if (file && file.extension === "md") {
                    if (!checking)
                        this.signFile(file);
                    return true;
                }
                return false;
            },
        });
        this.addCommand({
            id: "copy-public-key",
            name: "Copy public key to clipboard",
            callback: () => this.copyPublicKey(),
        });
    }
    async updateStatusForActiveFile() {
        if (!this.statusBarItem)
            return;
        const file = this.app.workspace.getActiveFile();
        if (!file || file.extension !== "md") {
            this.statusBarItem.setText("");
            return;
        }
        const manifestPath = path.join(this.vaultRoot, "manifest.json");
        const mapKey = file.path;
        let manifest = null;
        try {
            const data = await fs.promises.readFile(manifestPath, "utf8");
            manifest = JSON.parse(data);
        }
        catch (err) {
            if (err.code !== "ENOENT") {
                new obsidian_1.Notice("⚠️ Manifest Corrupted or Unreadable");
                console.error("Manifest read error:", err);
            }
            this.showUnsigned();
            return;
        }
        const entry = manifest[mapKey];
        if (!entry) {
            this.showUnsigned();
            return;
        }
        const content = await this.app.vault.readBinary(file);
        const hash = (0, crypto_1.createHash)("sha256")
            .update(Buffer.from(content))
            .digest("hex");
        if (hash === entry.file_hash) {
            this.statusBarItem.setText("✓ Signed");
            this.statusBarItem.style.color = "var(--color-green)";
        }
        else {
            this.showUnsigned();
        }
    }
    showUnsigned() {
        if (!this.statusBarItem)
            return;
        this.statusBarItem.setText("Unsigned");
        this.statusBarItem.style.color = "var(--text-muted)";
    }
    async signFile(file) {
        if (!fs.existsSync(this.binaryPath)) {
            new obsidian_1.Notice(`❌ Signer binary missing at ${this.binaryPath}`);
            return;
        }
        const fullPath = this.app.vault.adapter.getFullPath(file.path);
        if (!fullPath) {
            new obsidian_1.Notice(`❌ Cannot get file path.`);
            return;
        }
        const manifestPath = path.join(this.vaultRoot, "manifest.json");
        console.log(`Using manifest: ${manifestPath}`);
        new obsidian_1.Notice(`🔏 Signing ${file.name}...`);
        try {
            // FIX: Use execFilePromise with strict array arguments
            const { stdout, stderr } = await execFilePromise(this.binaryPath, [
                `--manifest=${manifestPath}`,
                fullPath,
            ]);
            if (stderr)
                console.warn(stderr);
            console.log(stdout);
            new obsidian_1.Notice(`✅ Signed: ${file.name}`);
            await this.showPublicKeyIfNew();
            await this.updateStatusForActiveFile();
        }
        catch (error) {
            const errorMsg = error.message + (error.stderr || "");
            if (errorMsg.includes("--init") ||
                errorMsg.includes("no manifest.json")) {
                new obsidian_1.Notice(`📄 Creating manifest at ${manifestPath}...`);
                await this.initManifest(manifestPath);
                try {
                    const { stdout, stderr } = await execFilePromise(this.binaryPath, [
                        `--manifest=${manifestPath}`,
                        fullPath,
                    ]);
                    if (stderr)
                        console.warn(stderr);
                    console.log(stdout);
                    new obsidian_1.Notice(`✅ Signed after manifest init: ${file.name}`);
                    await this.showPublicKeyIfNew();
                    await this.updateStatusForActiveFile();
                }
                catch (retryError) {
                    new obsidian_1.Notice(`❌ Still failed: ${retryError.message}`);
                    console.error(retryError);
                }
            }
            else {
                new obsidian_1.Notice(`❌ Signing failed: ${error.message}`);
                console.error(error);
            }
        }
    }
    async initManifest(manifestPath) {
        try {
            const dummyPath = path.join(this.vaultRoot, "dummy.md");
            const { stderr } = await execFilePromise(this.binaryPath, [
                "--init",
                `--manifest=${manifestPath}`,
                dummyPath,
            ]);
            if (stderr)
                console.warn(stderr);
            new obsidian_1.Notice(`✅ Manifest created at ${manifestPath}`);
        }
        catch (err) {
            new obsidian_1.Notice(`❌ Failed to create manifest: ${err.message}`);
            console.error(err);
        }
    }
    async showPublicKeyIfNew() {
        const pubKey = await this.getPublicKey();
        if (pubKey) {
            const flagPath = path.join(this.pluginDir, ".pubkey_shown");
            if (!fs.existsSync(flagPath)) {
                new obsidian_1.Notice(`🔑 Your public key:\n${pubKey}\nAdd this to TrustedMaintainers on server.`, 10000);
                fs.writeFileSync(flagPath, pubKey);
            }
        }
    }
    async getPublicKey() {
        if (!fs.existsSync(this.binaryPath))
            return null;
        try {
            const { stdout } = await execFilePromise(this.binaryPath, [
                "--print-pubkey",
            ]);
            return stdout.trim();
        }
        catch (err) {
            return null;
        }
    }
    async copyPublicKey() {
        const pubKey = await this.getPublicKey();
        if (pubKey) {
            await navigator.clipboard.writeText(pubKey);
            new obsidian_1.Notice(`📋 Public key copied to clipboard.`);
        }
        else {
            new obsidian_1.Notice(`❌ No public key found. Sign a note first to generate one.`);
        }
    }
    onunload() { }
}
exports.default = RedSignerPlugin;
