<template>
  <div class="px-6 py-5 border-b border-[#262626] flex flex-wrap items-center justify-between gap-3">
    <h2 class="text-sm font-bold tracking-widest uppercase flex items-center gap-2">
      <span class="w-1 h-4 bg-white"></span>
      {{ title }}
    </h2>
    <div class="flex gap-4">
      <button v-if="showExport" @click="$emit('export')" class="text-xs btn-outline px-3 py-1.5 rounded">
        {{ exportLabel }}
      </button>
      <button v-if="showRefresh" @click="$emit('refresh')" class="text-xs btn-primary px-3 py-1.5 rounded">
        {{ refreshLabel }}
      </button>
    </div>
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  showExport: { type: Boolean, default: false },
  showRefresh: { type: Boolean, default: false },
  exportLabel: { type: String, default: '导出 CSV' },
  refreshLabel: { type: String, default: '刷新' },
})
defineEmits(['export', 'refresh'])
</script>
