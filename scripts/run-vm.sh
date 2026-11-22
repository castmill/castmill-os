#!/usr/bin/env bash
# Run a Castmill OS development VM on an Apple Silicon Mac (or arm64 Linux).
#
#   scripts/run-vm.sh fetch [run-id]   Download the vm-arm64 image built by GitHub Actions
#   scripts/run-vm.sh run [image]      Boot the image in QEMU (HVF on macOS, KVM on Linux)
#   scripts/run-vm.sh reset            Delete the VM disk and UEFI variables (fresh device)
#
# Requirements (macOS): brew install qemu zstd gh
# See docs/development.md for details.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VM_DIR="${CASTMILL_VM_DIR:-$ROOT/.vm}"
DISK="$VM_DIR/disk.qcow2"
VARS="$VM_DIR/efivars.fd"
MEM="${CASTMILL_VM_MEM:-4096}"
CPUS="${CASTMILL_VM_CPUS:-4}"
OUTPUTS="${CASTMILL_VM_OUTPUTS:-2}"      # number of virtual displays
SSH_PORT="${CASTMILL_VM_SSH_PORT:-2222}" # dev images only

die() { echo "error: $*" >&2; exit 1; }

find_firmware() {
  local candidates=()
  if command -v brew >/dev/null 2>&1; then
    candidates+=("$(brew --prefix qemu)/share/qemu")
  fi
  candidates+=(/usr/share/qemu /usr/share/AAVMF /usr/share/edk2/aarch64)
  for dir in "${candidates[@]}"; do
    for code in edk2-aarch64-code.fd AAVMF_CODE.fd QEMU_EFI.fd; do
      if [[ -f "$dir/$code" ]]; then
        FW_CODE="$dir/$code"
        FW_VARS_TEMPLATE="$dir/edk2-arm-vars.fd"
        [[ -f "$FW_VARS_TEMPLATE" ]] || FW_VARS_TEMPLATE="$dir/AAVMF_VARS.fd"
        return 0
      fi
    done
  done
  die "UEFI firmware for aarch64 not found (install qemu: brew install qemu)"
}

accel() {
  case "$(uname -s)" in
    Darwin) echo "hvf" ;;
    Linux)  [[ -w /dev/kvm ]] && echo "kvm" || echo "tcg" ;;
    *)      echo "tcg" ;;
  esac
}

cmd_fetch() {
  command -v gh >/dev/null || die "GitHub CLI not found (brew install gh)"
  local run_id="${1:-}"
  mkdir -p "$VM_DIR/download"
  if [[ -z "$run_id" ]]; then
    run_id="$(gh run list --workflow build.yml --branch main --status success \
      --limit 1 --json databaseId --jq '.[0].databaseId')"
    [[ -n "$run_id" ]] || die "no successful build.yml run found on main"
  fi
  echo "Downloading vm-arm64 image from workflow run $run_id ..."
  rm -rf "$VM_DIR/download"/*
  gh run download "$run_id" --name image-vm-arm64 --dir "$VM_DIR/download"
  ls -lh "$VM_DIR/download"
}

cmd_run() {
  command -v qemu-system-aarch64 >/dev/null || die "qemu-system-aarch64 not found (brew install qemu)"
  local image="${1:-}"
  if [[ -z "$image" ]]; then
    # shellcheck disable=SC2012  # file names are produced by our own build
    image="$(ls -t "$VM_DIR"/download/*.wic.zst 2>/dev/null | head -1 || true)"
  fi
  find_firmware
  mkdir -p "$VM_DIR"

  if [[ ! -f "$DISK" ]]; then
    [[ -n "$image" && -f "$image" ]] || die "no image found; run '$0 fetch' or pass a .wic.zst path"
    echo "Creating VM disk from $image ..."
    zstd -dc "$image" > "$VM_DIR/disk.raw"
    qemu-img convert -f raw -O qcow2 "$VM_DIR/disk.raw" "$DISK"
    rm -f "$VM_DIR/disk.raw"
    qemu-img resize "$DISK" 16G >/dev/null   # room for A/B slots + data
  fi
  [[ -f "$VARS" ]] || cp "$FW_VARS_TEMPLATE" "$VARS"

  # Optional extra QEMU arguments, e.g. CASTMILL_VM_EXTRA_ARGS="-display cocoa"
  local extra=()
  read -r -a extra <<< "${CASTMILL_VM_EXTRA_ARGS:-}"

  echo "Booting Castmill OS VM (accel=$(accel), ${CPUS} CPUs, ${MEM} MB, ${OUTPUTS} outputs)."
  echo "Serial console on this terminal. Dev images: ssh -p $SSH_PORT root@localhost"

  exec qemu-system-aarch64 \
    -machine virt -accel "$(accel)" -cpu host \
    -smp "$CPUS" -m "$MEM" \
    -drive if=pflash,format=raw,readonly=on,file="$FW_CODE" \
    -drive if=pflash,format=raw,file="$VARS" \
    -drive if=virtio,format=qcow2,file="$DISK",discard=unmap \
    -device virtio-gpu-pci,max_outputs="$OUTPUTS" \
    -device qemu-xhci -device usb-kbd -device usb-tablet \
    -device virtio-rng-pci \
    -device i6300esb -action watchdog=reset \
    -netdev user,id=net0,hostfwd=tcp::"$SSH_PORT"-:22 \
    -device virtio-net-pci,netdev=net0 \
    -serial mon:stdio \
    ${extra[@]+"${extra[@]}"}
}

cmd_reset() {
  rm -f "$DISK" "$VARS"
  echo "VM disk and UEFI variables removed. Next 'run' starts from a fresh image."
}

case "${1:-}" in
  fetch) shift; cmd_fetch "$@" ;;
  run)   shift; cmd_run "$@" ;;
  reset) cmd_reset ;;
  *) sed -n '2,10p' "$0"; exit 1 ;;
esac
