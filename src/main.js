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
const execFilePromise = (0, util_1.promisify)(child_process_1.execFile);
// --- Predefined branch options (real sciences) ---
const subBranchOptions = {
    "Formal Sciences": ["Logic", "Mathematics", "Computer Science", "Statistics", "Information Theory"],
    "Physical Sciences": ["Physics", "Chemistry", "Astronomy", "Earth Sciences", "Materials Science"],
    "Social Sciences": ["Sociology", "Psychology", "Economics", "Political Science", "Anthropology"],
    "Applied Sciences": ["Engineering", "Medicine", "Agriculture", "Architecture", "Technology", "Cryptography"],
    "Arts & Humanities": ["Literature", "History", "Philosophy", "Visual Arts", "Music", "Theatre"],
    "Philosophy & Ethics": ["Epistemology", "Metaphysics", "Ethics", "Aesthetics", "Logic"]
};
// --- Modal for branch selection and signing ---
class SignModal extends obsidian_1.Modal {
    constructor(app, plugin, file) {
        super(app);
        this.publicKey = null;
        this.isAuthor = false;
        this.hasBranch = false;
        this.plugin = plugin;
        this.file = file;
    }
    async onOpen() {
        this.publicKey = await this.plugin.getPublicKey();
        this.isAuthor = await this.plugin.isBranchAuthor();
        this.hasBranch = !!(await this.plugin.getBranch()).branch;
        const { contentEl } = this;
        contentEl.empty();
        contentEl.createEl("h2", { text: "Red Signer" });
        if (!this.file) {
            contentEl.createEl("p", { text: "No markdown file is currently active." });
            return;
        }
        contentEl.createEl("h3", { text: `Current file: ${this.file.name}` });
        contentEl.createEl("h4", { text: "Your Public Key:" });
        const keyContainer = contentEl.createDiv({ cls: "red-signer-key-container" });
        if (this.publicKey) {
            const keyText = keyContainer.createEl("code", { text: this.publicKey });
            keyText.style.cssText = "word-break:break-all; display:block; margin:0.5em 0; padding:0.5em; background:#f0f0f0; border-radius:4px;";
            const copyBtn = keyContainer.createEl("button", { text: "Copy to Clipboard" });
            copyBtn.onclick = async () => {
                await navigator.clipboard.writeText(this.publicKey);
                new obsidian_1.Notice("Public key copied!");
            };
        }
        else {
            keyContainer.createEl("p", { text: "No public key found. Sign a note first to generate one." });
        }
        // Branch selection
        contentEl.createEl("h4", { text: "Knowledge Branch" });
        const branchSelect = contentEl.createEl("select");
        const branches = Object.keys(subBranchOptions);
        const currentBranch = this.plugin.currentBranch;
        // Placeholder option
        const placeholderOption = branchSelect.createEl("option", { text: "-- Select a branch --", value: "" });
        placeholderOption.disabled = true;
        if (!this.hasBranch)
            placeholderOption.selected = true;
        for (const b of branches) {
            const option = branchSelect.createEl("option", { text: b, value: b });
            if (currentBranch === b)
                option.selected = true;
        }
        // Sub‑branch dropdown
        contentEl.createEl("h4", { text: "Sub‑Branch" });
        const subBranchSelect = contentEl.createEl("select");
        const updateSubBranchOptions = () => {
            const selectedBranch = branchSelect.value;
            subBranchSelect.empty();
            // Placeholder for sub-branch
            const subPlaceholder = subBranchSelect.createEl("option", { text: "-- Select a sub-branch --", value: "" });
            subPlaceholder.disabled = true;
            if (!selectedBranch) {
                subPlaceholder.selected = true;
                return;
            }
            const options = subBranchOptions[selectedBranch] || ["General"];
            for (const opt of options) {
                const option = subBranchSelect.createEl("option", { text: opt, value: opt });
                if (this.plugin.currentSubBranch === opt && this.hasBranch)
                    option.selected = true;
            }
            // If no branch exists, force user to pick explicitly: leave placeholder selected
            if (!this.hasBranch && subBranchSelect.options.length > 1) {
                subBranchSelect.options[0].selected = true;
            }
        };
        // Function to validate and enable/disable sign button
        const validateAndEnable = () => {
            const branchValid = branchSelect.value && branchSelect.value !== "";
            const subValid = subBranchSelect.value && subBranchSelect.value !== "";
            signBtn.disabled = !(branchValid && subValid);
        };
        branchSelect.addEventListener("change", () => {
            updateSubBranchOptions();
            validateAndEnable();
        });
        subBranchSelect.addEventListener("change", validateAndEnable);
        updateSubBranchOptions();
        // Disable dropdowns if branch already exists and user is not author
        if (this.hasBranch && !this.isAuthor) {
            branchSelect.disabled = true;
            subBranchSelect.disabled = true;
        }
        else if (!this.hasBranch) {
            branchSelect.disabled = false;
            subBranchSelect.disabled = false;
        }
        else {
            branchSelect.disabled = false;
            subBranchSelect.disabled = false;
        }
        if (!this.isAuthor && this.hasBranch) {
            contentEl.createEl("p", { text: "🔒 Classification locked by original author.", cls: "red-signer-lock" });
        }
        else if (!this.hasBranch) {
            contentEl.createEl("p", { text: "📚 No branch set yet. You must select a branch and sub-branch before signing.", cls: "red-signer-info" });
        }
        const signBtn = contentEl.createEl("button", { text: "✍️ Sign this note", cls: "mod-cta" });
        signBtn.style.marginTop = "1em";
        signBtn.disabled = true; // initially disabled
        signBtn.onclick = async () => {
            signBtn.disabled = true;
            signBtn.setText("Signing...");
            const newBranch = branchSelect.value;
            const newSubBranch = subBranchSelect.value;
            // If no branch exists, initialise the database with chosen branch
            if (!this.hasBranch) {
                await this.plugin.initDatabase(newBranch, newSubBranch);
            }
            else if (this.isAuthor && (newBranch !== currentBranch || newSubBranch !== this.plugin.currentSubBranch)) {
                await this.plugin.updateBranch(newBranch, newSubBranch);
            }
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
// --- Main Plugin Class (Database only) ---
class RedSignerPlugin extends obsidian_1.Plugin {
    constructor() {
        super(...arguments);
        this.binaryPath = "";
        this.pluginDir = "";
        this.vaultRoot = "";
        this.dbPath = "";
        this.statusBarItem = null;
        this.currentBranch = "";
        this.currentSubBranch = "";
        this.statusCheckTimeout = null;
    }
    async onload() {
        // 1. Determine absolute vault root
        const adapter = this.app.vault.adapter;
        this.vaultRoot = adapter.getBasePath ? adapter.getBasePath() : adapter.basePath || "";
        if (!this.vaultRoot) {
            new obsidian_1.Notice("❌ Red Signer only works on desktop Obsidian with a local filesystem.");
            return;
        }
        console.log("[Red Signer] Vault root detected:", this.vaultRoot);
        // 2. Resolve absolute plugin directory (ensuring absolute path for fs operations)
        const manifestDir = this.manifest.dir;
        this.pluginDir = path.join(this.vaultRoot, manifestDir);
        console.log("[Red Signer] Plugin directory resolved to:", this.pluginDir);
        // 3. Set database path (Vault Root -> .red-signer folder -> signer.db)
        this.dbPath = path.join(this.vaultRoot, ".red-signer", "signer.db");
        let binaryName;
        switch (process.platform) {
            case "win32":
                binaryName = "signer-windows-x64.exe";
                break;
            case "darwin":
                binaryName = process.arch === "arm64" ? "signer-macos-arm64" : "signer-macos-x64";
                break;
            case "linux":
                binaryName = process.arch === "arm64" || process.arch === "aarch64" ? "signer-linux-arm64" : "signer-linux-x64";
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
        // Ensure README in key directory
        this.ensureReadme().catch(console.error);
        // Load branch info from database
        await this.loadBranchFromDb();
        // Status bar
        this.statusBarItem = this.addStatusBarItem();
        this.statusBarItem.addClass("red-signer-status");
        this.updateStatusForActiveFile();
        // Event listeners
        this.registerEvent(this.app.vault.on("modify", (file) => {
            const activeFile = this.app.workspace.getActiveFile();
            if (activeFile && file === activeFile)
                this.updateStatusForActiveFile();
        }));
        this.registerEvent(this.app.workspace.on("active-leaf-change", () => {
            this.updateStatusForActiveFile();
        }));
        // Ribbon icon
        this.addRibbonIcon("signature", "Red Signer: Sign current note", async () => {
            const file = this.app.workspace.getActiveFile();
            if (file && file.extension === "md") {
                new SignModal(this.app, this, file).open();
            }
            else {
                new obsidian_1.Notice("Please open a markdown note first.");
            }
        });
        // Editor context menu
        this.registerEvent(this.app.workspace.on("editor-menu", (menu, _editor, view) => {
            const file = view.file;
            if (file && file.extension === "md") {
                menu.addItem((item) => {
                    item.setTitle("Sign this note directly")
                        .setIcon("checkmark")
                        .onClick(async () => { await this.signFile(file); });
                });
            }
        }));
        // Commands
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
    async ensureReadme() {
        const homedir = require('os').homedir();
        const redNetworkDir = path.join(homedir, ".red-network");
        const readmePath = path.join(redNetworkDir, "README.md");
        if (!fs.existsSync(readmePath)) {
            if (!fs.existsSync(redNetworkDir)) {
                fs.mkdirSync(redNetworkDir, { recursive: true, mode: 0o700 });
            }
            const content = `# RED Network Identity
  
This directory contains your private Ed25519 key (maintainer.key) used by the Red Signer Obsidian plugin.

**⚠️ WARNING: Do not delete this file unless you intend to lose your contributor identity.**

- If you delete maintainer.key, you will no longer be able to sign notes as the original author of any vault.
- You will lose the ability to modify branch classification for vaults you authored.
- A new key will be generated automatically, but it will be a different identity.

To back up your identity, copy the file maintainer.key to a secure location (e.g., an encrypted USB drive).

For more information, see https://github.com/RED-Collective/red-engine
        `;
            fs.writeFileSync(readmePath, content, { mode: 0o644 });
            console.log("Created README in ~/.red-network");
        }
    }
    async loadBranchFromDb() {
        if (!fs.existsSync(this.binaryPath))
            return;
        try {
            const { stdout } = await execFilePromise(this.binaryPath, [
                "--db", this.dbPath,
                "--get-branch"
            ]);
            const data = JSON.parse(stdout);
            this.currentBranch = data.branch || "";
            this.currentSubBranch = data.sub_branch || "";
        }
        catch (err) {
            // Database might not exist yet; ignore
            this.currentBranch = "";
            this.currentSubBranch = "";
        }
    }
    async getBranch() {
        if (!fs.existsSync(this.binaryPath))
            return { branch: "", sub_branch: "", branch_author: "" };
        try {
            const { stdout } = await execFilePromise(this.binaryPath, [
                "--db", this.dbPath,
                "--get-branch"
            ]);
            return JSON.parse(stdout);
        }
        catch (err) {
            return { branch: "", sub_branch: "", branch_author: "" };
        }
    }
    async isBranchAuthor() {
        const branchInfo = await this.getBranch();
        const authorPubKey = branchInfo.branch_author;
        if (!authorPubKey)
            return true;
        const currentPubKey = await this.getPublicKey();
        return currentPubKey === authorPubKey;
    }
    async initDatabase(branch, subBranch) {
        try {
            await execFilePromise(this.binaryPath, [
                "--db", this.dbPath,
                "--init",
                "--branch", branch,
                "--sub-branch", subBranch
            ]);
            this.currentBranch = branch;
            this.currentSubBranch = subBranch;
            new obsidian_1.Notice(`✅ Database initialised with branch: ${branch}/${subBranch}`);
            return true;
        }
        catch (err) {
            new obsidian_1.Notice(`❌ Failed to initialise database: ${err.message}`);
            return false;
        }
    }
    async updateBranch(branch, subBranch) {
        const isAuthor = await this.isBranchAuthor();
        if (!isAuthor) {
            new obsidian_1.Notice("❌ Only the original author can change the branch classification.");
            return false;
        }
        const oldBranch = this.currentBranch || "(none)";
        const oldSub = this.currentSubBranch || "(none)";
        const confirmMsg = `Change classification from\nBranch: ${oldBranch}\nSub‑branch: ${oldSub}\nto\nBranch: ${branch}\nSub‑branch: ${subBranch} ?\n\nThis will affect the entire vault.`;
        if (!confirm(confirmMsg))
            return false;
        try {
            const args = ["--db", this.dbPath, "--set-branch", "--branch", branch];
            if (subBranch)
                args.push("--sub-branch", subBranch);
            await execFilePromise(this.binaryPath, args);
            this.currentBranch = branch;
            this.currentSubBranch = subBranch;
            new obsidian_1.Notice("✅ Branch classification updated.");
            return true;
        }
        catch (err) {
            new obsidian_1.Notice(`❌ Failed to update branch: ${err.message}`);
            return false;
        }
    }
    async checkFileStatus(filePath) {
        if (!fs.existsSync(this.binaryPath))
            return "unsigned";
        try {
            const { stdout } = await execFilePromise(this.binaryPath, [
                "--db", this.dbPath,
                "--check-status", filePath
            ]);
            const status = stdout.trim();
            if (status === "signed" || status === "unsigned" || status === "modified") {
                return status;
            }
            return "unsigned";
        }
        catch (err) {
            return "unsigned";
        }
    }
    async updateStatusForActiveFile() {
        if (!this.statusBarItem)
            return;
        const file = this.app.workspace.getActiveFile();
        if (!file || file.extension !== "md") {
            this.statusBarItem.setText("");
            return;
        }
        if (this.statusCheckTimeout)
            clearTimeout(this.statusCheckTimeout);
        this.statusCheckTimeout = setTimeout(async () => {
            var _a, _b;
            const fullPath = this.app.vault.adapter.getFullPath(file.path);
            if (!fullPath) {
                this.showUnsigned();
                return;
            }
            const status = await this.checkFileStatus(fullPath);
            if (status === "signed") {
                (_a = this.statusBarItem) === null || _a === void 0 ? void 0 : _a.setText("✓ Signed");
                if (this.statusBarItem)
                    this.statusBarItem.style.color = "var(--color-green)";
            }
            else if (status === "modified") {
                (_b = this.statusBarItem) === null || _b === void 0 ? void 0 : _b.setText("⚠ Modified");
                if (this.statusBarItem)
                    this.statusBarItem.style.color = "var(--color-orange)";
            }
            else {
                this.showUnsigned();
            }
        }, 100);
    }
    showUnsigned() {
        if (!this.statusBarItem)
            return;
        this.statusBarItem.setText("Unsigned");
        if (this.statusBarItem) {
            this.statusBarItem.style.color = "var(--text-muted)";
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
    async signFile(file) {
        // Enforce branch existence before signing
        if (!this.currentBranch) {
            new obsidian_1.Notice("⚠️ No branch set. Please select a branch and sub-branch before signing.");
            const activeFile = this.app.workspace.getActiveFile();
            if (activeFile && activeFile.extension === "md") {
                new SignModal(this.app, this, activeFile).open();
            }
            else {
                new obsidian_1.Notice("❌ No markdown file active. Open a note and try again.");
            }
            return;
        }
        if (!fs.existsSync(this.binaryPath)) {
            new obsidian_1.Notice(`❌ Signer binary missing at ${this.binaryPath}`);
            return;
        }
        const fullPath = this.app.vault.adapter.getFullPath(file.path);
        if (!fullPath) {
            new obsidian_1.Notice("❌ Cannot get file path.");
            return;
        }
        new obsidian_1.Notice(`🔏 Signing ${file.name}...`);
        try {
            await execFilePromise(this.binaryPath, [
                "--db", this.dbPath,
                fullPath
            ]);
            new obsidian_1.Notice(`✅ Signed: ${file.name}`);
            await this.loadBranchFromDb();
            await this.updateStatusForActiveFile();
            await this.showPublicKeyIfNew();
        }
        catch (error) {
            new obsidian_1.Notice(`❌ Signing failed: ${error.message}`);
            console.error(error);
        }
    }
    async getPublicKey() {
        if (!fs.existsSync(this.binaryPath))
            return null;
        try {
            const { stdout } = await execFilePromise(this.binaryPath, ["--print-pubkey"]);
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
            new obsidian_1.Notice("📋 Public key copied to clipboard.");
        }
        else {
            new obsidian_1.Notice("❌ No public key found. Sign a note first to generate one.");
        }
    }
    onunload() { }
}
exports.default = RedSignerPlugin;
