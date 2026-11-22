---
layout: home
title: Castmill OS
titleTemplate: Immutable Linux for digital signage

hero:
  name: Castmill OS
  text: The operating system your screens deserve
  tagline: Open source, immutable and self-healing Linux for digital signage. One signed image that updates itself safely and never needs a visit.
  actions:
    - theme: brand
      text: Get started
      link: /guide/introduction
    - theme: alt
      text: Try it on a Mac
      link: /guide/quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/castmill/castmill-os

features:
  - icon: 🧊
    title: Immutable and atomic
    details: A read-only system image. Updates are written to an inactive A/B slot and switched to in one step, so a device always runs a known, tested image.
    link: /operate/updates
    linkText: Updates and rollback
  - icon: 🩺
    title: Repairs itself
    details: Health-checked boots, automatic rollback, watchdogs at every layer and storage that survives power cuts. No one needs to go on site.
    link: /guide/how-it-works#self-healing
    linkText: How it heals
  - icon: 🔒
    title: Secure by default
    details: Signed updates, no inbound ports, sandboxed services and a locked-down kiosk shell. No shell, package manager or desktop on production devices.
    link: /specs/04-security
    linkText: Security design
  - icon: 🖥️
    title: One player per screen
    details: Every output runs its own Castmill player with independent content. Displays are detected and started on hotplug.
    link: /guide/how-it-works
    linkText: Architecture
  - icon: 🔗
    title: Castmill native
    details: Pair with a pincode or QR code like any Castmill player. The dashboard gets OS info, telemetry, restart and reboot through a native bridge.
    link: /setup/pairing
    linkText: Pair a device
  - icon: 🪶
    title: Small and focused
    details: Built with Yocto for one job. No X server, no LLVM, no package manager. Intel mini PCs and Raspberry Pi 5 are supported first.
    link: /install/
    linkText: Supported hardware
---

<div class="home-status">

::: warning Early days: v0.1
Castmill OS is at its first test release. Images are signed with **development keys** and aren't ready for screens in production yet. See the [implementation status](./implementation-status.md) and the [roadmap](./specs/11-roadmap.md).
:::

</div>

<style>
.home-status {
  max-width: 1152px;
  margin: 48px auto 0;
}
</style>
