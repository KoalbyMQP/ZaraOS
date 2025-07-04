# FIXME: setup flag should be set in justfile not in nix configs (to avoid burrying them!)
{
  description = "ZaraOS: ML-optimized Raspberry Pi OS with Integrated Package Manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    buildroot-src = {
      url = "github:buildroot/buildroot/2025.02.x";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, flake-utils, buildroot-src }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        # Build dependencies only
        buildrootDeps = with pkgs; [
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
          bison
          flex
          gperf
          texinfo
          help2man
          gnutar
          gzip
          bzip2
          xz
          unzip
          git
          subversion
          mercurial
          rsync
          python3
          bc
          ncurses5
          pkg-config
          perl
          cpio
          flock
          file
<<<<<<< HEAD
          qemu_full  # Full QEMU with all architectures
=======
>>>>>>> develop
        ];

        # Dev-only tools
        devTools = with pkgs; [
          podman
          just
          btop
          tree
          jq
          yq-go
          curl
          wget
        ];

        # Common shell setup
        commonShellHook = ''
          export BUILDROOT_SRC="${buildroot-src}"
          
          if [ ! -L "ZaraOS/buildroot" ]; then
            mkdir -p ZaraOS
            ln -sf "$BUILDROOT_SRC" ZaraOS/buildroot
          fi

          export ZARAOS_ROOT="$(pwd)"
          export BR2_EXTERNAL="$ZARAOS_ROOT/ZaraOS/external"
        '';

      in
      {
        devShells = {
          # Development environment with extra tools
          dev = pkgs.mkShell {
            buildInputs = buildrootDeps ++ devTools;

            shellHook = ''
              BLUE='\033[0;34m'
              GREEN='\033[0;32m'
              YELLOW='\033[1;33m'
              NC='\033[0m'

              echo -e "\n''${BLUE}=======================================''${NC}"
              echo -e "''${BLUE}  ZaraOS Development Environment''${NC}"
              echo -e "''${BLUE}=======================================''${NC}"
              echo -e "Platform: ''${YELLOW}${system}''${NC}"
              echo -e "Buildroot: ''${YELLOW}2025.02.x''${NC}"
              echo ""

              ${commonShellHook}

              echo -e "''${GREEN}✓ ZaraOS development environment ready''${NC}"
            '';
          };

          # Build environment - buildroot deps only
          build = pkgs.mkShell {
            buildInputs = buildrootDeps;
            
            shellHook = ''
              ${commonShellHook}
              
              export MAKEFLAGS="-j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 8)"
            '';
          };



          default = self.devShells.${system}.dev;
        };
      });
}