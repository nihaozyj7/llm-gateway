<template>
  <div>
    <!-- 顶部统计 -->
    <div class="grid lg:grid-cols-4 gap-6 mb-10">
      <StatCard label="聚合模型总数" :value="models.length" trendText="ALL" trendType="success" icon="lucide:box" />
      <StatCard label="关联渠道" :value="channelCount" trendText="STABLE" trendType="neutral" icon="lucide:network" />
      <StatCard label="配置价格模型" :value="pricedCount" trendText="ACTIVE" trendType="info" icon="lucide:tag" />
      <StatCard label="冷静中渠道" :value="cooldownCount" trendText="WATCH" trendType="warning" icon="lucide:alert-triangle" />
    </div>

    <div class="glass-card mb-8 p-6">
      <div class="flex flex-wrap md:flex-row justify-between gap-4 mb-6">
        <div class="relative flex-1 max-w-md">
          <Icon icon="lucide:search" class="absolute left-3 top-1/2 -translate-y-1/2 text-[#404040]" />
          <input v-model="keyword" placeholder="搜索模型 ID 或 关联渠道..."
            class="w-full py-2 pl-10 pr-4 text-sm rounded input-field focus:border-[#404040]" />
        </div>
        <div class="flex gap-2">
          <button @click="openLink" class="text-xs font-bold px-4 py-2 rounded btn-outline flex items-center gap-1">
            <Icon icon="lucide:link-2" /> 关联渠道
          </button>
          <button @click="syncModal = true" class="text-xs font-bold px-4 py-2 rounded btn-outline flex items-center gap-1">
            <Icon icon="lucide:refresh-cw" /> 从渠道同步模型
          </button>
          <button @click="openCreate" class="text-xs font-bold px-4 py-2 rounded btn-primary flex items-center gap-1">
            <Icon icon="lucide:plus" /> 手动添加模型
          </button>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left">
          <thead>
            <tr class="bg-[#1a1a1a] border-b border-[#262626]">
              <th class="px-4 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">模型 ID</th>
              <th class="px-4 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">可用渠道及优先级 (可拖拽调整)</th>
              <th class="px-4 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">映射规则</th>
              <th class="px-4 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest">状态</th>
              <th class="px-4 py-3 text-[10px] font-bold text-[#737373] uppercase tracking-widest text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <tr v-for="m in filtered" :key="m.id" class="hover:bg-[#161616] transition-colors">
              <td class="px-4 py-4">
                <div class="font-mono text-sm">{{ m.model_id }}</div>
                <div v-if="m.display_name" class="text-[10px] text-[#737373]">{{ m.display_name }}</div>
              </td>
              <td class="px-4 py-4">
                <div class="flex flex-wrap items-center">
                  <template v-for="(ch, i) in m.channels" :key="ch.channel_id">
                    <span
                      draggable="true"
                      @dragstart="onDragStart($event, m, i)"
                      @dragover.prevent
                      @drop="onDrop($event, m, i)"
                      @dragend="onDragEnd"
                      title="拖动可调整优先级"
                      class="px-2 py-1 rounded inline-flex items-center gap-1.5 text-[11px] cursor-grab select-none"
                      :class="i === 0 ? 'bg-[#262626] border border-[#333]' : 'bg-[#1a1a1a] border border-[#262626] opacity-60'">
                      <span>{{ ch.channel_name }}</span>
                      <span v-if="ch.model_priority === 0" class="text-[8px] text-[#737373] border border-[#333] rounded px-1 leading-tight" title="未单独调整,跟随渠道全局优先级">继承</span>
                    </span>
                    <span v-if="i < m.channels.length - 1" class="text-[#737373] select-none">→</span>
                  </template>
                  <span v-if="!m.channels.length" class="text-xs text-[#a3a3a3]">--</span>
                </div>
              </td>
              <td class="px-4 py-4 font-mono text-xs">
                <template v-if="hasMapping(m)">
                  <span v-for="(ch, i) in m.channels.filter(c => c.upstream_model_name && c.upstream_model_name !== m.model_id)" :key="i"
                    class="block">{{ ch.channel_name }} <span class="text-[#737373]">→</span> <span class="text-blue-400">{{ ch.upstream_model_name }}</span></span>
                </template>
                <span v-else class="text-[#a3a3a3] tracking-tight">--</span>
              </td>
              <td class="px-4 py-4">
                <StatusBadge v-if="hasCooldownChannel(m)" text="System Cool Down" type="error" />
                <StatusBadge v-else text="Enabled" type="success" />
              </td>
              <td class="px-4 py-4 text-right">
                <span class="text-xs font-bold uppercase tracking-tighter space-x-4">
                  <button @click="openTest(m)" class="text-green-500 hover:underline">测试</button>
                  <button @click="openLink(m)" class="text-white hover:underline">关联</button>
                  <button @click="removeModel(m)" class="text-red-500 hover:underline">删除</button>
                </span>
              </td>
            </tr>
            <tr v-if="!filtered.length">
              <td colspan="5" class="px-4 py-10 text-center text-sm text-[#737373]">暂无模型,先从渠道同步或手动添加</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 手动添加模型 -->
    <div v-if="createModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-sm p-8 shadow-2xl rounded-sm">
        <h3 class="text-lg font-bold uppercase tracking-wider mb-6">手动添加模型</h3>
        <form @submit.prevent="createModel" class="space-y-4">
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">模型 ID *</label>
            <input v-model="newModel.model_id" required placeholder="gpt-4o" class="w-full px-4 py-3 rounded input-field text-sm font-mono" />
          </div>
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">显示名称(可选)</label>
            <input v-model="newModel.display_name" placeholder="GPT-4o" class="w-full px-4 py-3 rounded input-field text-sm" />
          </div>
          <div class="flex gap-4 pt-4">
            <button type="button" @click="createModal = false" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">取消</button>
            <button type="submit" class="flex-1 py-3 text-sm font-bold rounded btn-primary">添加</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 关联渠道弹窗 -->
    <div v-if="linkModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-lg p-8 shadow-2xl rounded-sm">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-bold uppercase tracking-wider">关联渠道 — {{ linkTarget ? linkTarget.model_id : '选择模型' }}</h3>
          <button @click="linkModal = false" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
        </div>
        <div class="space-y-3 max-h-96 overflow-y-auto pr-1">
          <div v-for="ch in channels" :key="ch.id" class="p-3 bg-[#1a1a1a] border border-[#262626] rounded">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <input type="checkbox" v-model="checkedMap[ch.id]" class="w-4 h-4 accent-white" />
                <span class="text-sm">{{ ch.name }}</span>
                <span class="text-[10px] font-mono text-[#737373]">P{{ ch.priority }}</span>
              </div>
              <span v-if="isLinked(ch.id)" class="text-[10px] text-green-500 uppercase font-bold">已关联</span>
            </div>
            <div v-if="checkedMap[ch.id]" class="mt-2 pl-6">
              <label class="text-[10px] text-[#525252] block mb-1">渠道内模型名(留空=同名透传)</label>
              <input v-model="upstreamMap[ch.id]" placeholder="与网关模型名不同时填写"
                class="w-full px-3 py-2 rounded input-field text-xs font-mono" />
            </div>
          </div>
        </div>
        <div class="flex gap-4 pt-6">
          <button @click="linkModal = false" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">取消</button>
          <button @click="saveLinks" class="flex-1 py-3 text-sm font-bold rounded btn-primary">保存关联</button>
        </div>
      </div>
    </div>

    <!-- 模型测试弹窗 -->
    <div v-if="testModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-lg p-8 shadow-2xl rounded-sm">
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-lg font-bold uppercase tracking-wider">模型测试 — {{ testTarget ? testTarget.model_id : '' }}</h3>
          <button @click="closeTest" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
        </div>
        <p class="text-xs text-[#525252] mb-4">对关联渠道发送最小流式请求,测量可用性、首字延迟(TTFT)与回复速度(token/s)</p>
        <div v-if="testing" class="py-8 text-center text-sm text-[#a3a3a3]">
          <Icon icon="lucide:loader-2" class="animate-spin inline mr-2" />正在测试,请稍候(每个渠道最多约 20 秒)...
        </div>
        <div v-else class="space-y-3 max-h-80 overflow-y-auto pr-1">
          <div v-for="r in testResults" :key="r.channel_id" class="p-3 bg-[#1a1a1a] border border-[#262626] rounded">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Icon :icon="resultIcon(r)" class="text-sm" :class="resultColor(r)" />
                <span class="text-sm font-medium">{{ r.channel_name }}</span>
                <span class="text-[10px] font-mono text-[#737373]">P{{ r.priority }}</span>
                <StatusBadge v-if="r.channel_status === 'cooldown'" text="Cool Down" type="error" />
              </div>
              <span class="text-[10px] uppercase font-bold" :class="r.ok ? 'text-green-500' : 'text-red-500'">
                {{ r.skipped ? 'SKIPPED' : (r.ok ? 'OK' : 'FAIL') }}
              </span>
            </div>
            <div v-if="r.skip_reason" class="mt-1 text-xs text-[#737373]">{{ r.skip_reason }}</div>
            <div v-else-if="r.error" class="mt-1 text-xs text-red-500 font-mono break-all">{{ r.error }}</div>
            <div v-else class="mt-2 grid grid-cols-3 gap-2 text-xs">
              <div>
                <div class="text-[10px] text-[#737373] uppercase font-bold">首字延迟</div>
                <span class="font-mono">{{ r.ttft_ms }} ms</span>
              </div>
              <div>
                <div class="text-[10px] text-[#737373] uppercase font-bold">总耗时</div>
                <span class="font-mono">{{ r.total_ms }} ms</span>
              </div>
              <div>
                <div class="text-[10px] text-[#737373] uppercase font-bold">回复速度</div>
                <span class="font-mono">{{ r.speed_tps ? r.speed_tps.toFixed(1) : '--' }} token/s</span>
                <span v-if="r.tokens_estimated" class="text-[#737373]" title="上游未返回 usage,按输出字符估算">*</span>
              </div>
            </div>
            <div v-if="r.ok && r.completion_tokens" class="mt-1 text-[10px] text-[#737373]">
              输出 tokens: {{ r.completion_tokens }}{{ r.tokens_estimated ? '(估算)' : '' }}
            </div>
          </div>
          <div v-if="!testResults.length" class="py-6 text-center text-sm text-[#737373]">无结果</div>
        </div>
        <div class="flex gap-4 pt-6">
          <button v-if="!testing" @click="closeTest" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">关闭</button>
          <button v-if="!testing" @click="runTest" class="flex-1 py-3 text-sm font-bold rounded btn-primary">重新测试</button>
        </div>
      </div>
    </div>

    <!-- 同步弹窗 -->
    <div v-if="syncModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-sm p-8 shadow-2xl rounded-sm">
        <h3 class="text-lg font-bold uppercase tracking-wider mb-2">从渠道同步模型</h3>
        <p class="text-xs text-[#525252] mb-6">调用渠道的 /models 接口拉取模型列表并自动关联</p>
        <select v-model="syncChannelId" class="w-full px-3 py-3 rounded input-field text-sm">
          <option value="" disabled>选择目标渠道</option>
          <option v-for="ch in channels" :key="ch.id" :value="ch.id">{{ ch.name }}</option>
        </select>
        <div v-if="syncResult" class="mt-4 text-xs font-mono" :class="syncResult.ok ? 'text-green-500' : 'text-red-500'">{{ syncResult.msg }}</div>
        <div class="flex gap-4 pt-6">
          <button @click="syncModal = false" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">取消</button>
          <button @click="doSync" :disabled="!syncChannelId || syncing" class="flex-1 py-3 text-sm font-bold rounded btn-primary disabled:opacity-50">
            {{ syncing ? '同步中...' : '开始同步' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { api } from '../api'
import StatCard from '../components/StatCard.vue'
import StatusBadge from '../components/StatusBadge.vue'

const models = ref([])
const channels = ref([])
const keyword = ref('')
const createModal = ref(false)
const linkModal = ref(false)
const syncModal = ref(false)
const syncChannelId = ref('')
const syncing = ref(false)
const syncResult = ref(null)
const newModel = ref({ model_id: '', display_name: '' })
const linkTarget = ref(null)
const checkedMap = ref({})
const upstreamMap = ref({})
const testModal = ref(false)
const testing = ref(false)
const testTarget = ref(null)
const testResults = ref([])
let testAbort = null
let dragFrom = null

const channelCount = computed(() => new Set(models.value.flatMap(m => m.channels.map(c => c.channel_id))).size)
const pricedCount = computed(() => models.value.filter(m => m.price_input != null || m.price_output != null || m.price_cache_read != null).length)
const cooldownCount = computed(() => channels.value.filter(c => c.status === 'cooldown').length)

const filtered = computed(() => {
  if (!keyword.value) return models.value
  const kw = keyword.value.toLowerCase()
  return models.value.filter(m =>
    m.model_id.toLowerCase().includes(kw) ||
    m.channels.some(c => c.channel_name.toLowerCase().includes(kw))
  )
})

function hasMapping(m) {
  return m.channels.some(c => c.upstream_model_name && c.upstream_model_name !== m.model_id)
}
function hasCooldownChannel(m) {
  return m.channels.some(c => c.status === 'cooldown')
}
function isLinked(channelId) {
  return linkTarget.value?.channels.some(c => c.channel_id === channelId)
}

function openCreate() {
  newModel.value = { model_id: '', display_name: '' }
  createModal.value = true
}
async function createModel() {
  await api.createModel(newModel.value.model_id, newModel.value.display_name)
  createModal.value = false
  load()
}
function openLink(m) {
  linkTarget.value = m
  checkedMap.value = {}
  upstreamMap.value = {}
  for (const ch of m.channels) {
    checkedMap.value[ch.channel_id] = true
    upstreamMap.value[ch.channel_id] = ch.upstream_model_name || ''
  }
  linkModal.value = true
}
async function saveLinks() {
  const target = linkTarget.value
  for (const ch of channels.value) {
    const checked = !!checkedMap.value[ch.id]
    const wasLinked = isLinked(ch.id)
    if (checked && !wasLinked) {
      await api.linkChannel(target.id, ch.id, upstreamMap.value[ch.id] || '')
    } else if (checked && wasLinked) {
      const cur = target.channels.find(c => c.channel_id === ch.id)
      if (cur.upstream_model_name !== (upstreamMap.value[ch.id] || '')) {
        // 更新映射:先删后加
        await api.unlinkChannel(target.id, ch.id)
        await api.linkChannel(target.id, ch.id, upstreamMap.value[ch.id] || '')
      }
    } else if (!checked && wasLinked) {
      await api.unlinkChannel(target.id, ch.id)
    }
  }
  linkModal.value = false
  load()
}
async function removeModel(m) {
  if (!confirm(`删除模型 ${m.model_id}?`)) return
  await api.deleteModel(m.id)
  load()
}
function openTest(m) {
  testTarget.value = m
  testModal.value = true
  runTest()
}
async function runTest() {
  if (!testTarget.value || testing.value) return
  testing.value = true
  testResults.value = []
  if (testAbort) testAbort.abort()
  testAbort = new AbortController()
  try {
    const r = await api.testModel(testTarget.value.model_id, testAbort.signal)
    testResults.value = r.results || []
  } catch (e) {
    if (e.name === 'AbortError') return // 用户取消,静默
    testResults.value = []
    alert('测试失败:' + e.message)
  } finally {
    testing.value = false
    testAbort = null
  }
}
function closeTest() {
  if (testing.value && testAbort) testAbort.abort()
  testModal.value = false
  testTarget.value = null
  testResults.value = []
  testing.value = false
  testAbort = null
}
function resultIcon(r) {
  if (r.skipped) return 'lucide:skip-forward'
  return r.ok ? 'lucide:check-circle-2' : 'lucide:x-circle'
}
function resultColor(r) {
  if (r.skipped) return 'text-[#737373]'
  return r.ok ? 'text-green-500' : 'text-red-500'
}

// 渠道优先级拖拽排序:drop 后按新顺序重排 priority 并保存
function onDragStart(e, m, i) {
  dragFrom = { model: m, index: i }
  e.dataTransfer.effectAllowed = 'move'
}
function onDragEnd() {
  dragFrom = null
}
function onDrop(e, m, i) {
  e.preventDefault()
  if (!dragFrom || dragFrom.model.id !== m.id || dragFrom.index === i) return
  const arr = m.channels
  const [moved] = arr.splice(dragFrom.index, 1)
  arr.splice(i, 0, moved)
  dragFrom = null
  savePriority(m)
}
async function savePriority(m) {
  const items = m.channels.map((c, idx) => ({ id: c.channel_id, priority: idx + 1 }))
  try {
    await api.reorderModelChannels(m.id, items)
    await load()
  } catch (e) {
    alert('保存优先级失败:' + e.message)
    await load()
  }
}
async function doSync() {
  syncing.value = true
  syncResult.value = null
  try {
    const r = await api.syncChannelModels(syncChannelId.value)
    syncResult.value = { ok: true, msg: `同步完成:共 ${r.total} 个模型,新增/关联 ${r.added}` }
    load()
  } catch (e) {
    syncResult.value = { ok: false, msg: e.message }
  } finally {
    syncing.value = false
  }
}
async function load() {
  const [m, c] = await Promise.all([api.listModels(), api.listChannels()])
  models.value = m.models
  channels.value = c.channels
}
onMounted(load)
</script>
