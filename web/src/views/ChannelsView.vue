<template>
  <div>
    <div class="mb-10">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <div class="w-2 h-8 bg-white"></div>
          <h1 class="text-2xl font-bold tracking-tight uppercase">渠道管理</h1>
        </div>
        <button @click="openCreate" class="text-xs font-bold px-4 py-2 rounded btn-primary flex items-center gap-1">
          <Icon icon="lucide:plus" /> 添加新渠道
        </button>
      </div>
      <p class="text-sm text-[#737373] mt-2">上游渠道(OpenAI 兼容端点)配置与监控,数字越小优先级越高</p>
    </div>

    <div class="channel-grid">
      <!-- 渠道卡片 -->
      <div v-for="(c, i) in channels" :key="c.id" class="glass-card flex flex-col"
        :class="{ 'border-amber-500/30 ring-1 ring-amber-500/20': c.status === 'cooldown' }">
        <div class="p-6 border-b border-[#262626] flex justify-between items-start">
          <div>
            <div class="text-3xl font-bold mono-text text-[#404040]">{{ String(i + 1).padStart(2, '0') }}</div>
            <h3 class="text-lg font-bold mt-1">{{ c.name }}</h3>
            <div class="text-xs font-mono text-[#737373] mt-1 break-all">{{ c.base_url }}</div>
          </div>
          <StatusBadge v-if="c.status === 'cooldown'" text="Cooldown" type="error" />
          <StatusBadge v-else-if="c.enabled" text="Online" type="success" />
          <StatusBadge v-else text="Disabled" type="neutral" />
        </div>

        <!-- 冷静期错误条 -->
        <div v-if="c.status === 'cooldown'" class="p-6 pb-0">
          <div class="bg-amber-500/5 border border-amber-500/20 p-3 rounded-sm mb-4">
            <div class="flex items-center gap-2 mb-1">
              <Icon icon="lucide:alert-circle" class="text-amber-500" />
              <span class="text-[10px] text-amber-500 uppercase font-bold">最近错误</span>
            </div>
            <div class="text-xs text-[#a3a3a3] font-mono break-all">{{ c.last_error || '连续失败触发冷静' }}</div>
          </div>
        </div>

        <div class="p-6 grid grid-cols-2 gap-4">
          <div>
            <div class="text-[10px] text-[#737373] uppercase font-bold tracking-widest mb-1">优先级</div>
            <div class="text-xl font-bold mono-text">{{ c.priority }}</div>
          </div>
          <div>
            <div class="text-[10px] text-[#737373] uppercase font-bold tracking-widest mb-1">失败计数</div>
            <div class="text-xl font-bold mono-text" :class="c.failure_count ? 'text-amber-500' : ''">{{ c.failure_count }}</div>
          </div>
        </div>

        <div class="px-6 pb-6 mt-auto flex gap-2">
          <template v-if="c.status === 'cooldown'">
            <button @click="recover(c.id)" class="flex-1 py-2 text-[10px] font-bold uppercase tracking-tighter bg-amber-500 text-black hover:bg-amber-400 rounded">
              手动解除冷静
            </button>
            <button @click="edit(c)" class="px-3 py-2 border border-[#262626] rounded text-[#a3a3a3] hover:bg-[#1a1a1a]">
              <Icon icon="lucide:settings" />
            </button>
            <button @click="remove(c)" title="删除渠道" class="px-3 py-2 border border-[#262626] rounded text-[#a3a3a3] hover:text-red-500 hover:bg-red-500/10">
              <Icon icon="lucide:trash-2" />
            </button>
          </template>
          <template v-else>
            <button @click="test(c)" :disabled="testing === c.id"
              class="flex-1 py-2 text-[10px] font-bold uppercase tracking-tighter border border-[#262626] hover:bg-[#1a1a1a] rounded">
              {{ testing === c.id ? '测试中...' : '测试连接' }}
            </button>
            <button @click="edit(c)" class="flex-1 py-2 text-[10px] font-bold uppercase tracking-tighter border border-[#262626] hover:bg-[#1a1a1a] rounded">
              编辑渠道
            </button>
            <button @click="toggle(c)" class="px-3 py-2 border border-[#262626] rounded"
              :class="c.enabled ? 'text-red-500 hover:bg-red-500/10' : 'text-green-500 hover:bg-green-500/10'">
              <Icon :icon="c.enabled ? 'lucide:power' : 'lucide:toggle-right'" />
            </button>
            <button @click="remove(c)" title="删除渠道" class="px-3 py-2 border border-[#262626] rounded text-[#a3a3a3] hover:text-red-500 hover:bg-red-500/10">
              <Icon icon="lucide:trash-2" />
            </button>
          </template>
        </div>
      </div>

      <!-- 添加占位卡 -->
      <div @click="openCreate" class="glass-card flex flex-col items-center justify-center p-12 border-dashed border-2 border-[#262626] hover:border-[#404040] hover:bg-[#111] transition-all cursor-pointer">
        <Icon icon="lucide:plus" class="text-4xl text-[#262626] group-hover:text-[#737373]" />
        <span class="text-[10px] uppercase font-bold tracking-[0.3em] text-[#404040] mt-3">点击快速添加</span>
      </div>
    </div>

    <!-- 测试结果 -->
    <div v-if="testResult" class="mt-6 glass-card p-4 text-xs font-mono" :class="testResult.ok ? 'text-green-500' : 'text-red-500'">
      {{ testResult.message }}
    </div>

    <!-- 编辑/新建弹窗 -->
    <div v-if="showModal" class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-6">
      <div class="glass-card w-full max-w-md p-8 shadow-2xl rounded-sm">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-bold uppercase tracking-wider">{{ form.id ? '编辑渠道' : '添加新渠道' }}</h3>
          <button @click="showModal = false" class="text-[#737373] hover:text-white"><Icon icon="lucide:x" /></button>
        </div>
        <form @submit.prevent="save" class="space-y-4">
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">渠道名称 *</label>
            <input v-model="form.name" required placeholder="例如:OpenAI Official" class="w-full px-4 py-3 rounded input-field text-sm" />
          </div>
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">Base URL *</label>
            <input v-model="form.base_url" required placeholder="https://api.openai.com/v1 或 https://api.deepseek.com"
              class="w-full px-4 py-3 rounded input-field text-sm font-mono" />
            <p class="text-[10px] text-[#525252] mt-1">客户端路径 /v1/* 将拼接在此地址后(去 /v1 前缀)</p>
          </div>
          <div>
            <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">API Key *</label>
            <input v-model="form.api_key" :required="!form.id" :placeholder="form.id ? form.masked_key : 'sk-...'" class="w-full px-4 py-3 rounded input-field text-sm font-mono" />
            <p v-if="form.id" class="text-[10px] text-[#525252] mt-1">留空表示不修改现有密钥</p>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">优先级</label>
              <input v-model.number="form.priority" type="number" class="w-full px-4 py-3 rounded input-field text-sm mono-text" />
            </div>
            <div>
              <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">鉴权头(可选)</label>
              <input v-model="form.auth_header" placeholder="Authorization" class="w-full px-4 py-3 rounded input-field text-sm mono-text" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">超时(毫秒)</label>
              <input v-model.number="form.timeout_ms" type="number" min="0" placeholder="0 = 全局(首次响应默认 60000)"
                class="w-full px-4 py-3 rounded input-field text-sm mono-text" />
              <p class="text-[10px] text-[#525252] mt-1">流式 = 首次响应(TTFB)超时,全局默认 60000ms;非流式 = 完整请求超时,全局默认 300000ms(5 分钟)</p>
            </div>
            <div>
              <label class="text-[10px] font-bold text-[#737373] uppercase tracking-widest block mb-2">冷静期(毫秒)</label>
              <input v-model.number="form.cooldown_ms" type="number" min="0" placeholder="0 = 全局(默认 600000)"
                class="w-full px-4 py-3 rounded input-field text-sm mono-text" />
              <p class="text-[10px] text-[#525252] mt-1">连续失败后冷静时长,留空或 0 使用全局配置</p>
            </div>
          </div>
          <div class="flex gap-4 pt-4">
            <button type="button" @click="showModal = false" class="flex-1 py-3 text-sm font-bold rounded border border-[#262626] hover:bg-[#1a1a1a]">取消</button>
            <button type="submit" class="flex-1 py-3 text-sm font-bold rounded btn-primary">保存</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge.vue'

const channels = ref([])
const testing = ref(0)
const testResult = ref(null)
const showModal = ref(false)
const form = ref({ id: null, name: '', base_url: '', api_key: '', auth_header: 'Authorization', priority: 0, enabled: true, timeout_ms: 0, cooldown_ms: 0 })

async function load() {
  const data = await api.listChannels()
  channels.value = data.channels
}
function openCreate() {
  form.value = { id: null, name: '', base_url: '', api_key: '', auth_header: 'Authorization', priority: 0, enabled: true, timeout_ms: 0, cooldown_ms: 0, masked_key: '' }
  showModal.value = true
}
function edit(c) {
  form.value = { ...c, api_key: '', auth_header: c.auth_header || 'Authorization', masked_key: c.api_key, timeout_ms: c.timeout_ms || 0, cooldown_ms: c.cooldown_ms || 0 }
  showModal.value = true
}
async function save() {
  // 空输入兜底:转成数字,避免提交 "" 导致后端 int64 解码失败
  form.value.timeout_ms = parseInt(form.value.timeout_ms, 10) || 0
  form.value.cooldown_ms = parseInt(form.value.cooldown_ms, 10) || 0
  if (form.value.id) await api.updateChannel(form.value.id, form.value)
  else await api.createChannel(form.value)
  showModal.value = false
  load()
}
async function toggle(c) {
  await api.updateChannel(c.id, { ...c, api_key: '', enabled: !c.enabled })
  load()
}
async function test(c) {
  testing.value = c.id
  testResult.value = null
  const r = await api.testChannel(c.id)
  testResult.value = {
    ok: r.ok,
    message: r.ok ? `连通正常 · ${r.latency_ms}ms` : `失败:${r.error || 'HTTP ' + r.status}`,
  }
  testing.value = 0
}
async function recover(id) {
  await api.recoverChannel(id)
  load()
}
async function remove(c) {
  if (!confirm(`删除渠道「${c.name}」?该渠道的模型关联将一并移除。`)) return
  await api.deleteChannel(c.id)
  load()
}
onMounted(load)
</script>
