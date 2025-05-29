{
  description = "ZaraOS: ML-optimized Raspberry Pi OS with Integrated Package Manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    buildroot-src = {
      url = "github:buildroot/buildroot/2025.02.x"; # FIXME: is pinning this version ideal? 
      flake = false;
    };
  };

  outputs = { self, nixpkgs, flake-utils, buildroot-src }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        commonPackages = with pkgs; [
          git
          just
          btop
          tree
          jq
          yq-go
          curl
          wget
        ];

        buildrootDeps = with pkgs; [
          # Essential build tools
          gcc
          gnumake
          binutils
          coreutils
          findutils
          diffutils
          patch
          gawk
          gnused
          gnugrep
          which
          file
          
          # Cross-compilation toolchain deps
          bison
          flex
          gperf
          texinfo
          help2man
          
          # Archive handling
          gnutar
          gzip
          bzip2
          xz
          unzip
          
          # Version control
          git
          subversion
          mercurial
          
          # Network tools for downloading sources
          rsync
          
          # Python for Buildroot scripts
          python3
          
          # Additional tools
          bc
          ncurses5
          pkg-config
        ];

        piCrossDeps = with pkgs; [
          pkgsCross.aarch64-multiplatform.stdenv.cc
          qemu
        ];

        docAndTestDeps = with pkgs; [
          # cross-platform
         ] ++ lib.optionals stdenv.isLinux [
          # Linux-only
          lshw
          usbutils
          pciutils
          parted
          dosfstools
          e2fsprogs
        ];

        darwinPackages = with pkgs; lib.optionals stdenv.isDarwin [
          darwin.cctools
          libiconv
        ];

        linuxPackages = with pkgs; lib.optionals stdenv.isLinux [
          strace
          ltrace
        ];

      in
      {
        devShells = {
          default = pkgs.mkShell {
            buildInputs = commonPackages 
                       ++ buildrootDeps 
                       ++ piCrossDeps 
                       ++ docAndTestDeps 
                       ++ darwinPackages 
                       ++ linuxPackages;

            shellHook = ''
              # Simple colors
              BLUE='\033[0;34m'
              GREEN='\033[0;32m'
              YELLOW='\033[1;33m'
              RED='\033[0;31m'
              NC='\033[0m' # No Color

              echo -e "\n''${BLUE}=======================================''${NC}"
              echo -e "''${BLUE}  ZaraOS Development Environment''${NC}"
              echo -e "''${BLUE}=======================================''${NC}"
              echo -e "Platform: ''${YELLOW}${system}''${NC}"
              echo -e "Buildroot: ''${YELLOW}2025.02.x''${NC}" # FIXME: print actual version 
              echo ""

              # Set up Buildroot path
              export BUILDROOT_SRC="${buildroot-src}"
              
              # Link to buildroot in nix store
              if [ ! -L "ZaraOS/buildroot" ]; then
                mkdir -p ZaraOS
                ln -sf "$BUILDROOT_SRC" ZaraOS/buildroot
              fi

              export ZARAOS_ROOT="" # TODO:: add corretc path here
              export PATH="$ZARAOS_ROOT/tools:$PATH"

              echo -e "''${GREEN}✓ ZaraOS development environment ready''${NC}"
            '';

            BR2_EXTERNAL = "./ZaraOS/external/"; # external buildroot configs
          };

          ci = pkgs.mkShell {
            buildInputs = commonPackages ++ buildrootDeps;
            
            shellHook = ''
              export BUILDROOT_SRC="${buildroot-src}"
              export ZARAOS_ROOT="$(pwd)"
              export MAKEFLAGS="-j$(nproc)"
              
              # Link to buildroot in nix store
              if [ ! -L "ZaraOS/buildroot" ]; then
                mkdir -p ZaraOS
                ln -sf "$BUILDROOT_SRC" ZaraOS/buildroot
              fi
            '';
          };
        };

        # Package the development tools
        packages = {
        };
      });
}