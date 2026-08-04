<template>
  <div class="glass-card p-6">
    <div class="flex items-center justify-between mb-4">
      <span class="text-xs font-medium text-[#737373] tracking-wider uppercase">{{ label }}</span>
      <Icon :icon="icon" class="text-[#737373]" />
    </div>
    <div class="flex items-baseline gap-2">
      <h3 class="text-4xl font-bold mono-text">{{ value }}</h3>
      <span class="text-xs font-medium font-mono" :class="trendClass">{{ trendText }}</span>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Icon } from '@iconify/vue'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  trendText: { type: String, default: '' },
  trendType: { type: String, default: 'neutral' }, // success | neutral | info | warning | secondary
  icon: { type: String, default: 'lucide:activity' },
})

const trendClass = computed(() => {
  const map = {
    success: 'text-green-500',
    info: 'text-blue-500',
    warning: 'text-amber-500',
    secondary: 'text-[#737373] font-normal',
    neutral: 'text-neutral-500',
  }
  return map[props.trendType] || map.neutral
})
</script>
