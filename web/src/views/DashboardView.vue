<template>
  <div>
    <!-- 页头 -->
    <div class="flex items-center justify-between mb-8">
      <div class="flex items-center gap-3">
        <div class="w-2 h-8 bg-white"></div>
        <h1 class="text-2xl font-bold tracking-tight uppercase">概览</h1>
      </div>
      <button @click="load" class="text-xs btn-primary px-4 py-2 rounded uppercase">刷新</button>
    </div>

    <!-- 顶部统计 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-10">
      <StatCard label="总请求量" :value="fmtNum(summary.request_count)" trendText="ALL TIME" trendType="success" icon="lucide:activity" />
      <StatCard label="成功率" :value="successRate" trendText="STABLE" trendType="neutral" icon="lucide:check-circle" />
      <StatCard label="总 TOKEN 用量" :value="fmtTokens(summary.total_tokens)" trendText="TOKENS" trendType="warning" icon="lucide:cpu" />
      <StatCard label="累计成本" :value="fmtCost(summary.cost)" trendText="RMB" trendType="secondary" icon="lucide:credit-card" />
    </div>

    <!-- 最近请求日志 -->
    <div class="glass-card overflow-hidden">
      <TableHeader title="最近请求日志" show-export @export="exportCSV" @refresh="load" />
      <div class="overflow-x-auto">
        <table class="w-full text-left">
          <thead>
            <tr class="bg-[#1a1a1a] border-b border-[#262626]">
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">时间</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">渠道</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">模型</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">状态</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">耗时</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">TOKENS</th>
              <th class="px-6 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">成本</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <tr v-for="log in logs" :key="log.id" @click="$router.push('/logs')" class="hover:bg-[#1a1a1a] transition-colors cursor-pointer">
              <td class="px-6 py-4 text-xs font-mono text-[#737373]">{{ fmtTimeShort(log.request_time) }}</td>
              <td class="px-6 py-4 text-sm font-medium">{{ log.channel_name || '--' }}</td>
              <td class="px-6 py-4 text-xs font-mono">{{ log.model }}</td>
              <td class="px-6 py-4"><StatusBadge :text="badgeText(log.status)" :type="badgeType(log.status)" /></td>
              <td class="px-6 py-4 text-xs font-mono text-[#a3a3a3]">{{ (log.latency_ms / 1000).toFixed(2) }}s</td>
              <td class="px-6 py-4 text-xs font-mono text-[#a3a3a3]">{{ fmtNum(log.total_tokens) }}</td>
              <td class="px-6 py-4 text-xs font-mono text-white">{{ fmtCost(log.cost) }}</td>
            </tr>
            <tr v-if="!logs.length">
              <td colspan="7" class="px-6 py-10 text-center text-sm text-[#737373]">暂无请求日志,发起一次网关请求后即可看到</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 提醒区 -->
    <div v-if="cooldown.length" class="mt-10 grid grid-cols-1 md:grid-cols-2 gap-6">
      <div v-for="c in cooldown" :key="c.id" class="glass-card p-6 border-l-4 border-amber-500">
        <div class="flex items-start gap-4">
          <div class="p-2 bg-amber-500/10 rounded">
            <Icon icon="lucide:alert-triangle" class="text-amber-500 text-xl" />
          </div>
          <div>
            <h4 class="text-sm font-bold mb-1">渠道冷静提醒</h4>
            <p class="text-xs text-[#a3a3a3] leading-relaxed">
              渠道 <span class="text-white font-mono uppercase">{{ c.name }}</span> 触发失败阈值,已进入冷静期
              ({{ c.last_error || '连续失败' }})。
            </p>
            <button @click="recover(c.id)" class="mt-3 text-[10px] font-bold uppercase tracking-tighter text-amber-500 hover:text-amber-400">手动恢复渠道</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { api, fmtNum, fmtTokens, fmtCost, fmtTimeShort } from '../api'
import StatCard from '../components/StatCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import TableHeader from '../components/TableHeader.vue'

const summary = ref({ request_count: 0, success_count: 0, fail_count: 0, total_tokens: 0, cost: 0 })
const cooldown = ref([])
const logs = ref([])

const successRate = computed(() => {
  if (!summary.value.request_count) return '--'
  return ((summary.value.success_count / summary.value.request_count) * 100).toFixed(1) + '%'
})

function badgeText(s) {
  const map = { success: 'Success', retry_success: 'Retry [1]', fail: 'Failed', biz_error: 'Biz Error' }
  return map[s] || s
}
function badgeType(s) {
  if (s === 'success') return 'success'
  if (s === 'retry_success') return 'info'
  return 'error'
}

async function load() {
  const [d, l] = await Promise.all([api.dashboard(), api.listLogs({ page_size: 6 })])
  summary.value = d.summary
  cooldown.value = d.cooldown || []
  logs.value = l.logs || []
}

async function recover(id) {
  await api.recoverChannel(id)
  load()
}

function exportCSV() {
  const rows = [['时间', '渠道', '模型', '状态', '耗时(ms)', 'Tokens', '成本(元)']]
  for (const l of logs.value) {
    rows.push([l.request_time, l.channel_name, l.model, l.status, l.latency_ms, l.total_tokens, l.cost])
  }
  const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g, '""')}"`).join(',')).join('\n')
  const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'request-logs.csv'
  a.click()
}

onMounted(load)
</script>
