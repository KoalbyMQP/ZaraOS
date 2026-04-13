#!/bin/bash
set -e

# ===================================================================
# ZaraOS Buildroot Builder — Incremental-aware
# ===================================================================
# Detects existing build state and only re-configures when needed.
# Buildroot's own dependency tracking handles the rest — only changed
# packages are rebuilt, which takes ~2-5 min instead of 30-60 min.
# ===================================================================

WORKSPACE_PATH="${WORKSPACE_PATH:-$(pwd)}"
EXTERNAL_PATH="${EXTERNAL_PATH:-${WORKSPACE_PATH}/ZaraOS}"
BUILD_DIR="${BUILD_DIR:-/tmp/zaraos-build}"
DL_DIR="${DL_DIR:-/tmp/zaraos-dl}"
OUTPUT_DIR="${OUTPUT_DIR:-${WORKSPACE_PATH}/output}"
DEFCONFIG="${DEFCONFIG:-zaraos_pi5_defconfig}"
JOBS="${JOBS:-$(nproc)}"
BUILDROOT="/opt/buildroot"

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  ZaraOS Build — ${DEFCONFIG}"
echo "═══════════════════════════════════════════════════════════════"
echo "  Workspace:  $WORKSPACE_PATH"
echo "  External:   $EXTERNAL_PATH"
echo "  Build dir:  $BUILD_DIR"
echo "  Downloads:  $DL_DIR"
echo "  Jobs:       $JOBS"
echo "═══════════════════════════════════════════════════════════════"

if [[ ! -d "$BUILDROOT" ]]; then
    echo "ERROR: Buildroot not found at $BUILDROOT"
    exit 1
fi

if [[ ! -d "$EXTERNAL_PATH" ]]; then
    echo "ERROR: ZaraOS external tree not found at $EXTERNAL_PATH"
    exit 1
fi

mkdir -p "$BUILD_DIR" "$DL_DIR" "$OUTPUT_DIR"

# ── Decide whether to (re-)configure ────────────────────────────────
needs_configure=false

if [[ ! -f "$BUILD_DIR/.config" ]]; then
    echo "[INFO] No existing .config — running initial configure"
    needs_configure=true
else
    # Check if the defconfig changed since last configure.
    # Buildroot writes the defconfig name into .br2-external.in;
    # a simple stamp file is more reliable.
    STAMP="$BUILD_DIR/.zaraos-defconfig-stamp"
    DEFCONFIG_FILE="$EXTERNAL_PATH/configs/$DEFCONFIG"

    if [[ ! -f "$STAMP" ]]; then
        echo "[INFO] No defconfig stamp — reconfiguring"
        needs_configure=true
    elif [[ -f "$DEFCONFIG_FILE" ]] && [[ "$DEFCONFIG_FILE" -nt "$STAMP" ]]; then
        echo "[INFO] Defconfig changed since last build — reconfiguring"
        needs_configure=true
    fi
fi

if $needs_configure; then
    echo "[BUILD] Configuring with $DEFCONFIG..."
    make -C "$BUILDROOT" \
        O="$BUILD_DIR" \
        BR2_DL_DIR="$DL_DIR" \
        BR2_EXTERNAL="$EXTERNAL_PATH" \
        "$DEFCONFIG"

    # Write stamp
    touch "$BUILD_DIR/.zaraos-defconfig-stamp"
    echo "[OK] Configured"
else
    echo "[SKIP] Config up to date — skipping configure"
fi

# ── Build (Buildroot handles incremental dependencies) ──────────────
echo "[BUILD] Building... (incremental — only changed packages rebuild)"
make -C "$BUILDROOT" \
    O="$BUILD_DIR" \
    BR2_DL_DIR="$DL_DIR" \
    BR2_EXTERNAL="$EXTERNAL_PATH" \
    -j"$JOBS"

echo "[OK] Build complete"

# ── Copy artifacts ──────────────────────────────────────────────────
if [ -d "$BUILD_DIR/images" ]; then
    mkdir -p "$OUTPUT_DIR"
    cp -r "$BUILD_DIR/images"/* "$OUTPUT_DIR/"
    echo ""
    echo "Artifacts:"
    ls -lh "$OUTPUT_DIR/" | grep -v "^total"
fi
