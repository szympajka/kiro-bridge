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
        tagRelease = pkgs.writeShellApplication {
          name = "tag-release";
          runtimeInputs = [ pkgs.git ];
          text = ''
            set -euo pipefail

            repo_root="$(git rev-parse --show-toplevel)"
            cd "$repo_root"

            if [ ! -f .version ]; then
              echo ".version file not found" >&2
              exit 1
            fi

            version="$(tr -d '[:space:]' < .version)"
            case "$version" in
              [0-9]*.[0-9]*.[0-9]*) ;;
              *)
                echo "invalid version in .version: $version" >&2
                exit 1
                ;;
            esac

            if [ -n "$(git status --short)" ]; then
              echo "git worktree is dirty; commit or stash changes before tagging" >&2
              exit 1
            fi

            tag="v$version"
            if git rev-parse --verify --quiet "refs/tags/$tag" >/dev/null; then
              echo "tag already exists: $tag" >&2
              exit 1
            fi

            git tag -a "$tag" -m "Release $tag"
            echo "created tag $tag"
          '';
        };
        release = pkgs.writeShellApplication {
          name = "release";
          runtimeInputs = [ pkgs.git tagRelease ];
          text = ''
            set -euo pipefail

            repo_root="$(git rev-parse --show-toplevel)"
            cd "$repo_root"

            current_branch="$(git symbolic-ref --quiet --short HEAD || true)"
            if [ -z "$current_branch" ]; then
              echo "HEAD is detached; checkout a branch before releasing" >&2
              exit 1
            fi

            tag-release
            git push origin "$current_branch" --tags
            echo "pushed branch $current_branch and tags"
          '';
        };
      in {
        packages.default = pkgs.buildGoModule {
          inherit pname version;
          src = ./.;
          vendorHash = null;
          ldflags = [ "-X main.version=${version}" ];
        };

        apps.tag-release = {
          type = "app";
          program = "${tagRelease}/bin/tag-release";
        };

        apps.release = {
          type = "app";
          program = "${release}/bin/release";
        };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls ];
        };
      };
    };
}
