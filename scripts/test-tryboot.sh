#!/bin/sh
# Tests the Raspberry Pi tryboot RAUC backend (castmill-tryboot, ADR-0009)
# through an update, a rollback and a recovery, with temporary files in
# place of the boot partition, /run and /proc.
set -eu

BACKEND="$(cd "$(dirname "$0")/.." && pwd)/layers/meta-castmill/recipes-bsp/castmill-tryboot/files/castmill-tryboot"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

cat >"$T/tryboot.conf" <<'EOF'
SLOT_A_PART=2
SLOT_B_PART=3
EOF
export CASTMILL_TRYBOOT_CONF="$T/tryboot.conf"
export AUTOBOOT="$T/autoboot.txt"
export REBOOT_PARAM="$T/run/reboot-param"
export BAD_FILE="$T/data/tryboot-bad"
export PROC_CMDLINE="$T/cmdline"
printf '[all]\ntryboot_a_b=1\nboot_partition=2\n[tryboot]\nboot_partition=3\n' >"$AUTOBOOT"

fails=0
rb() { sh "$BACKEND" "$@"; }
boot() { echo "console=tty1 root=PARTUUID=x rauc.slot=$1 rootwait" >"$PROC_CMDLINE"; }
# A reboot: the firmware uses [tryboot] once if armed; /run is cleared.
reboot() {
    if grep -qw tryboot "$REBOOT_PARAM" 2>/dev/null; then
        p=$(awk '/^\[tryboot\]/{s=1;next} /^\[/{s=0} s&&/^boot_partition=/{sub(/.*=/,"");print}' "$AUTOBOOT")
    else
        p=$(awk '/^\[all\]/{s=1;next} /^\[/{s=0} s&&/^boot_partition=/{sub(/.*=/,"");print}' "$AUTOBOOT")
    fi
    rm -f "$REBOOT_PARAM"
    if [ "$p" = 2 ]; then boot A; else boot B; fi
}
expect() { # description, expected, actual
    if [ "$2" = "$3" ]; then
        echo "ok   - $1"
    else
        echo "FAIL - $1: expected '$2', got '$3'"
        fails=$((fails + 1))
    fi
}
booted() { rb get-current; }

boot A
expect "boots slot A" A "$(booted)"
expect "A is primary" A "$(rb get-primary)"
expect "A is good" good "$(rb get-state A)"

# Update installed into B, activated (mark-active other).
rb set-primary B
expect "B is primary after activation" B "$(rb get-primary)"
expect "next reboot is a tryboot" "0 tryboot" "$(cat "$REBOOT_PARAM")"
# Power cut before the planned reboot: /run is lost, A boots.
rm -f "$REBOOT_PARAM"
reboot
expect "power cut before activation boots A" A "$(booted)"
expect "A is primary again" A "$(rb get-primary)"

# Activate again and reboot: trial boot of B.
rb set-primary B
reboot
expect "trial boot runs B" B "$(booted)"
expect "B isn't committed yet" "[all]" "$(grep -A2 '^\[all\]' "$AUTOBOOT" | grep -q 'boot_partition=2' && echo '[all]')"
# Health check passes: mark-good booted commits B.
rb set-state B good
expect "B committed" B "$(rb get-primary)"
reboot
expect "B boots after commit" B "$(booted)"

# Next update into A fails its health check: mark-bad booted, reboot.
rb set-primary A
reboot
expect "trial boot runs A" A "$(booted)"
rb set-state A bad
expect "A is bad" bad "$(rb get-state A)"
expect "B stays primary" B "$(rb get-primary)"
reboot
expect "rollback boots B" B "$(booted)"

# A trial that hangs: the watchdog reset is a normal boot.
rb set-primary A
rm -f "$REBOOT_PARAM" # reset without the tryboot flag
reboot
expect "watchdog reset boots B" B "$(booted)"

# Re-activating the committed slot cancels a pending trial.
rb set-primary A
rb set-primary B
expect "no tryboot armed" "" "$(cat "$REBOOT_PARAM" 2>/dev/null || true)"

# A later, good update into A clears its bad flag when it commits.
rb set-primary A
reboot
rb set-state A good
expect "A good again" good "$(rb get-state A)"
expect "A committed" A "$(rb get-primary)"

# Errors.
if rb set-primary C 2>/dev/null; then fails=$((fails + 1)) && echo "FAIL - unknown slot accepted"; else echo "ok   - unknown slot rejected"; fi
if rb set-state A maybe 2>/dev/null; then fails=$((fails + 1)) && echo "FAIL - unknown state accepted"; else echo "ok   - unknown state rejected"; fi

[ "$fails" = 0 ] || { echo "$fails failed"; exit 1; }
echo "all passed"
