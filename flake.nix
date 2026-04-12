{
  description = "ZaraOS - kernel build & QEMU test environment";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Cross-compilation toolchain: runs on host, produces aarch64-linux binaries
        crossPkgs = pkgs.pkgsCross.aarch64-multiplatform;
        crossCC = crossPkgs.stdenv.cc;

        # Static aarch64 BusyBox for the QEMU initramfs
        busyboxStatic = crossPkgs.busybox.override {
          enableStatic = true;
        };

        # Shim headers for cross-compiling Linux kernel on macOS.
        # Three issues to fix:
        #   1. No elf.h (macOS uses Mach-O)
        #   2. No byteswap.h (macOS has OSByteOrder instead)
        #   3. uuid_t typedef conflict (macOS sys/types.h defines it as char[16],
        #      kernel defines it as a struct — we force-include a compat header
        #      that pulls in macOS headers first, then undefs uuid_t)
        linuxHostHeaders = pkgs.runCommand "linux-host-headers" {} ''
          mkdir -p $out/include

          # elf.h — from cross-toolchain glibc
          cp ${crossPkgs.stdenv.cc.libc.dev}/include/elf.h $out/include/

          # byteswap.h — macOS compat shim
          cat > $out/include/byteswap.h << 'EOF'
          #ifndef _BYTESWAP_H
          #define _BYTESWAP_H
          #include <libkern/OSByteOrder.h>
          #define bswap_16(x) OSSwapInt16(x)
          #define bswap_32(x) OSSwapInt32(x)
          #define bswap_64(x) OSSwapInt64(x)
          #endif
          EOF

          # endian.h — macOS compat shim (macOS has machine/endian.h)
          cat > $out/include/endian.h << 'EOF'
          #ifndef _ENDIAN_H
          #define _ENDIAN_H
          #include <machine/endian.h>
          #include <libkern/OSByteOrder.h>
          #define htobe16(x) OSSwapHostToBigInt16(x)
          #define htole16(x) OSSwapHostToLittleInt16(x)
          #define be16toh(x) OSSwapBigToHostInt16(x)
          #define le16toh(x) OSSwapLittleToHostInt16(x)
          #define htobe32(x) OSSwapHostToBigInt32(x)
          #define htole32(x) OSSwapHostToLittleInt32(x)
          #define be32toh(x) OSSwapBigToHostInt32(x)
          #define le32toh(x) OSSwapLittleToHostInt32(x)
          #define htobe64(x) OSSwapHostToBigInt64(x)
          #define htole64(x) OSSwapHostToLittleInt64(x)
          #define be64toh(x) OSSwapBigToHostInt64(x)
          #define le64toh(x) OSSwapLittleToHostInt64(x)
          #endif
          EOF

          # compat-macos.h — force-included to fix uuid_t conflict.
          # macOS sys/types.h defines uuid_t as unsigned char[16].
          # The kernel's scripts/mod/file2alias.c defines uuid_t as a struct.
          # We include sys/types.h early (so its include guard prevents
          # re-inclusion later), then undef uuid_t so the kernel can define it.
          cat > $out/include/compat-macos.h << 'EOF'
          #ifdef __APPLE__
          /* Rename macOS uuid_t to avoid conflict with kernel's struct uuid_t.
             macOS sys/types.h typedefs uuid_t as unsigned char[16].
             The kernel's scripts/mod/file2alias.c typedefs uuid_t as a struct.
             By pre-defining uuid_t as a macro, the macOS typedef creates
             __darwin_compat_uuid_t instead, so no conflict occurs when the
             kernel later does: typedef struct { ... } uuid_t; */
          #define uuid_t __darwin_compat_uuid_t
          #include <sys/types.h>
          #include <unistd.h>
          #undef uuid_t
          #endif
          EOF
        '';
      in {
        devShells.default = pkgs.mkShell {
          depsBuildBuild = [ crossCC ];

          nativeBuildInputs = with pkgs; [
            # Kernel build tools (GNU Make 4.x required, macOS ships 3.81)
            gnumake
            bison
            flex
            bc
            perl
            openssl
            ncurses          # menuconfig TUI
            pkg-config
            dtc              # device tree compiler

            # QEMU for local VM testing
            qemu

            # Build acceleration
            ccache

            # Initramfs & archive tools
            cpio
            gnutar
            gzip
            rsync

            # General
            git
            curl
          ];

          shellHook = ''
            # Auto-detect cross-compiler prefix from nix
            for prefix in aarch64-unknown-linux-gnu- aarch64-linux-gnu-; do
              if command -v "''${prefix}gcc" &>/dev/null; then
                export CROSS_COMPILE="$prefix"
                break
              fi
            done
            export ARCH=arm64

            # Provide Linux-specific headers and macOS compat fixes to the host compiler.
            export HOSTCFLAGS="-I${linuxHostHeaders}/include -include ${linuxHostHeaders}/include/compat-macos.h"

            # Pre-built static aarch64 BusyBox for the QEMU initramfs
            export BUSYBOX_BIN="${busyboxStatic}/bin/busybox"

            echo ""
            echo "  ZaraOS Kernel Dev Shell"
            echo "  ───────────────────────────────────"
            if [ -n "''${CROSS_COMPILE:-}" ] && command -v "''${CROSS_COMPILE}gcc" &>/dev/null; then
              echo "  Cross-compiler: $(''${CROSS_COMPILE}gcc --version | head -1)"
            else
              echo "  Cross-compiler: not found (check nix build logs)"
            fi
            echo "  GNU Make:       $(make --version | head -1)"
            echo "  QEMU:           $(qemu-system-aarch64 --version | head -1)"
            echo ""
            echo "  Build & test kernel in QEMU:"
            echo "    ./ZaraOS/scripts/build-kernel-local.sh --qemu"
            echo "    ./ZaraOS/scripts/qemu-test-kernel.sh"
            echo ""
            echo "  Or one command:  ./ZaraOS/scripts/kernel-workflow.sh qemu"
            echo ""
          '';
        };
      });
}
