<template>
  <div>
    <!-- 页头 -->
    <div class="mb-10">
      <div class="flex items-center gap-3">
        <div class="w-2 h-8 bg-white"></div>
        <h1 class="text-2xl font-bold tracking-tight uppercase">价格设置</h1>
      </div>
      <p class="text-sm text-[#737373] mt-2 max-w-2xl">配置聚合模型单价(元/千 Token),用于日志成本估算。同名模型全局统一定价,不填则成本计为 0。</p>
    </div>

    <!-- 定价表 -->
    <div class="glass-card overflow-hidden mb-8">
      <TableHeader title="全局模型定价表" show-refresh refresh-label="保存修改" @refresh="saveAll" />
      <div class="p-6 border-b bg-[#0e0e0e] flex flex-wrap gap-4 justify-between items-center">
        <div class="relative">
          <Icon icon="lucide:search" class="absolute left-3 top-1/2 -translate-y-1/2 text-[#404040]" />
          <input v-model="keyword" placeholder="搜索模型 ID..."
            class="bg-[#1a1a1a] border-[#262626] rounded py-1.5 pl-9 pr-4 text-xs input-field" />
        </div>
        <div class="flex items-center gap-4">
          <select v-model="series" class="bg-[#1a1a1a] border-[#262626] rounded px-3 py-1.5 text-xs input-field">
            <option value="">所有系列</option>
            <option value="gpt">GPT Series</option>
            <option value="claude">Claude Series</option>
            <option value="deepseek">DeepSeek Series</option>
          </select>
          <span class="text-[10px] font-bold uppercase text-[#737373]">* 汇率参考: 1 USD ≈ 7.24 CNY</span>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-left">
          <thead>
            <tr class="bg-[#1a1a1a] border-b border-[#262626]">
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">模型 ID</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">关联渠道数</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">输入单价 (元/1K)</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">输出单价 (元/1K)</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">计费状态</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <tr v-for="m in filtered" :key="m.id" class="hover:bg-[#1a1a1a] transition-colors">
              <td class="px-6 py-4 font-mono text-sm">{{ m.model_id }}</td>
              <td class="px-6 py-4 text-xs text-[#737373]">{{ m.channels.length }} Channels</td>
              <td class="px-6 py-4">
                <input v-model="prices[m.id].input" type="number" step="0.001" min="0" placeholder="0.000"
                  class="price-input w-24 px-2 py-1 text-xs mono-text" />
              </td>
              <td class="px-6 py-4">
                <input v-model="prices[m.id].output" type="number" step="0.001" min="0" placeholder="0.000"
                  class="price-input w-24 px-2 py-1 text-xs mono-text" />
              </td>
              <td class="px-6 py-4">
                <StatusBadge v-if="m.price_input != null || m.price_output != null" text="Active" type="success" />
                <StatusBadge v-else text="Free Tier?" type="info" />
              </td>
              <td class="px-6 py-4 text-right">
                <button @click="resetPrice(m)" class="text-[10px] font-bold uppercase tracking-widest text-[#a3a3a3] hover:text-white">重置</button>
              </td>
            </tr>
            <tr v-if="!filtered.length">
              <td colspan="6" class="px-6 py-10 text-center text-sm text-[#737373]">暂无模型,请先到模型管理页添加或同步</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="px-6 py-3 border-t border-[#262626] bg-[#0e0e0e] text-xs text-[#737373]">
        {{ filtered.length }} 个模型 · 修改后点击右上角「保存修改」生效
      </div>
    </div>

    <div class="grid lg:grid-cols-2 gap-6">
      <!-- 批量定价操作 -->
      <div class="glass-card overflow-hidden">
        <TableHeader title="批量定价操作" />
        <div class="p-6 space-y-6">
          <div>
            <label class="text-[10px] font-bold uppercase tracking-widest block mb-2 text-[#737373]">按渠道快速应用单价</label>
            <div class="flex gap-2">
              <select v-model="batchChannel" class="flex-1 bg-[#1a1a1a] border-[#262626] rounded px-3 py-2 text-xs input-field">
                <option value="" disabled>选择目标渠道...</option>
                <option v-for="c in channels" :key="c.id" :value="c.id">{{ c.name }}</option>
              </select>
              <button @click="importFromChannel" class="px-6 py-2 rounded text-xs uppercase btn-primary">拉取官方价</button>
            </div>
            <p class="text-[10px] text-[#525252] mt-2">警告: 该操作将覆盖此渠道下所有模型的当前单价设置。</p>
          </div>
          <div class="pt-4 border-t border-[#262626] flex gap-4">
            <button @click="importJSON" class="flex-1 bg-[#1a1a1a] border-[#262626] py-3 rounded text-xs font-bold uppercase hover:bg-[#262626] flex items-center justify-center gap-2">
              <Icon icon="lucide:upload" /> 导入价格表 (JSON)
            </button>
            <button @click="exportJSON" class="flex-1 bg-[#1a1a1a] border-[#262626] py-3 rounded text-xs font-bold uppercase hover:bg-[#262626] flex items-center justify-center gap-2">
              <Icon icon="lucide:download" /> 导出价格表 (JSON)
            </button>
          </div>
          <input ref="fileInput" type="file" accept="application/json" class="hidden" @change="onFile" />
        </div>
      </div>

      <!-- 成本预览计算器 -->
      <div class="glass-card flex flex-col items-center justify-center p-8 text-center border-dashed border-2 border-[#262626]">
        <Icon icon="lucide:calculator" class="text-4xl text-[#262626] mb-4" />
        <h4 class="text-sm font-bold mb-1">成本预览计算器</h4>
        <p class="text-xs text-[#737373] mb-6">输入消耗 Token 估算成本(按全局平均单价)</p>
        <div class="max-w-xs space-y-3 w-full">
          <div class="flex justify-between items-center bg-[#1a1a1a] border border-[#262626] px-4 py-3 rounded">
            <span class="text-xs text-[#a3a3a3]">预估消耗</span>
            <span class="font-mono text-sm">{{ fmtTokens(estimateTokens) }} Tokens</span>
          </div>
          <div class="flex justify-between items-center bg-white/5 border border-white/10 px-4 py-3 rounded">
            <span class="text-xs text-[#a3a3a3]">总成本</span>
            <span class="text-lg font-bold mono-text">{{ fmtCost(estimateCost) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { api, fmtTokens, fmtCost } from '../api'
import TableHeader from '../components/TableHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'

const models = ref([])
const channels = ref([])
const keyword = ref('')
const series = ref('')
const prices = ref({})
const batchChannel = ref('')
const fileInput = ref(null)

const filtered = computed(() => {
  let list = models.value
  if (series.value) list = list.filter(m => m.model_id.toLowerCase().includes(series.value))
  if (keyword.value) list = list.filter(m => m.model_id.toLowerCase().includes(keyword.value.toLowerCase()))
  return list
})
const estimateTokens = computed(() => 1000000)
const estimateCost = computed(() => {
  let total = 0
  for (const m of models.value) {
    const p = prices.value[m.id]
    total += estimateTokens.value / 1000 * (Number(p.input) || 0)
  }
  return total
})

function initPrices() {
  prices.value = {}
  for (const m of models.value) {
    prices.value[m.id] = { input: m.price_input ?? '', output: m.price_output ?? '' }
  }
}

async function saveAll() {
  for (const m of models.value) {
    const p = prices.value[m.id]
    const input = p.input === '' ? null : Number(p.input)
    const output = p.output === '' ? null : Number(p.output)
    if (input !== m.price_input || output !== m.price_output) {
      await api.updatePrice(m.id, input, output)
    }
  }
  await load()
}
function resetPrice(m) {
  prices.value[m.id] = { input: '', output: '' }
}
async function importFromChannel() {
  if (!batchChannel.value) return alert('请选择渠道')
  const res = await fetch(`/api/admin/channels/${batchChannel.value}`)
  const ch = await res.json()
  if (!confirm(`将按渠道 ${ch.name} 拉取官方价并覆盖当前单价?`)) return
  // 简化:提示手动输入,真实价格源需调用渠道官网价格(一期不做自动拉取)
  alert('一期未内置官方价源。请导出价格表后填写,或手动逐行输入。')
}
function exportJSON() {
  const data = models.value.map(m => ({ model_id: m.model_id, price_input: m.price_input, price_output: m.price_output }))
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'pricing.json'
  a.click()
}
function importJSON() {
  fileInput.value?.click()
}
async function onFile(e) {
  const f = e.target.files[0]
  if (!f) return
  const data = JSON.parse(await f.text())
  for (const item of data) {
    const m = models.value.find(x => x.model_id === item.model_id)
    if (m) {
      await api.updatePrice(m.id, item.price_input ?? null, item.price_output ?? null)
    }
  }
  e.target.value = ''
  await load()
}

async function load() {
  const [m, c] = await Promise.all([api.listModels(), api.listChannels()])
  models.value = m.models
  channels.value = c.channels
  initPrices()
}
onMounted(load)
</script>
