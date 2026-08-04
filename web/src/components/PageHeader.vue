<template>
  <header class="sticky top-0 z-40 bg-[#0a0a0a] border-b border-[#262626]">
    <div class="max-w-[1440px] mx-auto px-6 h-16 flex items-center justify-between">
      <div class="flex items-center gap-12">
        <div class="text-lg font-bold tracking-tighter flex items-center gap-2">
          <div class="w-2 h-6 bg-white"></div>
          LLM GATEWAY
        </div>
        <nav class="hidden md:flex items-center gap-8 h-16">
          <router-link to="/dashboard" class="nav-link flex items-center h-full px-1 pt-1 text-sm font-medium" :class="{ active: active === 'dashboard' }">概览</router-link>
          <router-link to="/channels" class="nav-link flex items-center h-full px-1 pt-1 text-sm font-medium" :class="{ active: active === 'channels' }">渠道管理</router-link>
          <router-link to="/models" class="nav-link flex items-center h-full px-1 pt-1 text-sm font-medium" :class="{ active: active === 'models' }">模型管理</router-link>
          <router-link to="/pricing" class="nav-link flex items-center h-full px-1 pt-1 text-sm font-medium" :class="{ active: active === 'pricing' }">价格设置</router-link>
          <router-link to="/keys" class="nav-link flex items-center h-full px-1 pt-1 text-sm font-medium" :class="{ active: active === 'keys' }">API Keys</router-link>
        </nav>
      </div>
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2 px-3 py-1.5 bg-[#1a1a1a] border border-[#262626] rounded text-xs font-mono">
          <span class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></span>
          GATEWAY ONLINE
        </div>
      </div>
    </div>
  </header>
</template>

<script setup>
defineProps({ active: { type: String, default: 'dashboard' } })
</script>
