# ADR-0009: A/B boot on Raspberry Pi 5 with the firmware's tryboot

- Status: Accepted
- Date: 2026-11-23
- Resolves: the open question in [spec 03](../specs/03-updates.md#open-questions) ("Exact boot-counting mechanism on Pi 5")

## Context

Castmill OS updates the inactive root filesystem slot with RAUC and rolls back when the new slot doesn't boot or fails the health check ([ADR-0002](0002-rauc-ab-updates.md)). On x86-64, GRUB does the boot counting. The Raspberry Pi 5 has no GRUB: its EEPROM bootloader loads the kernel, device tree and `config.txt` from a FAT partition, and can't read ext4.

`castmill-agent` and `castmill-health` only talk to RAUC: `mark-active` to activate an update, `mark-good` to commit it, `mark-bad` to reject it. Whatever the Pi uses must fit that model, including two rules from spec 03 and spec 05:

- An update is only activated inside the maintenance window. A power cut before then must not boot the new slot (REL-005).
- If the new slot hangs or fails the health check, the device comes back on the previous slot by itself (REL-001).

## Decision

Use the Pi firmware's **tryboot** A/B support, with one FAT boot partition per slot and a RAUC custom bootloader backend.

**Disk layout** (GPT):

| # | Partition | Contents |
|---|---|---|
| 1 | `boot` (FAT, 64 MB) | `autoboot.txt`; `castmill.conf` and `factory-reset` as on other targets |
| 2 | `boot_a` (FAT, 128 MB) | Kernel, device trees, overlays, `config.txt`, `cmdline.txt` for slot A |
| 3 | `boot_b` (FAT, 128 MB) | The same for slot B |
| 4, 5 | `rootfs_a`, `rootfs_b` | Root filesystems |
| 6 | `data` | Persistent data |

`autoboot.txt` on partition 1 selects the boot partition:

```ini
[all]
tryboot_a_b=1
boot_partition=2
[tryboot]
boot_partition=3
```

The `[all]` section is the committed slot. The `[tryboot]` section is used once, after a reboot with the `tryboot` flag (`reboot "0 tryboot"`). Any later reset (watchdog, power cut, a normal reboot) boots the committed slot again.

**RAUC backend** (`bootloader=custom`, `castmill-tryboot`):

| RAUC call | What it does |
|---|---|
| `set-primary X` (`mark-active`) | `[tryboot] boot_partition` = slot X; writes `0 tryboot` to `/run/systemd/reboot-param`, so the agent's next reboot is a tryboot |
| `set-state X good` (`mark-good`) | If X is the booted slot: `[all] boot_partition` = slot X (commit); clears X's bad flag |
| `set-state X bad` (`mark-bad`) | Records X as bad (`/data/rauc/tryboot-bad`); cancels a pending tryboot into X |
| `get-primary` | The pending tryboot slot if armed, otherwise the committed slot |
| `get-state X` | `bad` if recorded, otherwise `good` |
| `get-current` | The booted slot (`rauc.slot=` on the kernel command line) |

`/run/systemd/reboot-param` is read by systemd when it reboots, whatever asked for the reboot. It lives in `/run`, so a power cut forgets it: REL-005 holds. A hang in the trial boot ends with a watchdog reset into the committed slot, and `castmill-health` then marks the new slot bad, as on x86.

**Boot files in updates:** the bundle still contains only the root filesystem. The image installs the boot files (kernel, device trees, overlays, `config.txt`, `cmdline.txt`) in `/usr/lib/castmill/boot`. A RAUC `post-install` handler formats the target slot's boot partition, copies them from the new root filesystem, and sets `root=` and `rauc.slot=` in `cmdline.txt` for that slot. The Pi's bootloader and firmware live in its EEPROM and are not updated by Castmill OS (see Consequences).

## Alternatives considered

- **U-Boot with RAUC's U-Boot backend.** Standard boot counting, but adds a bootloader to maintain, Pi 5 support in U-Boot is less complete than the firmware's (PCIe/NVMe boot, display), and boot gets slower. tryboot is built into the firmware Raspberry Pi maintains.
- **One boot partition with `os_prefix` directories per slot.** Fewer partitions, but every update rewrites files on the partition that also holds the committed slot's boot files and `config.txt`; a power cut during that write can break both slots. Separate partitions keep the committed slot's files untouched.
- **Boot files as a second RAUC slot in the bundle.** Needs a FAT image per bundle, and `cmdline.txt` differs per slot (`root=`), so it would still need a post-install fix-up. Building the boot partition on the device from the new root filesystem is simpler.

## Consequences

- The Pi image installs `rauc`, `castmill-tryboot` and `dosfstools`, and the build produces a RAUC bundle for the Pi as for x86-64.
- Existing Pi installs (v0.1, two partitions fewer) can't be updated in place; they have to be re-flashed once.
- The EEPROM bootloader isn't updated by Castmill OS. It must support tryboot with `autoboot.txt` (all Pi 5 EEPROM releases do); EEPROM updates stay a manual step (`rpi-eeprom`) for now.
- Verified so far: the backend logic in tests and the image build. Booting, tryboot and rollback must be verified on a Pi 5.
