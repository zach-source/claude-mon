# Home-manager module for claude-mon
flake:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.claude-mon;
  tomlFormat = pkgs.formats.toml { };
in
{
  options.programs.claude-mon = {
    enable = lib.mkEnableOption "claude-mon TUI for monitoring Claude Code";

    package = lib.mkOption {
      type = lib.types.package;
      default = flake.packages.${pkgs.system}.claude-mon;
      defaultText = lib.literalExpression "flake.packages.\${pkgs.system}.claude-mon";
      description = "The claude-mon package to use.";
    };

    settings = lib.mkOption {
      type = tomlFormat.type;
      default = { };
      example = lib.literalExpression ''
        {
          theme = "catppuccin";
          leader_key = "ctrl+g";
          keys = {
            quit = "q";
            help = "?";
            next_tab = "tab";
            prev_tab = "shift+tab";
          };
        }
      '';
      description = ''
        Configuration for claude-mon. See the project documentation for
        available options.
      '';
    };

    daemon = {
      enable = lib.mkEnableOption "claude-mon daemon for background edit tracking";

      autoStart = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Whether to automatically start the daemon service.";
      };
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    xdg.configFile."claude-follow/config.toml" = lib.mkIf (cfg.settings != { }) {
      source = tomlFormat.generate "claude-mon-config" cfg.settings;
    };

    # Daemon service (launchd on macOS, systemd on Linux)
    launchd.agents.claude-mon-daemon = lib.mkIf (cfg.daemon.enable && pkgs.stdenv.isDarwin) {
      enable = cfg.daemon.autoStart;
      config = {
        Label = "com.claude-mon.daemon";
        ProgramArguments = [
          "${cfg.package}/bin/claude-mon"
          "daemon"
        ];
        KeepAlive = true;
        RunAtLoad = true;
        StandardOutPath = "/tmp/claude-mon-daemon.log";
        StandardErrorPath = "/tmp/claude-mon-daemon.err";
      };
    };

    systemd.user.services.claude-mon-daemon = lib.mkIf (cfg.daemon.enable && pkgs.stdenv.isLinux) {
      Unit = {
        Description = "Claude-mon daemon for background edit tracking";
        After = [ "graphical-session.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "${cfg.package}/bin/claude-mon daemon";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = {
        WantedBy = [ "default.target" ];
      };
    };
  };
}
