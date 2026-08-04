<template>
  <div>
    <div class="flex justify-between items-end mb-8">
      <div>
        <h1 class="text-2xl font-bold uppercase tracking-tight">API Keys 管理</h1>
        <p class="text-sm text-[#737373] mt-1">签发 / 撤销网关 API Key,客户端调用 /v1/* 时校验</p>
      </div>
      <button @click="openCreate" class="bg-white text-black px-4 py-2 rounded text-sm font-bold hover:bg-[#e5e5e5] flex items-center gap-1">
        <Icon icon="lucide:plus" class="text-lg" /> 创建新密钥
      </button>
    </div>

    <!-- 客户端接入指南 -->
    <div class="glass-card mb-8 p-6">
      <div class="flex items-center justify-between mb-5">
        <div class="flex items-center gap-2">
          <Icon icon="lucide:plug" class="text-[#737373]" />
          <h2 class="text-sm font-bold uppercase tracking-widest">客户端接入指南</h2>
        </div>
        <span class="text-[10px] font-mono text-[#737373]">OpenAI 兼容</span>
      </div>

      <div class="grid md:grid-cols-2 gap-6 mb-6">
        <!-- Base URL -->
        <div>
          <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">Base URL(客户端填写此地址)</label>
          <div class="flex items-center gap-2">
            <code class="flex-1 px-3 py-2 rounded input-field text-xs font-mono text-white select-all">{{ baseUrl }}</code>
            <button @click="copy(baseUrl)" title="复制 Base URL" class="p-2 hover:bg-[#262626] rounded text-[#a3a3a3] hover:text-white">
              <Icon icon="lucide:copy" class="text-lg" />
            </button>
          </div>
          <p class="text-[10px] text-[#525252] mt-1.5">OpenAI SDK 中 base_url 填此地址,如 Python: OpenAI(base_url="{{ baseUrl }}")</p>
        </div>

        <!-- 认证 -->
        <div>
          <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">认证方式</label>
          <div class="flex items-center gap-2">
            <code class="flex-1 px-3 py-2 rounded input-field text-xs font-mono text-white select-all">Authorization: Bearer &lt;你的 API key&gt;</code>
            <button @click="copy('Authorization: Bearer <你的 API key>')" title="复制认证头" class="p-2 hover:bg-[#262626] rounded text-[#a3a3a3] hover:text-white">
              <Icon icon="lucide:copy" class="text-lg" />
            </button>
          </div>
          <p class="text-[10px] text-[#525252] mt-1.5">用上方创建的 API key 替换 &lt;你的 API key&gt;</p>
        </div>
      </div>

      <!-- 支持的接口 -->
      <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-3">当前支持的接口</label>
      <p class="text-[11px] text-[#737373] leading-relaxed mb-3">网关为<b class="text-white/70">透传代理</b>,本身不聚合、不转换接口能力:以下入口只是透传的 OpenAI 风格接口,具体是否可用、支持哪些能力(如工具调用、多模态)取决于所关联的<b class="text-white/70">上游渠道本身</b>是否支持。</p>
      <div class="space-y-2">
        <div v-for="ep in endpoints" :key="ep.path" class="flex items-center gap-3 p-3 bg-[#1a1a1a] border border-[#262626] rounded">
          <span class="w-14 text-center text-[10px] font-bold px-2 py-1 rounded"
            :class="ep.method === 'GET' ? 'bg-blue-500/10 text-blue-400' : 'bg-green-500/10 text-green-400'">{{ ep.method }}</span>
          <code class="font-mono text-xs text-white flex-1">{{ ep.path }}</code>
          <span class="text-[11px] text-[#737373] hidden md:inline">{{ ep.desc }}</span>
          <button @click="copy(baseUrl + ep.path)" title="复制完整地址" class="p-1.5 hover:bg-[#262626] rounded text-[#a3a3a3] hover:text-white">
            <Icon icon="lucide:copy" class="text-sm" />
          </button>
        </div>
      </div>

      <!-- curl 示例 -->
      <div class="mt-6">
        <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">快速开始(curl 示例)</label>
        <div class="relative">
          <pre class="json-block">{{ curlExample }}</pre>
          <button @click="copy(curlExample)" class="absolute top-2 right-2 p-1.5 hover:bg-[#262626] rounded text-[#737373] hover:text-white" title="复制示例">
            <Icon icon="lucide:copy" class="text-sm" />
          </button>
        </div>
      </div>
    </div>

    <div class="glass-card overflow-hidden">
      <TableHeader title="网关 API 密钥列表" show-refresh refresh-label="刷新列表" @refresh="load" />
      <div class="overflow-x-auto">
        <table class="w-full text-left">
          <thead>
            <tr class="bg-[#1a1a1a] border-b border-[#262626]">
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">密钥名称</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">密钥内容</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">创建时间</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">最后使用</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">使用次数</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest">状态</th>
              <th class="px-6 py-4 text-[10px] font-bold text-[#737373] uppercase tracking-widest text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#262626]">
            <tr v-for="k in keys" :key="k.id" :class="{ 'opacity-60': !k.enabled }" class="hover:bg-[#1a1a1a] transition-colors">
              <td class="px-6 py-4 text-sm font-medium">{{ k.name }}</td>
              <td class="px-6 py-4">
                <div class="flex gap-2 font-mono text-xs items-center">
                  <span class="text-[#a3a3a3]">{{ masked(k.key_prefix) }}</span>
                  <span class="text-white">{{ tail(k.key_prefix) }}</span>
                  <button @click="copy(k.key_secret)" title="复制完整密钥" class="p-1.5 hover:bg-[#262626] rounded text-[#737373] hover:text-white">
                    <Icon icon="lucide:copy" class="text-sm" />
                  </button>
                </div>
              </td>
              <td class="px-6 py-4 text-xs font-mono text-[#737373]">{{ fmtTime(k.created_at) }}</td>
              <td class="px-6 py-4 text-xs font-mono text-[#737373]">{{ k.last_used_at ? relative(k.last_used_at) : '从未使用' }}</td>
              <td class="px-6 py-4 text-xs font-mono text-[#a3a3a3]">{{ fmtNum(k.usage_count) }}</td>
              <td class="px-6 py-4">
                <label class="switch">
                  <input type="checkbox" :checked="k.enabled" @change="toggle(k)" />
                  <span class="slider"></span>
                </label>
              </td>
              <td class="px-6 py-4 text-right">
                <button @click="remove(k)" title="撤销密钥" class="p-2 hover:bg-[#262626] rounded text-[#a3a3a3] hover:text-red-500">
                  <Icon icon="lucide:trash-2" class="text-lg" />
                </button>
              </td>
            </tr>
            <tr v-if="!keys.length">
              <td colspan="7" class="px-6 py-10 text-center text-sm text-[#737373]">暂无密钥,点击右上角「创建新密钥」</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 新建密钥弹窗 -->
    <div v-if="showModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-md p-8 shadow-2xl rounded-sm">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-bold uppercase tracking-wider">创建新密钥</h3>
          <button @click="showModal = false" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
        </div>
        <form @submit.prevent="create" class="space-y-4">
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">密钥名称</label>
            <input v-model="name" required placeholder="例如:生产环境主节点" class="w-full px-4 py-3 rounded input-field text-sm" />
            <p class="text-[10px] text-[#525252] mt-1">用于标识该密钥的用途,便于日后撤销管理。</p>
          </div>
          <div class="flex gap-4 pt-4">
            <button type="button" @click="showModal = false" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">取消</button>
            <button type="submit" class="flex-1 py-3 text-sm font-bold rounded btn-primary">确认创建</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 创建成功展示明文 -->
    <div v-if="createdKey" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-md p-8 shadow-2xl rounded-sm">
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-lg font-bold uppercase tracking-wider">密钥创建成功</h3>
          <button @click="createdKey = null" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
        </div>
        <div class="json-block select-all">{{ createdKey }}</div>
        <div class="flex gap-4 pt-6">
          <button @click="copy(createdKey)" class="flex-1 py-3 text-sm font-bold rounded btn-outline">{{ copied ? '已复制 ✓' : '复制密钥' }}</button>
          <button @click="createdKey = null" class="flex-1 py-3 text-sm font-bold rounded btn-primary">我已保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { api, fmtNum, fmtTime } from '../api'
import TableHeader from '../components/TableHeader.vue'

const keys = ref([])
const showModal = ref(false)
const createdKey = ref(null)
const name = ref('')
const copied = ref(false)
let copyTimer = null

// 客户端接入指南:base URL 取自当前页面地址(管理界面与 /v1 同源)
const baseUrl = computed(() => `${window.location.origin}/v1`)
const endpoints = [
  { method: 'GET', path: '/v1/models', desc: '获取可用模型列表' },
  { method: 'POST', path: '/v1/chat/completions', desc: '对话补全(支持 stream)' },
]
const curlExample = computed(() => `curl ${baseUrl.value}/chat/completions \\
  -H "Authorization: Bearer sk-gate-xxx" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"你好"}],"stream":true}'`)

function masked(prefix) {
  return prefix ? prefix.slice(0, -4).replace(/./g, '•') : '••••'
}
function tail(prefix) {
  return prefix ? prefix.slice(-4) : ''
}
function relative(t) {
  const diff = Date.now() - new Date(t).getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const h = Math.floor(min / 60)
  if (h < 24) return `${h} 小时前`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d} 天前`
  return `${Math.floor(d / 30)} 月前`
}
async function copy(text) {
  if (!text) {
    alert('该密钥为旧版本创建,没有保存明文,请删除后重新创建')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => (copied.value = false), 2000)
  } catch { alert('复制失败') }
}
function openCreate() {
  name.value = ''
  showModal.value = true
}
async function create() {
  const r = await api.createKey(name.value)
  createdKey.value = r.key
  showModal.value = false
  load()
}
async function toggle(k) {
  if (k.enabled) await api.disableKey(k.id)
  else await api.enableKey(k.id)
  load()
}
async function remove(k) {
  if (!confirm(`撤销密钥「${k.name}」?使用该密钥的客户端将立即失效。`)) return
  await api.deleteKey(k.id)
  load()
}
async function load() {
  const r = await api.listKeys()
  keys.value = r.keys
}
onMounted(load)
</script>
