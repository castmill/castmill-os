# Virtual machine

The `vm-arm64` image is a **development image** of Castmill OS for virtual machines. Use it to try Castmill OS, test pairing and updates, and develop without hardware.

## Requirements

- An **Apple Silicon Mac** (M1 or newer), or an **arm64 Linux** machine with KVM
- `qemu`, `zstd` and the GitHub CLI (`gh`)
- A clone of the [castmill-os repository](https://github.com/castmill/castmill-os)

```sh
brew install qemu zstd gh          # macOS
```

QEMU runs the image with hardware virtualization (Hypervisor.framework on macOS, KVM on Linux), so it runs at close to native speed. It also provides UEFI firmware, two virtual screens and a hardware watchdog, which Castmill OS uses for its A/B boot and recovery.

## Run it

```sh
scripts/run-vm.sh fetch            # latest vm-arm64 image built from main
scripts/run-vm.sh fetch <run-id>   # ...or from a specific GitHub Actions run
scripts/run-vm.sh run              # boot it
scripts/run-vm.sh reset            # delete the VM disk: a fresh, unpaired device
```

`run` also accepts the path of a downloaded image, for example from a release:

```sh
gh release download v0.1.0 -R castmill/castmill-os -p '*genericarm64*' -D .vm/download
scripts/run-vm.sh run .vm/download/castmill-os-image-genericarm64.rootfs.wic.zst
```

The VM disk lives in `.vm/` and survives restarts, like a real device.

### Options

| Variable | Default | Description |
|---|---|---|
| `CASTMILL_VM_MEM` | `4096` | Memory in MB |
| `CASTMILL_VM_CPUS` | `4` | Virtual CPUs |
| `CASTMILL_VM_OUTPUTS` | `2` | Number of virtual screens |
| `CASTMILL_VM_SSH_PORT` | `2222` | Host port forwarded to the VM's SSH |
| `CASTMILL_VM_DIR` | `.vm` | Where the VM disk and UEFI variables are kept |

```sh
CASTMILL_VM_OUTPUTS=1 CASTMILL_VM_MEM=2048 scripts/run-vm.sh run
```

## Development and production images

| | Development image (VM) | Production images |
|---|---|---|
| SSH | `ssh -p 2222 root@localhost`, empty root password | No SSH, no root login |
| Banner | `DEVELOPMENT IMAGE` on the console and status page | – |
| `[backend]` in `castmill.conf` | Honored: point it at your own Castmill server | Ignored |
| Plain `http` Castmill servers | Allowed | Only `https`, or `http` on loopback |
| Debug tools | Included | – |

## What the VM is good for

- boot, the A/B slots, updates and rollback
- pairing with Castmill and the dashboard features
- several screens, `castmill.conf`, factory reset
- working on the agent and the kiosk shell

It's **not** good for video performance (no hardware decoding), display hotplug, or anything hardware-specific. Use a Raspberry Pi 5 or an Intel mini PC for those.

## Next steps

- [Pair it with Castmill](../setup/pairing.md), or with a [local Castmill server](../development.md#pair-the-device-with-castmill).
- [Test an update and a rollback](../development.md#test-an-update-and-a-rollback).
