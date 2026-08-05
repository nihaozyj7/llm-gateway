// 管理 API 封装
async function req(url, options = {}) {
  const res = await fetch(url, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = await res.json()
      msg = data.error || data.message || msg
    } catch { /* ignore */ }
    throw new Error(msg)
  }
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  // 渠道
  listChannels: () => req('/api/admin/channels'),
  createChannel: (c) => req('/api/admin/channels', { method: 'POST', body: JSON.stringify(c) }),
  updateChannel: (id, c) => req(`/api/admin/channels/${id}`, { method: 'PUT', body: JSON.stringify(c) }),
  deleteChannel: (id) => req(`/api/admin/channels/${id}`, { method: 'DELETE' }),
  reorderChannels: (items) => req('/api/admin/channels/reorder', { method: 'POST', body: JSON.stringify({ items }) }),
  testChannel: (id) => req(`/api/admin/test?channel_id=${id}`, { method: 'POST' }),
  recoverChannel: (id) => req(`/api/admin/channels/${id}/recover`, { method: 'POST' }),

  // 模型
  listModels: () => req('/api/admin/models'),
  createModel: (model_id, display_name) => req('/api/admin/models', { method: 'POST', body: JSON.stringify({ model_id, display_name }) }),
  deleteModel: (id) => req(`/api/admin/models/${id}`, { method: 'DELETE' }),
  updatePrice: (id, price_input, price_output, price_cache_read) => req(`/api/admin/models/${id}/price`, { method: 'PUT', body: JSON.stringify({ price_input, price_output, price_cache_read }) }),
  fetchChannelModels: (channel_id) => req('/api/admin/models/fetch', { method: 'POST', body: JSON.stringify({ channel_id }) }),
  syncChannelModels: (channel_id, models) => req('/api/admin/models/sync', { method: 'POST', body: JSON.stringify({ channel_id, models }) }),
  reorderModelChannels: (modelId, items) => req(`/api/admin/models/${modelId}/reorder`, { method: 'POST', body: JSON.stringify({ items }) }),
  linkChannel: (modelId, channel_id, upstream_model_name) => req(`/api/admin/models/${modelId}/channels`, { method: 'POST', body: JSON.stringify({ channel_id, upstream_model_name }) }),
  unlinkChannel: (modelId, channel_id) => req(`/api/admin/models/${modelId}/channels`, { method: 'DELETE', body: JSON.stringify({ channel_id }) }),
  testModel: (model_id, signal) => req('/api/admin/models/test', { method: 'POST', body: JSON.stringify({ model_id }), signal }),

  // API Keys
  listKeys: () => req('/api/admin/keys'),
  createKey: (name) => req('/api/admin/keys', { method: 'POST', body: JSON.stringify({ name }) }),
  deleteKey: (id) => req(`/api/admin/keys/${id}`, { method: 'DELETE' }),
  enableKey: (id) => req(`/api/admin/keys/${id}/enable`, { method: 'POST' }),
  disableKey: (id) => req(`/api/admin/keys/${id}/disable`, { method: 'POST' }),

  // 日志
  listLogs: (params) => {
    const qs = new URLSearchParams(params).toString()
    return req(`/api/admin/logs?${qs}`)
  },
  getLog: (id) => req(`/api/admin/logs/${id}`),
  clearLogs: () => req('/api/admin/logs/clear', { method: 'POST' }),

  // 统计
  stats: (params) => {
    const qs = new URLSearchParams(params).toString()
    return req(`/api/admin/stats?${qs}`)
  },
  dashboard: () => req('/api/admin/dashboard'),
  resetStats: () => req('/api/admin/stats/reset', { method: 'POST' }),
}

export function fmtTime(t) {
  if (!t) return '--'
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { hour12: false })
}
export function fmtTimeShort(t) {
  if (!t) return '--'
  const d = new Date(t)
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}
export function fmtNum(n) {
  if (n === null || n === undefined) return '0'
  return Number(n).toLocaleString('en-US')
}
export function fmtTokens(n) {
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'
  return String(n || 0)
}
export function fmtCost(c) {
  return '¥ ' + Number(c || 0).toFixed(3)
}
