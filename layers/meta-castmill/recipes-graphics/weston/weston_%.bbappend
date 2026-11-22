# Kiosk shell pins one player window per output by app ID (ADR-0004); the
# systemd module provides sd_notify readiness and watchdog for the session.
PACKAGECONFIG:append = " shell-kiosk systemd"
# No demo clients, no extra shells, no Xwayland (minimal footprint).
PACKAGECONFIG:remove = "shell-ivi shell-fullscreen xwayland clients"
