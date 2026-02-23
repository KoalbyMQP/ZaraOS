{
  description = "Cortex";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "cortex";
          version = "0.0.1";
          src = ./.;

          # Run `nix build` once, it will fail and print the correct hash.
          # Paste that hash here.
          vendorHash = null;

          # cortex main package lives under cmd/
          subPackages = [ "cmd" ];

          meta = {
            description = "Robot control plane API";
            mainProgram = "cmd";
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
          ];
        };
      });
}
