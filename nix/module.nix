{ config, lib, pkgs, kiro-bridge, ... }:

let
  cfg = config.services.kiro-bridge;
  defaultPackage = kiro-bridge.packages.${pkgs.stdenv.hostPlatform.system}.default;
  homeDir = "/Users/${cfg.user}";
in {
  options.services.kiro-bridge = {
    enable = lib.mkEnableOption "kiro-bridge HTTP-to-ACP proxy";

    package = lib.mkOption {
      type = lib.types.package;
      default = defaultPackage;
      description = "The kiro-bridge package to use.";
    };

    cwd = lib.mkOption {
      type = lib.types.str;
      default = homeDir;
      description = "Working directory for ACP sessions.";
    };

    port = lib.mkOption {
      type = lib.types.port;
      default = 11435;
      description = "HTTP server port.";
    };

    cliPath = lib.mkOption {
      type = lib.types.str;
      default = "${homeDir}/.nix-profile/bin/kiro-cli";
      description = "Path to kiro-cli binary.";
    };

    agent = lib.mkOption {
      type = lib.types.str;
      default = "kiro-bridge";
      description = "Kiro agent config to activate.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      description = "User to run the service as.";
    };

    extraEnv = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = {};
      description = "Extra environment variables passed to the service.";
    };
  };

  config = lib.mkIf cfg.enable {
    environment.etc."newsyslog.d/kiro-bridge.conf".text = ''
      # logfilename          [owner:group]  mode  count  size  when  flags
      /tmp/kiro-bridge.log                  644   3      1024  *     J
    '';

    launchd.agents.kiro-bridge = {
      serviceConfig = {
        Program = "${cfg.package}/bin/kiro-bridge";
        EnvironmentVariables = {
          KIRO_BRIDGE_PORT = toString cfg.port;
          KIRO_BRIDGE_CWD = cfg.cwd;
          KIRO_CLI_PATH = cfg.cliPath;
          KIRO_BRIDGE_AGENT = cfg.agent;
          HOME = homeDir;
          PATH = lib.concatStringsSep ":" [
            "${homeDir}/.nix-profile/bin"
            "/run/current-system/sw/bin"
            "/nix/var/nix/profiles/default/bin"
            "/usr/bin"
            "/bin"
          ];
        } // cfg.extraEnv;
        KeepAlive = true;
        RunAtLoad = true;
        ThrottleInterval = 10;
        StandardOutPath = "/tmp/kiro-bridge.log";
        StandardErrorPath = "/tmp/kiro-bridge.log";
      };
    };
  };
}
