#!/bin/bash
set -e

# ============================================================================
# Kiro Proxy Build Script
# Builds kiro-go (Go backend) + kiro-launcher (Wails GUI) for all platforms
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GO_DIR="$SCRIPT_DIR/kiro-go"
WAILS_DIR="$SCRIPT_DIR/kiro-launcher"
OUTPUT_DIR="$SCRIPT_DIR/release"
WAILS="${WAILS:-$(which wails 2>/dev/null || echo "$HOME/go/bin/wails")}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

usage() {
    echo "Usage: $0 [target...]"
    echo ""
    echo "Targets:"
    echo "  mac          Build for macOS (arm64 + x86_64 universal binary)"
    echo "  mac-arm64    Build for macOS arm64 only"
    echo "  mac-x64      Build for macOS x86_64 only"
    echo "  win          Build for Windows x86_64"
    echo "  linux        Build kiro-go for Linux amd64 (server deployment)"
    echo "  server       Build kiro-go for Linux + deploy to server"
    echo "  all          Build for all platforms (mac-arm64 + mac-x64 + win + linux)"
    echo "  clean        Clean build artifacts"
    echo ""
    echo "If no target is specified, builds for current platform."
}

clean() {
    info "Cleaning build artifacts..."
    rm -rf "$OUTPUT_DIR"
    rm -rf "$WAILS_DIR/build"
    rm -f "$WAILS_DIR/sidecar/kiro-go" "$WAILS_DIR/sidecar/kiro-go.exe"
    info "Clean complete."
}

# Build kiro-go for a specific GOOS/GOARCH
# Sets GO_BIN to the output binary path
GO_BIN=""
build_go() {
    local goos="$1"
    local goarch="$2"
    local label="$3"

    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"

    info "Building kiro-go for $label (GOOS=$goos GOARCH=$goarch)..."
    (cd "$GO_DIR" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -o "/tmp/kiro-go-${goos}-${goarch}${ext}" .)

    GO_BIN="/tmp/kiro-go-${goos}-${goarch}${ext}"
    if [ ! -f "$GO_BIN" ]; then
        error "kiro-go binary not found at $GO_BIN"
    fi
    info "kiro-go binary: $GO_BIN ($(du -h "$GO_BIN" | cut -f1))"
}

# Stage kiro-go binary into sidecar/ for Wails embedding
stage_sidecar() {
    local go_binary="$1"
    local goos="$2"

    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"

    local sidecar_dir="$WAILS_DIR/sidecar"
    mkdir -p "$sidecar_dir"
    rm -f "$sidecar_dir/kiro-go" "$sidecar_dir/kiro-go.exe"

    cp "$go_binary" "$sidecar_dir/kiro-go${ext}"
    info "Staged kiro-go into sidecar/ for embedding"
}

# Build kiro-launcher (Wails) with embedded kiro-go
build_wails() {
    local goos="$1"
    local goarch="$2"
    local label="$3"
    local go_binary="$4"

    stage_sidecar "$go_binary" "$goos"

    info "Building kiro-launcher for $label (GOOS=$goos GOARCH=$goarch)..."

    local out_dir="$OUTPUT_DIR/$label"
    mkdir -p "$out_dir"

    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"

    if [ "$goos" == "darwin" ]; then
        (cd "$WAILS_DIR" && GOOS=$goos GOARCH=$goarch $WAILS build -clean -o "kiro-launcher")

        if [ -d "$WAILS_DIR/build/bin/kiro-launcher.app" ]; then
            cp -R "$WAILS_DIR/build/bin/kiro-launcher.app" "$out_dir/"
            info "macOS app: $out_dir/kiro-launcher.app (单文件，kiro-go 已内嵌)"
        else
            cp "$WAILS_DIR/build/bin/kiro-launcher" "$out_dir/"
            info "macOS binary: $out_dir/kiro-launcher (单文件，kiro-go 已内嵌)"
        fi
    else
        (cd "$WAILS_DIR" && \
            CGO_ENABLED=1 \
            GOOS=$goos GOARCH=$goarch \
            CC=x86_64-w64-mingw32-gcc \
            CXX=x86_64-w64-mingw32-g++ \
            $WAILS build -clean -o "kiro-launcher${ext}" -skipbindings)

        cp "$WAILS_DIR/build/bin/kiro-launcher${ext}" "$out_dir/"
        info "Windows binary: $out_dir/kiro-launcher${ext} (单文件，kiro-go 已内嵌)"
    fi

    # Clean sidecar after build
    rm -f "$WAILS_DIR/sidecar/kiro-go" "$WAILS_DIR/sidecar/kiro-go.exe"
}

# ── macOS arm64 ──
build_mac_arm64() {
    build_go "darwin" "arm64" "macOS arm64"
    build_wails "darwin" "arm64" "mac-arm64" "$GO_BIN"
}

# ── macOS x86_64 ──
build_mac_x64() {
    build_go "darwin" "amd64" "macOS x86_64"
    build_wails "darwin" "amd64" "mac-x64" "$GO_BIN"
}

