{
  description = "ralphglasses — command-and-control TUI for parallel multi-LLM agent fleets";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "ralphglasses";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # Will need updating after first build

          # Skip tests that need network/display
          checkFlags = [ "-short" ];

          meta = with pkgs.lib; {
            description = "Command-and-control TUI for parallel multi-LLM agent fleets";
            homepage = "https://github.com/hairglasses-studio/ralphglasses";
            license = licenses.mit;
            maintainers = [];
            platforms = platforms.unix;
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            goreleaser
          ];
        };
      }
    );
}
