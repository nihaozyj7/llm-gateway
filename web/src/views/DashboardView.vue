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
        <button @click="clearLogs" class="text-xs px-4 py-2 rounded uppercase border border-[#262626] hover:bg-[#1a1a1a] text-[#a3a3a3] hover:text-white">
          <Icon icon="lucide:trash-2" class="inline mr-1" />清除日志
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
            <option value="success">成功</option>
            <option value="retry_success">重试成功</option>
            <option value="fail">失败</option>
            <option value="biz_error">业务错误</option>
            <option value="canceled">客户端断开</option>
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
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest" title="输出 token 速度 = 输出 tokens(Completion,含思考) ÷ 请求总耗时">Token 速度</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">预估成本</th>
              <th class="p-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <template v-for="log in logs" :key="log.id">
              <tr class="hover:bg-[#1a1a1a] transition-colors cursor-pointer" @click="openDetail(log)">
                <td class="p-4">
                  <div class="flex items-center gap-2">
                    <!-- 流式/非流式标记 -->
                    <span v-if="log.is_stream" class="text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20" title="流式请求(SSE)">流</span>
                    <span v-else class="text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-[#262626] text-[#a3a3a3] border border-[#404040]" title="非流式请求">非流</span>
                  </div>
                  <div class="font-mono text-xs mt-1">{{ fmtTimeShort(log.request_time) }}</div>
                  <div class="text-[10px] opacity-50 font-mono">{{ log.request_id }}</div>
                </td>
                <td class="p-4 font-mono text-xs text-[#a3a3a3]">{{ log.source_ip }}</td>
                <td class="p-4">
                  <div class="text-xs font-medium uppercase tracking-tighter">
                    <!-- 渠道尝试链路:渠道1(失败)→渠道2(成功),每个渠道按自身结果着色 -->
                    <template v-if="channelTrail(log).length">
                      <template v-for="(t, i) in channelTrail(log)" :key="i">
                        <span v-if="i" class="text-[#525252]">→</span>
                        <span :class="trailCls(t)" :title="t.reason || t.channel_name">{{ t.channel_name }}</span>
                      </template>
                    </template>
                    <!-- 旧日志(无链路字段)回退:失败展示 渠道 → 模型,其余展示渠道名 -->
                    <template v-else-if="log.status === 'fail'">{{ log.channel_name ? log.channel_name + ' → ' + log.model : '--' }}</template>
                    <template v-else>{{ log.channel_name || '--' }}</template>
                  </div>
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
                  <button @click.stop="openDetail(log)" class="text-xs underline underline-offset-4 decoration-[#404040] hover:text-white">详情</button>
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

    <!-- 日志详情弹出层 -->
    <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center p-6" @click.self="detail = null">
      <div class="absolute inset-0 bg-black/70 backdrop-blur-sm"></div>
      <div class="relative w-full max-w-4xl max-h-[88vh] flex flex-col bg-[#121212] border border-[#262626] rounded-xl shadow-2xl overflow-hidden">
        <!-- 弹层头部 -->
        <div class="flex items-center justify-between px-6 py-4 border-b border-[#262626] bg-[#0e0e0e] shrink-0">
          <div class="flex items-center gap-3 min-w-0">
            <span v-if="detail.is_stream" class="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">流</span>
            <span v-else class="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-[#262626] text-[#a3a3a3] border border-[#404040]">非流</span>
            <span class="text-sm font-bold font-mono">{{ detail.request_id }}</span>
            <StatusBadge :text="badgeText(detail.status)" :type="badgeType(detail.status)" />
          </div>
          <div class="flex items-center gap-3 shrink-0">
            <span class="text-[10px] text-[#737373] font-mono">{{ fmtTimeShort(detail.request_time) }} · {{ detail.source_ip }}</span>
            <button @click="detail = null" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
          </div>
        </div>
        <!-- 弹层主体:元信息 + 请求/响应体 -->
        <div class="flex-1 overflow-y-auto p-6 space-y-5">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
            <div class="bg-[#0d0d0d] border border-[#262626] rounded p-3">
              <div class="text-[10px] uppercase tracking-widest text-[#737373] font-bold mb-1">渠道</div>
              <div class="font-mono break-all">{{ detail.channel_name || '--' }}</div>
            </div>
            <div class="bg-[#0d0d0d] border border-[#262626] rounded p-3">
              <div class="text-[10px] uppercase tracking-widest text-[#737373] font-bold mb-1">模型</div>
              <div class="font-mono break-all">
                <template v-if="detail.upstream_model && detail.upstream_model !== detail.model">{{ detail.model }} → {{ detail.upstream_model }}</template>
                <template v-else>{{ detail.model }}</template>
              </div>
            </div>
            <div class="bg-[#0d0d0d] border border-[#262626] rounded p-3">
              <div class="text-[10px] uppercase tracking-widest text-[#737373] font-bold mb-1">耗时</div>
              <div class="font-mono">{{ (detail.latency_ms / 1000).toFixed(2) }}s<template v-if="detail.first_response_ms">(首包 {{ (detail.first_response_ms / 1000).toFixed(2) }}s)</template></div>
            </div>
            <div class="bg-[#0d0d0d] border border-[#262626] rounded p-3">
              <div class="text-[10px] uppercase tracking-widest text-[#737373] font-bold mb-1">TOKENS</div>
              <div class="font-mono">{{ fmtNum(detail.prompt_tokens) }} / {{ fmtNum(detail.completion_tokens) }} / {{ fmtNum(detail.total_tokens) }}</div>
            </div>
          </div>

          <div v-if="detail.error" class="text-xs text-red-500 bg-red-500/5 border border-red-500/20 rounded p-3 break-all">
            <div class="font-bold mb-1">错误信息</div>
            <div class="whitespace-pre-line">{{ errorText(detail) }}</div>
          </div>

          <!-- 请求体(格式化) -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <span class="text-[10px] uppercase tracking-widest text-[#737373] font-bold">Request Payload (JSON)</span>
              <button @click="copy(detail.payload_request)" class="text-[10px] text-[#a3a3a3] hover:text-white">复制</button>
            </div>
            <pre class="json-block max-h-80 overflow-y-auto">{{ prettyJSON(detail.payload_request) }}</pre>
          </div>
          <!-- 响应体 -->
          <div v-if="detail.payload_response">
            <div class="flex items-center justify-between mb-2">
              <span class="text-[10px] uppercase tracking-widest text-[#737373] font-bold">Response Payload (JSON)</span>
              <button @click="copy(detail.payload_response)" class="text-[10px] text-[#a3a3a3] hover:text-white">复制</button>
            </div>
            <pre class="json-block max-h-80 overflow-y-auto">{{ prettyJSON(detail.payload_response) }}</pre>
          </div>
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
const detail = ref(null) // 日志详情弹出层当前展示的日志
const filters = ref({ channel_id: '', model: '', status: '', key_name: '', keyword: '' })

