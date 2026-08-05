<template>
  <div>
    <!-- 页头 -->
    <div class="mb-10">
      <div class="flex items-center gap-3">
        <div class="w-2 h-8 bg-white"></div>
        <h1 class="text-2xl font-bold tracking-tight uppercase">价格设置</h1>
      </div>
      <p class="text-sm text-[#737373] mt-2 max-w-2xl">配置聚合模型单价(元/百万 Token),用于日志成本估算。同名模型全局统一定价,不填则成本计为 0。</p>
    </div>

    <!-- 定价表 -->
    <div class="glass-card overflow-hidden mb-8">
      <TableHeader title="全局模型定价表" show-refresh refresh-label="保存修改" :busy="saving" :saved="saved" @refresh="saveAll" />
      <div class="p-6 border-b bg-[#0e0e0e] flex flex-wrap gap-4 justify-between items-center">
        <div class="relative">
          <Icon icon="lucide:search" class="absolute left-3 top-1/2 -translate-y-1/2 text-[#404040]" />
          <input v-model="keyword" placeholder="搜索模型 ID..."
            class="bg-[#1a1a1a] border-[#262626] rounded py-1.5 pl-9 pr-4 text-xs input-field" />
        </div>
        <div class="flex items-center gap-4">
          <select v-model="series" class="bg-[#1a1a1a] border-[#262626] rounded px-3 py-1.5 text-xs input-field">
            <option value="">所有系列</option>
            <option value="gpt">GPT 系列</option>
            <option value="claude">Claude 系列</option>
            <option value="deepseek">DeepSeek 系列</option>
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
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">输入单价 (元/M)</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">缓存读取单价 (元/M)</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">输出单价 (元/M)</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">计费状态</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <tr v-for="m in filtered" :key="m.id" class="hover:bg-[#1a1a1a] transition-colors">
              <td class="px-6 py-4 font-mono text-sm">{{ m.model_id }}</td>
              <td class="px-6 py-4 text-xs text-[#737373]">{{ m.channels.length }} 个渠道</td>
              <td class="px-6 py-4">
                <input v-model="prices[m.id].input" type="number" step="0.0001" min="0" placeholder="0.0000"
                  class="price-input w-24 px-2 py-1 text-xs mono-text" />
              </td>
              <td class="px-6 py-4">
                <input v-model="prices[m.id].cache" type="number" step="0.0001" min="0" placeholder="0.0000"
                  class="price-input w-24 px-2 py-1 text-xs mono-text" />
              </td>
              <td class="px-6 py-4">
                <input v-model="prices[m.id].output" type="number" step="0.0001" min="0" placeholder="0.0000"
                  class="price-input w-24 px-2 py-1 text-xs mono-text" />
              </td>
              <td class="px-6 py-4">
                <StatusBadge v-if="m.price_input != null || m.price_output != null || m.price_cache_read != null" text="已计费" type="success" />
                <StatusBadge v-else text="未计费" type="info" />
              </td>
              <td class="px-6 py-4 text-right">
                <button @click="resetPrice(m)" class="text-[10px] font-bold uppercase tracking-widest text-[#a3a3a3] hover:text-white">重置</button>
              </td>
            </tr>
            <tr v-if="!filtered.length">
              <td colspan="7" class="px-6 py-10 text-center text-sm text-[#737373]">暂无模型,请先到模型管理页添加或同步</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="px-6 py-3 border-t border-[#262626] bg-[#0e0e0e] text-xs text-[#737373]">
        {{ filtered.length }} 个模型 · 修改后点击右上角「保存修改」生效
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api'
import TableHeader from '../components/TableHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'

const models = ref([])
const keyword = ref('')
const series = ref('')
const prices = ref({})
const saving = ref(false)
const saved = ref(false)

const filtered = computed(() => {
  let list = models.value
  if (series.value) list = list.filter(m => m.model_id.toLowerCase().includes(series.value))
  if (keyword.value) list = list.filter(m => m.model_id.toLowerCase().includes(keyword.value.toLowerCase()))
  return list
})

function initPrices() {
  prices.value = {}
  for (const m of models.value) {
    prices.value[m.id] = { input: m.price_input ?? '', cache: m.price_cache_read ?? '', output: m.price_output ?? '' }
  }
}

async function saveAll() {
  if (saving.value) return
  saving.value = true
  saved.value = false
  try {
    for (const m of models.value) {
      const p = prices.value[m.id]
      const input = p.input === '' ? null : Number(p.input)
      const cache = p.cache === '' ? null : Number(p.cache)
      const output = p.output === '' ? null : Number(p.output)
      if (input !== m.price_input || cache !== m.price_cache_read || output !== m.price_output) {
        await api.updatePrice(m.id, input, output, cache)
      }
    }
    await load()
    saved.value = true
    setTimeout(() => { saved.value = false }, 1500)
  } catch (e) {
    alert('保存失败: ' + (e.message || e))
  } finally {
    saving.value = false
  }
}
function resetPrice(m) {
  prices.value[m.id] = { input: '', cache: '', output: '' }
}
async function load() {
  models.value = (await api.listModels()).models
  initPrices()
}
onMounted(load)
</script>
