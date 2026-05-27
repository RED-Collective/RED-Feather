import { App, Modal, Plugin, Notice, TFile } from "obsidian";
import { execFile } from "child_process";
import * as path from "path";
import * as fs from "fs";
import { promisify } from "util";
import { createHash } from "crypto";

// FIX: Use execFile instead of exec to prevent command injection
const execFilePromise = promisify(execFile);

// --- Modal for signing with public key display ---
class SignModal extends Modal {
  private plugin: RedSignerPlugin;
  private file: TFile | null;
  private publicKey: string | null = null;

  constructor(app: App, plugin: RedSignerPlugin, file: TFile | null) {
    super(app);
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
        await navigator.clipboard.writeText(this.publicKey!);
        new Notice("Public key copied!");
      };
    } else {
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
      await this.plugin.signFile(this.file!);
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
export default class RedSignerPlugin extends Plugin {
  private binaryPath: string = "";
  private pluginDir: string = "";
  private vaultRoot: string = "";
  private statusBarItem: HTMLElement | null = null;

  async onload() {
    this.vaultRoot = (this.app.vault.adapter as any).getBasePath();
    if (!this.vaultRoot) {
      new Notice("❌ This plugin only works on desktop Obsidian.");
      return;
    }

    // DYNAMIC FOLDER RESOLUTION
    const manifestDir = this.manifest.dir || "";
    this.pluginDir = path.join(this.vaultRoot, manifestDir);

    let binaryName: string;
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
      } catch (err) {
        console.warn(`Could not set executable permission: ${err}`);
      }
    }

    if (!fs.existsSync(this.binaryPath)) {
      new Notice(`❌ Signer binary missing at ${this.binaryPath}`, 0);
      console.error(`Missing: ${this.binaryPath}`);
    }

    // --- Status Bar Indicator ---
    this.statusBarItem = this.addStatusBarItem();
    this.statusBarItem.addClass("red-signer-status");
    this.updateStatusForActiveFile();

    // --- Events to refresh status ---
    this.registerEvent(
      this.app.vault.on("modify", (file) => {
        const activeFile = this.app.workspace.getActiveFile();
        if (activeFile && file === activeFile) {
          this.updateStatusForActiveFile();
        }
      }),
    );
    this.registerEvent(
      this.app.workspace.on("active-leaf-change", () => {
        this.updateStatusForActiveFile();
      }),
    );

    // --- Ribbon icon ---
    this.addRibbonIcon(
      "signature",
      "Red Signer: Sign current note",
      async () => {
        const file = this.app.workspace.getActiveFile();
        if (file && file.extension === "md") {
          new SignModal(this.app, this, file).open();
        } else {
          new Notice("Please open a markdown note first.");
        }
      },
    );

    // --- Editor menu (direct sign) ---
    this.registerEvent(
      this.app.workspace.on("editor-menu", (menu, _editor, view) => {
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
      }),
    );

    // --- Commands ---
    this.addCommand({
      id: "sign-current-note",
      name: "Sign current note",
      checkCallback: (checking: boolean) => {
        const file = this.app.workspace.getActiveFile();
        if (file && file.extension === "md") {
          if (!checking) this.signFile(file);
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

  private async updateStatusForActiveFile() {
    if (!this.statusBarItem) return;
    const file = this.app.workspace.getActiveFile();
    if (!file || file.extension !== "md") {
      this.statusBarItem.setText("");
      return;
    }

    const manifestPath = path.join(this.vaultRoot, "manifest.json");
    const mapKey = file.path;

    let manifest: any = null;
    try {
      const data = await fs.promises.readFile(manifestPath, "utf8");
      manifest = JSON.parse(data);
    } catch (err: any) {
      if (err.code !== "ENOENT") {
        new Notice("⚠️ Manifest Corrupted or Unreadable");
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
    const hash = createHash("sha256")
      .update(Buffer.from(content))
      .digest("hex");

    if (hash === entry.file_hash) {
      this.statusBarItem.setText("✓ Signed");
      this.statusBarItem.style.color = "var(--color-green)";
    } else {
      this.showUnsigned();
    }
  }

  private showUnsigned() {
    if (!this.statusBarItem) return;
    this.statusBarItem.setText("Unsigned");
    this.statusBarItem.style.color = "var(--text-muted)";
  }

  async signFile(file: TFile) {
    if (!fs.existsSync(this.binaryPath)) {
      new Notice(`❌ Signer binary missing at ${this.binaryPath}`);
      return;
    }

    const fullPath = (this.app.vault.adapter as any).getFullPath(file.path);
    if (!fullPath) {
      new Notice(`❌ Cannot get file path.`);
      return;
    }

    const manifestPath = path.join(this.vaultRoot, "manifest.json");
    console.log(`Using manifest: ${manifestPath}`);

    new Notice(`🔏 Signing ${file.name}...`);

    try {
      // FIX: Use execFilePromise with strict array arguments
      const { stdout, stderr } = await execFilePromise(this.binaryPath, [
        `--manifest=${manifestPath}`,
        fullPath,
      ]);

      if (stderr) console.warn(stderr);
      console.log(stdout);
      new Notice(`✅ Signed: ${file.name}`);
      await this.showPublicKeyIfNew();
      await this.updateStatusForActiveFile();
    } catch (error: any) {
      const errorMsg = error.message + (error.stderr || "");
      if (
        errorMsg.includes("--init") ||
        errorMsg.includes("no manifest.json")
      ) {
        new Notice(`📄 Creating manifest at ${manifestPath}...`);
        await this.initManifest(manifestPath);
        try {
          const { stdout, stderr } = await execFilePromise(this.binaryPath, [
            `--manifest=${manifestPath}`,
            fullPath,
          ]);
          if (stderr) console.warn(stderr);
          console.log(stdout);
          new Notice(`✅ Signed after manifest init: ${file.name}`);
          await this.showPublicKeyIfNew();
          await this.updateStatusForActiveFile();
        } catch (retryError: any) {
          new Notice(`❌ Still failed: ${retryError.message}`);
          console.error(retryError);
        }
      } else {
        new Notice(`❌ Signing failed: ${error.message}`);
        console.error(error);
      }
    }
  }

  private async initManifest(manifestPath: string) {
    try {
      const dummyPath = path.join(this.vaultRoot, "dummy.md");
      const { stderr } = await execFilePromise(this.binaryPath, [
        "--init",
        `--manifest=${manifestPath}`,
        dummyPath,
      ]);
      if (stderr) console.warn(stderr);
      new Notice(`✅ Manifest created at ${manifestPath}`);
    } catch (err: any) {
      new Notice(`❌ Failed to create manifest: ${err.message}`);
      console.error(err);
    }
  }

  private async showPublicKeyIfNew() {
    const pubKey = await this.getPublicKey();
    if (pubKey) {
      const flagPath = path.join(this.pluginDir, ".pubkey_shown");
      if (!fs.existsSync(flagPath)) {
        new Notice(
          `🔑 Your public key:\n${pubKey}\nAdd this to TrustedMaintainers on server.`,
          10000,
        );
        fs.writeFileSync(flagPath, pubKey);
      }
    }
  }

  async getPublicKey(): Promise<string | null> {
    if (!fs.existsSync(this.binaryPath)) return null;
    try {
      const { stdout } = await execFilePromise(this.binaryPath, [
        "--print-pubkey",
      ]);
      return stdout.trim();
    } catch (err) {
      return null;
    }
  }

  private async copyPublicKey() {
    const pubKey = await this.getPublicKey();
    if (pubKey) {
      await navigator.clipboard.writeText(pubKey);
      new Notice(`📋 Public key copied to clipboard.`);
    } else {
      new Notice(`❌ No public key found. Sign a note first to generate one.`);
    }
  }

  onunload() {}
}
