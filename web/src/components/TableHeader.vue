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
      <button v-if="showRefresh" @click="$emit('refresh')" :disabled="busy"
        class="text-xs btn-primary px-3 py-1.5 rounded inline-flex items-center gap-1.5 disabled:opacity-60 disabled:cursor-not-allowed">
        <Icon v-if="busy" icon="lucide:loader-2" class="animate-spin" />
        <Icon v-else-if="saved" icon="lucide:check" class="text-green-400" />
        {{ busy ? busyLabel : saved ? savedLabel : refreshLabel }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { Icon } from '@iconify/vue'

defineProps({
  title: { type: String, default: '' },
  showExport: { type: Boolean, default: false },
  showRefresh: { type: Boolean, default: false },
  exportLabel: { type: String, default: '导出 CSV' },
  refreshLabel: { type: String, default: '刷新' },
  // 保存交互状态:busy 显示 spinner 并禁用,saved 短暂显示勾选
  busy: { type: Boolean, default: false },
  busyLabel: { type: String, default: '保存中…' },
  saved: { type: Boolean, default: false },
  savedLabel: { type: String, default: '已保存' },
})
defineEmits(['export', 'refresh'])
</script>
