#!/bin/bash
set -e

# ============================================================================
# Kiro Proxy Build Script
# Builds kiro-rs (Rust) + kiro-launcher (Wails/Go) for macOS and Windows
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RUST_DIR="$SCRIPT_DIR/kiro.rs"
WAILS_DIR="$SCRIPT_DIR/kiro-launcher"
OUTPUT_DIR="$SCRIPT_DIR/release"
CARGO="${CARGO:-$HOME/.cargo/bin/cargo}"
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
    echo "  all          Build for all platforms"
    echo "  clean        Clean build artifacts"
    echo ""
    echo "If no target is specified, builds for current platform."
}

clean() {
    info "Cleaning build artifacts..."
    rm -rf "$OUTPUT_DIR"
    (cd "$SCRIPT_DIR" && $CARGO clean 2>/dev/null || true)
    rm -rf "$WAILS_DIR/build"
    info "Clean complete."
}

# Build kiro-rs for a specific Rust target
# Sets RUST_BIN to the output binary path
RUST_BIN=""
build_rust() {
    local target="$1"
    local label="$2"
    info "Building kiro-rs for $label ($target)..."
    
    (cd "$SCRIPT_DIR" && $CARGO build --release --target "$target" -p kiro-rs)
    
    local ext=""
    [[ "$target" == *"windows"* ]] && ext=".exe"
    
    RUST_BIN="$SCRIPT_DIR/target/$target/release/kiro-rs${ext}"
    if [ ! -f "$RUST_BIN" ]; then
        error "kiro-rs binary not found at $RUST_BIN"
    fi
    info "kiro-rs binary: $RUST_BIN"
}

# Stage kiro-rs binary into sidecar/ for embedding
stage_sidecar() {
    local rust_binary="$1"
    local goos="$2"
    
    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"
    
    local sidecar_dir="$WAILS_DIR/sidecar"
    mkdir -p "$sidecar_dir"
    # Clean old binaries
    rm -f "$sidecar_dir/kiro-rs" "$sidecar_dir/kiro-rs.exe"
    
    cp "$rust_binary" "$sidecar_dir/kiro-rs${ext}"
    info "Staged kiro-rs into sidecar/ for embedding"
}

# Build kiro-launcher (Wails) with embedded kiro-rs
# Produces a SINGLE binary that contains everything
build_wails() {
    local goos="$1"
    local goarch="$2"
    local label="$3"
    local rust_binary="$4"
    
    # Stage the Rust binary for go:embed
    stage_sidecar "$rust_binary" "$goos"
    
    info "Building kiro-launcher for $label (GOOS=$goos GOARCH=$goarch)..."
    
    local out_dir="$OUTPUT_DIR/$label"
    mkdir -p "$out_dir"
    
    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"
    
    if [ "$goos" == "darwin" ]; then
        (cd "$WAILS_DIR" && GOOS=$goos GOARCH=$goarch $WAILS build -clean -o "kiro-launcher")
        
        if [ -d "$WAILS_DIR/build/bin/kiro-launcher.app" ]; then
            cp -R "$WAILS_DIR/build/bin/kiro-launcher.app" "$out_dir/"
            info "macOS app bundle: $out_dir/kiro-launcher.app (single binary, kiro-rs embedded)"
        else
            cp "$WAILS_DIR/build/bin/kiro-launcher" "$out_dir/"
            info "macOS binary: $out_dir/kiro-launcher (kiro-rs embedded)"
        fi
    else
        (cd "$WAILS_DIR" && \
            CGO_ENABLED=1 \
            GOOS=$goos GOARCH=$goarch \
            CC=x86_64-w64-mingw32-gcc \
            CXX=x86_64-w64-mingw32-g++ \
            $WAILS build -clean -o "kiro-launcher${ext}" -skipbindings)
        
        cp "$WAILS_DIR/build/bin/kiro-launcher${ext}" "$out_dir/"
        info "Windows binary: $out_dir/kiro-launcher${ext} (kiro-rs embedded)"
    fi
    
    # Clean sidecar after build
    rm -f "$WAILS_DIR/sidecar/kiro-rs" "$WAILS_DIR/sidecar/kiro-rs.exe"
}

