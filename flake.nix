{
  description = "Install dependencies to build and run the egwalker development environment";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs?rev=a3116115851d68b8952a2a4221cc25a84e56b532";
    systems.url = "github:nix-systems/default";
    flake-utils = {
      url = "github:numtide/flake-utils";
      inputs.systems.follows = "systems";
    };
  };
  outputs = {
    nixpkgs,
    flake-utils,
    ...
  }:
    flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            graphviz
            (python313.withPackages (ps: with ps; [
              matplotlib
            ]))
          ];
        };
      }
    );
}
