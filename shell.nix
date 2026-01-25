{ pkgs ? import <nixpkgs> {}}:
pkgs.mkShell {
  packages = with pkgs;[
    go
    gopls
    graphviz
    (python313.withPackages (ps: with ps; [
      matplotlib
    ]))
  ];
}
