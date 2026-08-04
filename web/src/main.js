import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './styles.css'

const routes = [
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', component: () => import('./views/DashboardView.vue'), meta: { title: '概览' } },
  { path: '/channels', component: () => import('./views/ChannelsView.vue'), meta: { title: '渠道管理' } },
  { path: '/models', component: () => import('./views/ModelsView.vue'), meta: { title: '模型管理' } },
  { path: '/pricing', component: () => import('./views/PricingView.vue'), meta: { title: '价格设置' } },
  { path: '/keys', component: () => import('./views/KeysView.vue'), meta: { title: 'API Keys' } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

createApp(App).use(router).mount('#app')
