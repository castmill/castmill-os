# Quick start on a Mac

The fastest way to see Castmill OS is the development virtual machine. It runs the arm64 image at close to native speed on any Apple Silicon Mac.

::: tip Time needed
About five minutes, plus the image download.
:::

## 1. Install the tools

You need [Homebrew](https://brew.sh) and a clone of the repository:

```sh
brew install qemu zstd gh
gh auth login                      # once, to download CI artifacts
git clone https://github.com/castmill/castmill-os.git
cd castmill-os
```

## 2. Download the image

Get the latest development image built from `main`:

```sh
scripts/run-vm.sh fetch
```

Or download the one from a release:

```sh
gh release download v0.1.0 -R castmill/castmill-os -p '*genericarm64*' -D .vm/download
```

## 3. Boot it

```sh
scripts/run-vm.sh run                    # the image from "fetch"
scripts/run-vm.sh run .vm/download/castmill-os-image-genericarm64.rootfs.wic.zst
```

A QEMU window opens with **two virtual screens**, and the serial console stays in your terminal. You'll see:

1. The Castmill OS logo with the version and the active slot:

   ```text
     \     /                       __            _ ____   ____  _____
      \ _ /       _________ ______/ /_____ ___  (_) / /  / __ \/ ___/
       (o)       / ___/ __ `/ ___/ __/ __ `__ \/ / / /  / / / /\__ \
      /| |\     / /__/ /_/ (__  ) /_/ / / / / / / / /  / /_/ /___/ /
     / | | \    \___/\__,_/____/\__/_/ /_/ /_/_/_/_/   \____//____/
      /___\

     Castmill OS 0.1.0 - genericarm64 - slot A - DEVELOPMENT IMAGE
   ```

2. Castmill's **pairing screen** on each virtual screen, with a pincode and a QR code.

## 4. Pair it

Open the Castmill dashboard, add a device and enter the pincode shown on a screen. Each screen is its own Castmill device. See [Pair with Castmill](../setup/pairing.md).

## Useful commands

```sh
ssh -p 2222 root@localhost         # development images only: root, empty password
castmill-agent status              # on the device: agent, outputs and update state
scripts/run-vm.sh reset            # start again from a fresh, unpaired device
```

The VM is good for checking boot, pairing, updates and rollbacks. It has no hardware video decoding, so test media performance on real hardware. More in [Virtual machine](../install/virtual-machine.md).
