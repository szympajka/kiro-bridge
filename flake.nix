{
  description = "kiro-bridge: OpenAI-compatible HTTP proxy to kiro-cli ACP";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs:
    inputs.flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "aarch64-darwin" "x86_64-darwin" "x86_64-linux" "aarch64-linux" ];

      flake.darwinModules.default = ./nix/module.nix;

      perSystem = { pkgs, ... }: let
        pname = "kiro-bridge";
        version = builtins.replaceStrings ["\n"] [""] (builtins.readFile ./.version);
      in {
        packages.default = pkgs.buildGoModule {
          inherit pname version;
          src = ./.;
          vendorHash = null;
          ldflags = [ "-X main.version=${version}" ];
        };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls ];
        };
      };
    };
}
