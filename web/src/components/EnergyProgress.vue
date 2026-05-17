<template>
  <div
    class="energy-progress"
    :class="{ 'energy-high': percent >= 80, 'energy-critical': percent >= 95 }"
    :title="`上下文能量：${percent.toFixed(1)}%（阈值 ${thresholdPercent}%）`"
  >
    <div class="energy-icon">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
        <path
          d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"
          :stroke="strokeColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          :fill="percent > 0 ? strokeColor : 'none'"
          fill-opacity="0.3"
        />
      </svg>
    </div>
    <div class="energy-bar-wrapper">
      <div class="energy-bar-track">
        <div
          class="energy-bar-fill"
          :style="{ width: percent + '%', background: gradientColor }"
        ></div>
        <div
          v-if="percent >= 70"
          class="energy-bar-glow"
          :style="{ width: percent + '%', background: glowColor }"
        ></div>
      </div>
    </div>
    <span class="energy-percent">{{ percent.toFixed(0) }}%</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  percent: number
  thresholdPercent: number
}>()

const strokeColor = computed(() => {
  const p = props.percent
  if (p >= 90) return '#ef4444'
  if (p >= 70) return '#f59e0b'
  if (p >= 50) return '#eab308'
  return '#22c55e'
})

const gradientColor = computed(() => {
  const p = props.percent
  if (p >= 90) return 'linear-gradient(90deg, #f59e0b, #ef4444)'
  if (p >= 70) return 'linear-gradient(90deg, #eab308, #f59e0b)'
  if (p >= 50) return 'linear-gradient(90deg, #22c55e, #eab308)'
  return 'linear-gradient(90deg, #10b981, #22c55e)'
})

const glowColor = computed(() => {
  if (props.percent >= 90) return 'rgba(239, 68, 68, 0.4)'
  if (props.percent >= 70) return 'rgba(245, 158, 11, 0.3)'
  return 'rgba(234, 179, 8, 0.2)'
})
</script>

<style scoped>
.energy-progress {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 8px;
  border-radius: 12px;
  background: var(--accent-soft);
  transition: all 0.3s ease;
  cursor: default;
  user-select: none;
}

.energy-progress:hover {
  background: var(--bg-tertiary, rgba(255, 255, 255, 0.08));
}

.energy-icon {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.energy-bar-wrapper {
  width: 60px;
  display: flex;
  align-items: center;
}

.energy-bar-track {
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: var(--border);
  position: relative;
  overflow: hidden;
}

.energy-bar-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.6s cubic-bezier(0.25, 0.8, 0.25, 1);
  position: relative;
  z-index: 1;
}

.energy-bar-glow {
  position: absolute;
  top: -2px;
  left: 0;
  height: 10px;
  border-radius: 5px;
  filter: blur(4px);
  opacity: 0.6;
  z-index: 0;
  animation: energy-pulse 2s ease-in-out infinite;
}

.energy-percent {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  min-width: 30px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.energy-high .energy-percent {
  color: #f59e0b;
}

.energy-critical .energy-percent {
  color: #ef4444;
  animation: energy-blink 1s ease-in-out infinite;
}

@keyframes energy-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

@keyframes energy-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
