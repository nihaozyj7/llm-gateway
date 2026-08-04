<template>
  <div class="min-h-screen flex flex-col">
    <PageHeader :active="active" />
    <main class="flex-1 max-w-[1440px] mx-auto w-full px-6 py-10">
      <router-view v-slot="{ Component }">
        <Transition name="page" mode="out-in">
          <component :is="Component" :key="route.path" />
        </Transition>
      </router-view>
    </main>
    <PageFooter />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import PageHeader from './components/PageHeader.vue'
import PageFooter from './components/PageFooter.vue'

const route = useRoute()
const active = computed(() => route.path.replace('/', '') || 'dashboard')
</script>
