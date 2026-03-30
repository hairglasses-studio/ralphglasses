{
  description = "ralphglasses — command-and-control TUI for parallel multi-LLM agent fleets";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # NixOS module — enables ralphglasses as a systemd service.
      # Usage in a NixOS configuration:
      #   imports = [ ralphglasses.nixosModules.default ];
      #   services.ralphglasses.enable = true;
      #   services.ralphglasses.scanPath = "/home/user/projects";
      nixosModule = { config, lib, pkgs, ... }:
        let
          cfg = config.services.ralphglasses;
          ralphglassesPkg = self.packages.${pkgs.system}.default;
        in {
          options.services.ralphglasses = {
            enable = lib.mkEnableOption "ralphglasses MCP server and fleet controller";

            scanPath = lib.mkOption {
              type = lib.types.str;
              default = "/home/ralphglasses/projects";
              description = "Root directory to scan for ralph-enabled repositories.";
            };

            logLevel = lib.mkOption {
              type = lib.types.enum [ "debug" "info" "warn" "error" ];
              default = "info";
              description = "Log level for the ralphglasses process.";
            };

            httpAddr = lib.mkOption {
              type = lib.types.str;
              default = ":9090";
              description = "HTTP address for /healthz, /readyz, and /metrics endpoints. Empty string disables.";
            };

            user = lib.mkOption {
              type = lib.types.str;
              default = "ralphglasses";
              description = "System user under which the service runs.";
            };

            group = lib.mkOption {
              type = lib.types.str;
              default = "ralphglasses";
              description = "System group under which the service runs.";
            };

            extraArgs = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [];
              description = "Additional command-line arguments passed to ralphglasses.";
              example = [ "--telemetry" "--log-format" "json" ];
            };
          };

          config = lib.mkIf cfg.enable {
            users.users.${cfg.user} = {
              isSystemUser = true;
              group = cfg.group;
              description = "ralphglasses service user";
              home = "/var/lib/ralphglasses";
              createHome = true;
            };

            users.groups.${cfg.group} = {};

            systemd.services.ralphglasses = {
              description = "ralphglasses fleet controller";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              serviceConfig = {
                Type = "simple";
                User = cfg.user;
                Group = cfg.group;
                WorkingDirectory = "/var/lib/ralphglasses";
                StateDirectory = "ralphglasses";
                LogsDirectory = "ralphglasses";

                ExecStart = lib.escapeShellArgs ([
                  "${ralphglassesPkg}/bin/ralphglasses"
                  "mcp"
                  "--scan-path" cfg.scanPath
                  "--log-level" cfg.logLevel
                ] ++ lib.optionals (cfg.httpAddr != "") [
                  "--http-addr" cfg.httpAddr
                ] ++ cfg.extraArgs);

                Restart = "on-failure";
                RestartSec = "5s";

                # Sandboxing / hardening
                NoNewPrivileges = true;
                ProtectSystem = "strict";
                ProtectHome = "read-only";
                PrivateTmp = true;
                PrivateDevices = true;
                ReadWritePaths = [ "/var/lib/ralphglasses" cfg.scanPath ];

                # systemd watchdog integration (sd_notify)
                WatchdogSec = "60s";
                NotifyAccess = "main";
              };
            };
          };
        };
    in
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
    ) // {
      # NixOS module exports — importable by downstream flakes.
      nixosModules.default = nixosModule;
      nixosModules.ralphglasses = nixosModule;
    };
}