# ── macOS arm64 ──
build_mac_arm64() {
    build_rust "aarch64-apple-darwin" "macOS arm64"
    build_wails "darwin" "arm64" "mac-arm64" "$RUST_BIN"
}

# ── macOS x86_64 ──
build_mac_x64() {
    build_rust "x86_64-apple-darwin" "macOS x86_64"
    build_wails "darwin" "amd64" "mac-x64" "$RUST_BIN"
}

# ── macOS universal ──
build_mac_universal() {
    info "Building macOS universal binary..."
    
    # Build both architectures for Rust
    build_rust "aarch64-apple-darwin" "macOS arm64"
    local arm64_rs="$RUST_BIN"
    build_rust "x86_64-apple-darwin" "macOS x86_64"
    local x64_rs="$RUST_BIN"
    
    # Create universal kiro-rs binary (temp location)
    local universal_rs="/tmp/kiro-rs-universal"
    lipo -create "$arm64_rs" "$x64_rs" -output "$universal_rs"
    info "Created universal kiro-rs binary"
    
    # Build Wails for both architectures, then lipo merge
    # -- arm64 --
    stage_sidecar "$universal_rs" "darwin"
    info "Building kiro-launcher for macOS arm64..."
    (cd "$WAILS_DIR" && GOOS=darwin GOARCH=arm64 $WAILS build -clean -o "kiro-launcher")
    
    local arm64_app="$WAILS_DIR/build/bin/kiro-launcher.app"
    local arm64_launcher=""
    if [ -d "$arm64_app" ]; then
        arm64_launcher="$arm64_app/Contents/MacOS/kiro-launcher"
    else
        arm64_launcher="$WAILS_DIR/build/bin/kiro-launcher"
    fi
    # Stash arm64 binary
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
    
    # Merge into universal binary in-place
    info "Creating universal kiro-launcher binary with lipo..."
    lipo -create "/tmp/kiro-launcher-arm64" "$x64_launcher" -output "$x64_launcher"
    rm -f "/tmp/kiro-launcher-arm64"
    
    # Copy final .app bundle (or binary) to output
    local out_dir="$OUTPUT_DIR/mac-universal"
    mkdir -p "$out_dir"
    if [ -d "$x64_app" ]; then
        cp -R "$x64_app" "$out_dir/kiro-launcher.app"
        info "macOS universal app: $out_dir/kiro-launcher.app"
    else
        cp "$x64_launcher" "$out_dir/kiro-launcher"
        info "macOS universal binary: $out_dir/kiro-launcher"
    fi
    
    # Clean up: remove intermediate kiro-rs from output (it's embedded in the .app)
    rm -f "$out_dir/kiro-rs"
    
    # Verify universal
    info "Verifying universal binary:"
    lipo -info "$out_dir/kiro-launcher.app/Contents/MacOS/kiro-launcher" 2>/dev/null || \
    lipo -info "$out_dir/kiro-launcher" 2>/dev/null || true
    
    # Clean sidecar and temp files
    rm -f "$WAILS_DIR/sidecar/kiro-rs" "$WAILS_DIR/sidecar/kiro-rs.exe"
    rm -f "/tmp/kiro-rs-universal"
    
    info "macOS universal build complete: $out_dir/"
}

# ── Windows x86_64 ──
build_win() {
    info "Building for Windows x86_64..."
    
    # Check for mingw-w64
    if ! command -v x86_64-w64-mingw32-gcc &>/dev/null; then
        error "mingw-w64 not found. Install with: brew install mingw-w64"
    fi
    
    build_rust "x86_64-pc-windows-gnu" "Windows x86_64"
    build_wails "windows" "amd64" "win-x64" "$RUST_BIN"
}

# ── Main ──

if [ $# -eq 0 ]; then
    # Default: build for current platform
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
        all)        build_mac_arm64; build_mac_x64; build_win ;;
        clean)      clean ;;
        -h|--help)  usage; exit 0 ;;
        *)          error "Unknown target: $target. Run '$0 --help' for usage." ;;
    esac
done

info "All builds complete! Output: $OUTPUT_DIR/"
ls -la "$OUTPUT_DIR/"
