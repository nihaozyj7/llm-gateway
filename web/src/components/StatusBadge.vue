<template>
  <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-bold uppercase" :class="badgeClass">
    <span class="w-1 h-1 rounded-full" :class="dotClass"></span>
    {{ text }}
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  text: { type: String, required: true },
  type: { type: String, default: 'success' }, // success | error | info | warning | neutral
})

const map = {
  success: { badge: 'bg-green-500/10 text-green-500', dot: 'bg-green-500' },
  error: { badge: 'bg-red-500/10 text-red-500', dot: 'bg-red-500' },
  info: { badge: 'bg-blue-500/10 text-blue-500', dot: 'bg-blue-500' },
  warning: { badge: 'bg-amber-500/10 text-amber-500', dot: 'bg-amber-500' },
  neutral: { badge: 'bg-neutral-500/10 text-neutral-400', dot: 'bg-neutral-500' },
}

const badgeClass = computed(() => map[props.type]?.badge || map.neutral.badge)
const dotClass = computed(() => map[props.type]?.dot || map.neutral.dot)
</script>