# ── macOS universal ──
build_mac_universal() {
    info "Building macOS universal binary..."

    # Build kiro-go for both architectures
    build_go "darwin" "arm64" "macOS arm64"
    local arm64_go="$GO_BIN"
    build_go "darwin" "amd64" "macOS x86_64"
    local x64_go="$GO_BIN"

    # Create universal kiro-go binary
    local universal_go="/tmp/kiro-go-universal"
    lipo -create "$arm64_go" "$x64_go" -output "$universal_go"
    info "Created universal kiro-go binary"

    # Build Wails for both architectures, then lipo merge
    stage_sidecar "$universal_go" "darwin"

    # -- arm64 --
    info "Building kiro-launcher for macOS arm64..."
    (cd "$WAILS_DIR" && GOOS=darwin GOARCH=arm64 $WAILS build -clean -o "kiro-launcher")

    local arm64_app="$WAILS_DIR/build/bin/kiro-launcher.app"
    local arm64_launcher=""
    if [ -d "$arm64_app" ]; then
        arm64_launcher="$arm64_app/Contents/MacOS/kiro-launcher"
    else
        arm64_launcher="$WAILS_DIR/build/bin/kiro-launcher"
    fi
    cp "$arm64_launcher" "/tmp/kiro-launcher-arm64"

    # -- x86_64 --
    info "Building kiro-launcher for macOS x86_64..."
    (cd "$WAILS_DIR" && GOOS=darwin GOARCH=amd64 $WAILS build -clean -o "kiro-launcher")

    local x64_app="$WAILS_DIR/build/bin/kiro-launcher.app"
    local x64_launcher=""
    if [ -d "$x64_app" ]; then
        x64_launcher="$x64_app/Contents/MacOS/kiro-launcher"
    else
        x64_launcher="$WAILS_DIR/build/bin/kiro-launcher"
    fi

    # Merge into universal binary
    info "Creating universal kiro-launcher binary with lipo..."
    lipo -create "/tmp/kiro-launcher-arm64" "$x64_launcher" -output "$x64_launcher"
    rm -f "/tmp/kiro-launcher-arm64"

    # Copy to output
    local out_dir="$OUTPUT_DIR/mac-universal"
    mkdir -p "$out_dir"
    if [ -d "$x64_app" ]; then
        cp -R "$x64_app" "$out_dir/kiro-launcher.app"
        info "macOS universal app: $out_dir/kiro-launcher.app (单文件，kiro-go 已内嵌)"
    else
        cp "$x64_launcher" "$out_dir/kiro-launcher"
        info "macOS universal binary: $out_dir/kiro-launcher (单文件，kiro-go 已内嵌)"
    fi

    # Verify
    info "Verifying universal binary:"
    lipo -info "$out_dir/kiro-launcher.app/Contents/MacOS/kiro-launcher" 2>/dev/null || \
    lipo -info "$out_dir/kiro-launcher" 2>/dev/null || true

    # Clean
    rm -f "$WAILS_DIR/sidecar/kiro-go" "$WAILS_DIR/sidecar/kiro-go.exe"
    rm -f "/tmp/kiro-go-universal"

    info "macOS universal build complete: $out_dir/"
}

# ── Windows x86_64 ──
build_win() {
    info "Building for Windows x86_64..."

    if ! command -v x86_64-w64-mingw32-gcc &>/dev/null; then
        error "mingw-w64 not found. Install with: brew install mingw-w64"
    fi

    build_go "windows" "amd64" "Windows x86_64"
    build_wails "windows" "amd64" "win-x64" "$GO_BIN"
}

# ── Linux amd64 (server) ──
build_linux() {
    build_go "linux" "amd64" "Linux amd64"

    local out_dir="$OUTPUT_DIR/linux-amd64"
    mkdir -p "$out_dir"
    cp "$GO_BIN" "$out_dir/kiro-go"
    info "Linux server binary: $out_dir/kiro-go"
}

# ── Deploy to server ──
deploy_server() {
    local server="${DEPLOY_SERVER:-root@117.72.183.248}"
    local remote_dir="/opt/kiro-proxy"

    build_go "linux" "amd64" "Linux amd64 (deploy)"

    info "Deploying to $server..."
    scp "$GO_BIN" "$server:$remote_dir/kiro-go-new"
    ssh "$server" "chmod +x $remote_dir/kiro-go-new && mv $remote_dir/kiro-go-new $remote_dir/kiro-go-latest && systemctl restart kiro-proxy && sleep 2 && systemctl status kiro-proxy --no-pager | head -12"
    info "Deploy complete!"
}

# ── Main ──

if [ $# -eq 0 ]; then
    case "$(uname -m)" in
        arm64|aarch64) build_mac_arm64 ;;
        x86_64)        build_mac_x64 ;;
        *)             error "Unknown architecture: $(uname -m)" ;;
    esac
    exit 0
fi

for target in "$@"; do
    case "$target" in
        mac)        build_mac_universal ;;
        mac-arm64)  build_mac_arm64 ;;
        mac-x64)    build_mac_x64 ;;
        win)        build_win ;;
        linux)      build_linux ;;
        server)     deploy_server ;;
        all)        build_mac_arm64; build_mac_x64; build_win; build_linux ;;
        clean)      clean ;;
        -h|--help)  usage; exit 0 ;;
        *)          error "Unknown target: $target. Run '$0 --help' for usage." ;;
    esac
done

info "All builds complete! Output: $OUTPUT_DIR/"
ls -la "$OUTPUT_DIR/" 2>/dev/null || true
