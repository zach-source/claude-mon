{
  description = "claude-mon - TUI for monitoring Claude Code file edits in real-time";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = if (self ? rev) then self.shortRev else "dev";
      in
      {
        packages = {
          claude-mon = pkgs.buildGoModule {
            pname = "claude-mon";
            inherit version;
            src = ./.;

            vendorHash = "sha256-GMsaY7QvFkl0Zgz/3jUyflOH8ZtltgUgnvUP1NO2Tms=";

            # Exclude e2e tests that require the binary to be built first
            excludedPackages = [ "internal/e2e" ];

            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            meta = with pkgs.lib; {
              description = "TUI for monitoring Claude Code file edits in real-time";
              homepage = "https://github.com/ztaylor/claude-mon";
              license = licenses.mit;
              maintainers = [ ];
              mainProgram = "claude-mon";
            };
          };

          default = self.packages.${system}.claude-mon;
        };

        apps = {
          claude-mon = flake-utils.lib.mkApp {
            drv = self.packages.${system}.claude-mon;
          };
          default = self.apps.${system}.claude-mon;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            gopls
            golangci-lint
            goreleaser
          ];
        };
      }
    )
    // {
      # Home-manager module
      homeManagerModules = {
        claude-mon = import ./nix/hm-module.nix self;
        default = self.homeManagerModules.claude-mon;
      };

      # Overlay for use in other flakes
      overlays.default = final: prev: {
        claude-mon = self.packages.${prev.system}.claude-mon;
      };
    };
}
