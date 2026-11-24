<script setup lang="ts">
import { useData } from 'vitepress'
// The same logo the device prints on the console at boot.
import logo from '../../../layers/meta-castmill/files/branding/logo.txt?raw'

const { theme } = useData()
</script>

<template>
  <figure class="boot" aria-label="Castmill OS boot screen">
    <div class="bar" aria-hidden="true"><span /><span /><span /><em>tty1</em></div>
    <pre class="art">{{ logo.replace(/\n+$/, '') }}</pre>
    <p class="info">
      Castmill OS {{ theme.release }} - raspberrypi5 - slot A<span class="cursor" aria-hidden="true">▍</span>
    </p>
  </figure>
</template>

<style scoped>
.boot {
  margin: 0;
  width: min(100%, 470px);
  border-radius: 14px;
  background: #0d1117;
  border: 1px solid #30363d;
  box-shadow: 0 24px 60px -20px rgba(253, 79, 0, 0.45), 0 2px 8px rgba(0, 0, 0, 0.3);
  overflow: hidden;
  font-family: var(--vp-font-family-mono);
  container-type: inline-size;
}
.bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  background: #161b22;
  border-bottom: 1px solid #30363d;
}
.bar span {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #30363d;
}
.bar em {
  margin-left: auto;
  font-style: normal;
  font-size: 11px;
  color: #8b949e;
}
.art {
  margin: 0;
  padding: 20px 16px 6px;
  /* The logo is 71 columns (about 43em): fit it to the card's width. */
  font-size: min(10.5px, calc((100cqw - 32px) / 43));
  line-height: 1.2;
  font-weight: 600;
  color: #ff8a4c;
  overflow: hidden;
}
.info {
  margin: 0;
  padding: 8px 16px 18px;
  font-size: clamp(9px, 1.6vw, 12px);
  color: #c9d1d9;
}
.cursor {
  color: #ff7a3d;
  animation: blink 1.1s steps(1) infinite;
  /* Own compositor layer: the blink never repaints the card, which made
     browsers re-rasterize the blurred hero glow behind it (a dark box
     flashing above the card). */
  will-change: opacity;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .cursor {
    animation: none;
  }
}
</style>
