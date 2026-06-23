# HowlerOps Installation Guide

HowlerOps ships a universal installation script that detects your platform and
installs the right artifact: the **HowlerOps desktop app** on macOS (Apple
Silicon) and the **`howlerops` CLI** on Linux.

## Quick Install

The fastest way to install HowlerOps is the installation script:

```bash
curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh
```

This will:
- Detect your OS and architecture
- Download the appropriate release artifact
- Verify SHA256 checksums for security
- Install to `~/.local/bin` (CLI) or `/Applications` (macOS app), falling back as needed
- Update your PATH if necessary

## Supported Platforms

| Operating System            | What gets installed   | Status                         |
|-----------------------------|-----------------------|--------------------------------|
| macOS — Apple Silicon (arm64) | `HowlerOps.app` desktop app | ✅ Published |
| Linux — x86_64 (amd64)      | `howlerops` CLI       | ✅ Published                   |
| Linux — arm64               | `howlerops` CLI       | ✅ Published                   |
| macOS — Intel (amd64)       | —                     | ⏸️ Paused (build matrix disabled) |
| Windows — native            | —                     | ❌ Not published — use WSL or build from source |

> **Windows:** native binaries are not published. Run HowlerOps under **WSL**
> (which uses the Linux CLI) or **build from source**. The build excludes the
> DuckDB federation engine on platforms without DuckDB bindings.
>
> **Intel macOS:** temporarily paused because the `macos-13` runners queue for a
> long time and stall releases. Re-add the `darwin/amd64` matrix entries in
> `.github/workflows/release.yml` to restore Intel builds.

## Installation Options

### Install a Specific Version

```bash
curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh -s -- --version v0.17.0
```

### Dry Run (Preview Without Installing)

```bash
curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh -s -- --dry-run
```

### Verbose Output

```bash
curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh -s -- --verbose
```

### Force Reinstall

```bash
curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh -s -- --force
```

## Manual Installation

If you prefer to install manually, download the matching artifact from the
[GitHub Releases](https://github.com/howlerops/howlerops/releases) page.

### macOS desktop app (Apple Silicon)

```bash
# Download the app bundle archive
curl -LO https://github.com/howlerops/howlerops/releases/latest/download/howlerops-darwin-arm64.tar.gz

# Extract and move into place
tar -xzf howlerops-darwin-arm64.tar.gz
mv HowlerOps.app /Applications/
```

The app is not notarized for distribution outside the App Store yet, so on first
launch you may need to right-click → **Open** (or allow it under **System
Settings → Privacy & Security**).

### Linux CLI

```bash
# Pick the archive for your architecture
curl -LO https://github.com/howlerops/howlerops/releases/latest/download/howlerops-cli-linux-amd64.tar.gz
# or: howlerops-cli-linux-arm64.tar.gz

tar -xzf howlerops-cli-linux-amd64.tar.gz
chmod +x howlerops
mv howlerops ~/.local/bin/   # or: sudo mv howlerops /usr/local/bin/
```

### Verify the checksum (recommended)

Each artifact ships an accompanying `.sha256` file, and releases include a
combined `checksums.txt`:

```bash
# Verify a single archive
curl -LO https://github.com/howlerops/howlerops/releases/latest/download/howlerops-cli-linux-amd64.tar.gz.sha256
sha256sum -c howlerops-cli-linux-amd64.tar.gz.sha256   # Linux
shasum -a 256 -c howlerops-cli-linux-amd64.tar.gz.sha256  # macOS
```

## Updating PATH

If the install directory is not in your PATH, add it to your shell profile:

```bash
# Bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

# Zsh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc

# Fish
set -Ua fish_user_paths $HOME/.local/bin
```

## Verifying Installation

```bash
# CLI
howlerops --version
howlerops --help
```

## Build from Source

Building from source works on any platform with a Go toolchain (and a C
compiler, since SQLite uses CGO). See the
[Development section of the README](../README.md#development) for the full setup.

```bash
git clone https://github.com/howlerops/howlerops.git
cd howlerops
make deps
make build      # desktop app (Wails v3)
```

## Troubleshooting

### Platform Not Supported / Download 404

`install.sh` only finds an artifact for the published platforms above. If you
see "Unsupported platform" or a download 404:

- On **Windows**, run the script inside **WSL**, or build from source.
- On **Intel macOS**, builds are paused — build from source or use a release that
  still published `darwin-amd64`.

### Permission Denied

```bash
chmod +x ~/.local/bin/howlerops
```

### Command Not Found

```bash
which howlerops    # confirm it is on PATH; add the install dir if not
```

### Checksum Verification Failed

Re-download (the transfer may have been corrupted) and confirm you are pulling
from the official GitHub releases. Report the issue if it persists.

## Security

The installation script:

- Downloads artifacts only from official GitHub releases over HTTPS
- Verifies SHA256 checksums before installing
- Requires no `sudo` unless writing to a system directory

For maximum safety, review the script before piping it to a shell, pin to a
specific `--version` in automation, and verify checksums manually.

## Getting Help

- **Issues:** https://github.com/howlerops/howlerops/issues
- **Releases:** https://github.com/howlerops/howlerops/releases

## License

HowlerOps is released under the MIT License. See [LICENSE](../LICENSE) for details.