const today = new Date().toLocaleDateString('zh-CN')
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const successRate = computed(() => {
  if (!summary.value.request_count) return '--'
  return ((summary.value.success_count / summary.value.request_count) * 100).toFixed(1) + '%'
})

function badgeText(s) {
  const map = { success: '成功', retry_success: '重试成功', fail: '失败', biz_error: '业务错误', canceled: '客户端断开' }
  return map[s] || s
}
// 错误信息中文化:解析后端 error 字符串,输出中文可读提示
function errorText(log) {
  if (!log || !log.error) return ''
  const raw = log.error
  // 所有渠道失败:all channels failed:渠道名(原因); 渠道名(原因)
  if (raw.includes('all channels failed')) {
    const parts = raw.split(':').slice(1).join(':').split('; ').filter(Boolean)
    if (parts.length) {
      const detail = parts.map(p => {
        const m = p.match(/^(.+)\((.+)\)$/)
        if (m) return `${m[1]}: ${translateErr(m[2])}`
        return translateErr(p)
      }).join('\n')
      return '所有渠道均失败:\n' + detail
    }
    return '所有渠道均失败'
  }
  return translateErr(raw)
}
// 常见上游错误英文 → 中文
function translateErr(e) {
  const lower = (e || '').toLowerCase()
  if (lower.includes('context deadline exceeded')) return '上游请求超时(超过渠道/全局超时限制)'
  if (lower.includes('context canceled')) return '客户端断开连接(请求已取消)'
  if (lower.includes('client canceled') || lower.includes('request canceled')) return '客户端断开连接(请求已取消)'
  if (lower.includes('connection refused')) return '连接被拒绝(上游不可达)'
  if (lower.includes('no such host')) return '域名解析失败(上游地址错误)'
  if (lower.includes('tls')) return 'TLS 握手失败(证书/协议问题)'
  if (lower.includes('timeout')) return '请求超时'
  if (lower.includes('no available channel')) return '无可用渠道(模型未配置渠道或全部冷却中)'
  if (lower.includes('upstream response too large')) return '上游响应体超限(>64MB)'
  return e
}
function badgeType(s) {
  if (s === 'success') return 'success'
  if (s === 'retry_success') return 'info'
  if (s === 'biz_error') return 'warning'
  if (s === 'canceled') return 'info'
  return 'error'
}
// 渠道尝试链路:解析 channel_trail JSON;旧日志无该字段时返回空数组(前端回退旧显示)
function channelTrail(log) {
  if (!log || !log.channel_trail) return []
  try {
    const arr = JSON.parse(log.channel_trail)
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}
// 链路渠道样式:正常响应不染色,故障红色,客户端取消灰色(非渠道故障)
function trailCls(t) {
  if (t.ok) return ''
  if (t.reason === 'client canceled' || t.reason === 'request canceled') return 'text-gray-500'
  return 'text-red-500'
}
function openDetail(log) {
  detail.value = log
}
// 输出 token 速度:输出 tokens(usage,含思考过程)÷ 请求总耗时(单位:tok/s)。
// 注意:不能减 first_response_ms——非流式请求首包与完成几乎同时,分母趋近 0 会算出天文数字;
// 且 usage 是模型结束后返回的本轮总量,直接除以总耗时即为平均输出速度。
// 非流式请求一次性返回,输出速度无意义,不显示。
function tokenSpeed(log) {
  if (!log || !log.is_stream || !log.completion_tokens || log.latency_ms <= 0) return '--'
  return fmtNum(Math.round((log.completion_tokens * 1000) / log.latency_ms)) + ' tok/s'
}
function tokenSpeedDetail(log) {
  if (!log || log.latency_ms <= 0) return ''
  return `输出 ${fmtNum(log.completion_tokens)} tokens ÷ ${log.latency_ms}ms ≈ ${tokenSpeed(log)}`
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

async function clearLogs() {
  if (!confirm('清除将删除全部请求日志记录,统计数字保留。此操作不可恢复,确定继续?')) return
  await api.clearLogs()
  page.value = 1
  await load()
}

async function recover(id) {
  await api.recoverChannel(id)
  load()
}

onMounted(load)
</script>
