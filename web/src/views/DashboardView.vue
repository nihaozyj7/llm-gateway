<template>
  <div>
    <!-- 页头 -->
    <div class="flex items-center justify-between mb-8">
      <div class="flex items-center gap-3">
        <div class="w-2 h-8 bg-white"></div>
        <h1 class="text-2xl font-bold tracking-tight uppercase">概览</h1>
      </div>
      <div class="flex items-center gap-3">
        <button @click="resetStats" class="text-xs px-4 py-2 rounded uppercase border border-[#262626] hover:bg-[#1a1a1a] text-[#a3a3a3] hover:text-white">
          <Icon icon="lucide:rotate-ccw" class="inline mr-1" />重置统计
        </button>
        <button @click="load" class="text-xs btn-primary px-4 py-2 rounded uppercase">刷新</button>
      </div>
    </div>

    <!-- 顶部统计 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-10">
      <StatCard label="总请求量" :value="fmtNum(summary.request_count)" trendText="ALL TIME" trendType="success" icon="lucide:activity" />
      <StatCard label="成功率" :value="successRate" trendText="STABLE" trendType="neutral" icon="lucide:check-circle" />
      <StatCard label="总 TOKEN 用量" :value="fmtTokens(summary.total_tokens)" trendText="TOKENS" trendType="warning" icon="lucide:cpu" />
      <StatCard label="累计成本" :value="fmtCost(summary.cost)" trendText="RMB" trendType="secondary" icon="lucide:credit-card" />
    </div>

    <!-- 请求日志(完整功能:筛选/分页/详情) -->
    <div class="glass-card overflow-hidden mb-10">
      <div class="px-6 py-5 border-b border-[#262626] flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-sm font-bold tracking-widest uppercase flex items-center gap-2">
          <span class="w-1 h-4 bg-white"></span>
          请求日志
        </h2>
        <div class="bg-[#121212] border border-[#262626] rounded px-3 py-2 flex gap-3 items-center">
          <Icon icon="lucide:calendar" class="text-[#737373]" />
          <span class="font-mono text-xs text-[#a3a3a3]">{{ today }}</span>
        </div>
      </div>

      <!-- 筛选 -->
      <div class="grid md:grid-cols-4 xl:grid-cols-5 gap-4 p-6 border-b border-[#262626] bg-[#0e0e0e]">
        <div>
          <label class="text-[10px] font-bold uppercase tracking-widest mb-2 block text-[#737373]">渠道筛选</label>
          <select v-model="filters.channel_id" @change="applyFilter" class="w-full bg-[#1a1a1a] border-[#262626] text-xs px-3 py-2 rounded focus:border-white input-field">
            <option value="">所有渠道</option>
            <option v-for="c in channels" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </div>
        <div>
          <label class="text-[10px] font-bold uppercase tracking-widest mb-2 block text-[#737373]">模型筛选</label>
          <select v-model="filters.model" @change="applyFilter" class="w-full bg-[#1a1a1a] border-[#262626] text-xs px-3 py-2 rounded focus:border-white input-field">
            <option value="">所有模型</option>
            <option v-for="m in models" :key="m.id" :value="m.model_id">{{ m.model_id }}</option>
          </select>
        </div>
        <div>
          <label class="text-[10px] font-bold uppercase tracking-widest mb-2 block text-[#737373]">状态筛选</label>
          <select v-model="filters.status" @change="applyFilter" class="w-full bg-[#1a1a1a] border-[#262626] text-xs px-3 py-2 rounded focus:border-white input-field">
            <option value="">所有状态</option>
            <option value="success">Success</option>
            <option value="retry_success">Retry</option>
            <option value="fail">Error</option>
            <option value="biz_error">Biz Error</option>
          </select>
        </div>
        <div>
          <label class="text-[10px] font-bold uppercase tracking-widest mb-2 block text-[#737373]">密钥筛选</label>
          <select v-model="filters.key_name" @change="applyFilter" class="w-full bg-[#1a1a1a] border-[#262626] text-xs px-3 py-2 rounded focus:border-white input-field">
            <option value="">所有密钥</option>
            <option v-for="k in keys" :key="k.id" :value="k.name">{{ k.name }}</option>
          </select>
        </div>
        <div>
          <label class="text-[10px] font-bold uppercase tracking-widest mb-2 block text-[#737373]">搜索关键词</label>
          <div class="relative">
            <Icon icon="lucide:search" class="absolute left-3 top-1/2 -translate-y-1/2 text-[#404040]" />
            <input v-model="filters.keyword" placeholder="IP, Request ID..."
              class="w-full bg-[#1a1a1a] border-[#262626] text-xs px-3 py-2 rounded pl-9 input-field" @keyup.enter="applyFilter" />
          </div>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left">
          <thead>
            <tr class="bg-[#1a1a1a] border-b border-[#262626]">
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">时间戳 / ID</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">源 IP</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">渠道 / 模型</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">状态</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest" title="请求发起 → 结束的总耗时">耗时</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest" title="P = Prompt: 输入(提示词)消耗的 tokens; C = Completion: 输出(生成内容)消耗的 tokens; T = Total: 输入+输出总计。蓝色数字为命中缓存的输入 tokens">TOKENS (P输入 / C输出 / T总计)</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest" title="输出 token 速度 = 输出 tokens(Completion) ÷ 输出耗时。输出耗时指从请求第一次被返回到结束的时间">Token 速度</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">预估成本</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <template v-for="log in logs" :key="log.id">
              <tr class="hover:bg-[#1a1a1a] transition-colors cursor-pointer" @click="toggleExpand(log.id)">
                <td class="p-4">
                  <div class="font-mono text-xs">{{ fmtTimeShort(log.request_time) }}</div>
                  <div class="text-[10px] opacity-50 font-mono">{{ log.request_id }}</div>
                </td>
                <td class="p-4 font-mono text-xs text-[#a3a3a3]">{{ log.source_ip }}</td>
                <td class="p-4">
                  <div class="text-xs font-medium uppercase tracking-tighter">{{ log.channel_name || '--' }}</div>
                  <div class="text-[11px] font-mono text-[#a3a3a3]">
                    <template v-if="log.upstream_model && log.upstream_model !== log.model">{{ log.model }} <span class="text-[#737373]">→</span> <span class="text-blue-400">{{ log.upstream_model }}</span></template>
                    <template v-else>{{ log.model }}</template>
                  </div>
                </td>
                <td class="p-4"><StatusBadge :text="badgeText(log.status)" :type="badgeType(log.status)" /></td>
                <td class="p-4 font-mono text-xs">{{ (log.latency_ms / 1000).toFixed(2) }}s</td>
                <td class="p-4 font-mono text-xs">
                  {{ fmtNum(log.prompt_tokens) }} / {{ fmtNum(log.completion_tokens) }} / <span class="text-white">{{ fmtNum(log.total_tokens) }}</span>
                  <span v-if="log.cache_read_tokens" class="text-[10px] text-blue-400" title="命中缓存读取的 tokens"> · Cache {{ fmtNum(log.cache_read_tokens) }}</span>
                </td>
                <td class="p-4 font-mono text-xs text-[#a3a3a3]" :title="tokenSpeedDetail(log)">{{ tokenSpeed(log) }}</td>
                <td class="p-4 font-mono text-xs text-white uppercase">{{ fmtCost(log.cost) }}</td>
                <td class="p-4">
                  <button @click.stop="toggleExpand(log.id)" class="text-xs underline underline-offset-4 decoration-[#404040] hover:text-white">
                    {{ expanded === log.id ? '收起' : '详情' }}
                  </button>
                </td>
              </tr>
              <!-- 展开行 -->
              <tr v-if="expanded === log.id" class="bg-[#0d0d0d]">
                <td colspan="9" class="p-8">
                  <div class="grid md:grid-cols-2 gap-8">
                    <div>
                      <div class="flex items-center justify-between mb-2">
                        <span class="text-[10px] uppercase tracking-widest text-[#737373] font-bold">Request Payload (JSON)</span>
                        <button @click="copy(log.payload_request)" class="text-[10px] text-[#a3a3a3] hover:text-white">复制</button>
                      </div>
                      <div class="json-block max-h-72 overflow-y-auto">{{ prettyJSON(log.payload_request) }}</div>
                    </div>
                    <div>
                      <div class="flex items-center justify-between mb-2">
                        <span class="text-[10px] uppercase tracking-widest text-[#737373] font-bold">Response Payload (JSON)</span>
                        <button @click="copy(log.payload_response)" class="text-[10px] text-[#a3a3a3] hover:text-white">复制</button>
                      </div>
                      <div class="json-block max-h-72 overflow-y-auto">{{ prettyJSON(log.payload_response) }}</div>
                    </div>
                  </div>
                  <div v-if="log.error" class="mt-4 text-xs text-red-500 bg-red-500/5 border border-red-500/20 rounded p-3 font-mono break-all">
                    错误信息:{{ log.error }}
                  </div>
                </td>
              </tr>
            </template>
            <tr v-if="!logs.length">
              <td colspan="9" class="p-10 text-center text-sm text-[#737373]">暂无日志记录</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页 -->
      <div class="px-6 py-4 border-t border-[#262626] bg-[#0e0e0e] flex items-center justify-between">
        <div class="flex items-center gap-4">
          <span class="text-xs font-bold uppercase tracking-widest">Page {{ page }} of {{ totalPages }}</span>
          <span class="text-xs opacity-50">Total {{ fmtNum(total) }} Entries</span>
        </div>
        <div class="flex gap-2">
          <button @click="changePage(page - 1)" :disabled="page <= 1"
            class="w-8 h-8 flex items-center justify-center rounded border border-[#262626] bg-[#1a1a1a] text-[#a3a3a3] hover:text-white disabled:opacity-30">
            <Icon icon="lucide:chevron-left" />
          </button>
          <button @click="changePage(page + 1)" :disabled="page >= totalPages"
            class="w-8 h-8 flex items-center justify-center rounded border border-[#262626] bg-[#1a1a1a] text-[#a3a3a3] hover:text-white disabled:opacity-30">
            <Icon icon="lucide:chevron-right" />
          </button>
        </div>
      </div>
    </div>

    <!-- 提醒区 -->
    <div v-if="cooldown.length" class="grid grid-cols-1 md:grid-cols-2 gap-6">
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

const summary = ref({ request_count: 0, success_count: 0, fail_count: 0, total_tokens: 0, cost: 0 })
const cooldown = ref([])
const logs = ref([])
const channels = ref([])
const models = ref([])
const keys = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const expanded = ref(null)
const filters = ref({ channel_id: '', model: '', status: '', key_name: '', keyword: '' })

const today = new Date().toLocaleDateString('zh-CN')
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
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
  if (s === 'biz_error') return 'warning'
  return 'error'
}
function toggleExpand(id) {
  expanded.value = expanded.value === id ? null : id
}
// 输出 token 速度:输出 tokens ÷ 输出耗时(从首次响应到结束,单位:tok/s)
function tokenSpeed(log) {
  if (!log || !log.completion_tokens || log.first_response_ms <= 0 || log.latency_ms <= 0) return '--'
  const outputMs = log.latency_ms - log.first_response_ms
  if (outputMs <= 0) return '--'
  return fmtNum(Math.round((log.completion_tokens * 1000) / outputMs)) + ' tok/s'
}
function tokenSpeedDetail(log) {
  if (!log || log.first_response_ms <= 0) return ''
  const outputMs = Math.max(0, log.latency_ms - log.first_response_ms)
  return `输出 ${fmtNum(log.completion_tokens)} tokens ÷ ${outputMs}ms ≈ ${tokenSpeed(log)}`
}
function prettyJSON(s) {
  if (!s) return '--'
  try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s }
}
async function copy(text) {
  try {
    await navigator.clipboard.writeText(text || '')
  } catch {
    alert('复制失败')
  }
}

async function loadLogs() {
  const params = { page: page.value, page_size: pageSize }
  if (filters.value.channel_id) params.channel_id = filters.value.channel_id
  if (filters.value.model) params.model = filters.value.model
  if (filters.value.status) params.status = filters.value.status
  if (filters.value.key_name) params.key_name = filters.value.key_name
  if (filters.value.keyword) params.keyword = filters.value.keyword
  const r = await api.listLogs(params)
  logs.value = r.logs
  total.value = r.total
}
function applyFilter() {
  page.value = 1
  loadLogs()
}
function changePage(p) {
  if (p < 1) return
  page.value = p
  loadLogs()
}

async function load() {
  const [d, c, m, k] = await Promise.all([api.dashboard(), api.listChannels(), api.listModels(), api.listKeys()])
  summary.value = d.summary
  cooldown.value = d.cooldown || []
  channels.value = c.channels
  models.value = m.models
  keys.value = k.keys || []
  await loadLogs()
}

async function resetStats() {
  if (!confirm('重置将清空全部统计数字(总请求量 / 成功率 / TOKEN / 成本归零),请求日志保留。确定继续?')) return
  await api.resetStats()
  await load()
}

async function recover(id) {
  await api.recoverChannel(id)
  load()
}

onMounted(load)
</script>
