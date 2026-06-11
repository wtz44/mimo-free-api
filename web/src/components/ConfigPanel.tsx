import { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Key, Cpu, Save, Eye, EyeOff, Check, Loader2, Users, Plus, Trash2, Globe } from 'lucide-react'
import { useSettings } from '../contexts/SettingsContext'
import { apiFetch } from '../lib/api'

interface Account {
  id: string
  service_token: string
  user_id: string
  ph: string
  active: boolean
}

export function ConfigPanel() {
  const { t, resolvedTheme, lang } = useSettings()
  const isDark = resolvedTheme === 'dark'

  const [apiKey, setApiKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const [baseUrl, setBaseUrl] = useState(() => localStorage.getItem('api_base_url') || '')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [defaultModel, setDefaultModel] = useState('mimo-v2.5')
  const [accounts, setAccounts] = useState<Account[]>([])
  const [loading, setLoading] = useState(true)

  // Add form state
  const [showAdd, setShowAdd] = useState(false)
  const [newId, setNewId] = useState('')
  const [newToken, setNewToken] = useState('')
  const [newUserId, setNewUserId] = useState('')
  const [newPh, setNewPh] = useState('')
  const [adding, setAdding] = useState(false)

  const loadConfig = () => {
    apiFetch('/admin/api/config')
      .then(r => r.json())
      .then(cfg => {
        setApiKey(cfg.api_key || '')
        setDefaultModel(cfg.default_model || 'mimo-v2.5')
        setAccounts(cfg.accounts || [])
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadConfig() }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      localStorage.setItem('api_base_url', baseUrl)
      await apiFetch('/admin/api/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ default_model: defaultModel })
      })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } finally {
      setSaving(false)
    }
  }

  const handleAdd = async () => {
    if (!newId.trim() || !newToken.trim()) return
    setAdding(true)
    try {
      const res = await apiFetch('/admin/api/accounts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: newId.trim(),
          service_token: newToken.trim(),
          user_id: newUserId.trim(),
          ph: newPh.trim(),
          active: true
        })
      })
      if (res.ok) {
        setNewId(''); setNewToken(''); setNewUserId(''); setNewPh('')
        setShowAdd(false)
        loadConfig()
      }
    } finally {
      setAdding(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(lang === 'zh' ? `确认删除账号 "${id}"？` : `Delete account "${id}"?`)) return
    const res = await apiFetch('/admin/api/accounts', {
        method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id })
    })
    if (res.ok) loadConfig()
  }

  const maskToken = (token: string) => {
    if (!token) return '-'
    if (token.length <= 12) return '***'
    return token.slice(0, 6) + '...' + token.slice(-4)
  }

  const inputClass = isDark
    ? 'w-full bg-white/[0.04] border border-white/[0.08] rounded-xl px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-purple-500/50 focus:ring-1 focus:ring-purple-500/20 transition-all font-mono text-sm'
    : 'w-full bg-white border border-gray-200 rounded-xl px-4 py-3 text-gray-800 placeholder-gray-400 focus:outline-none focus:border-blue-500/50 focus:ring-1 focus:ring-blue-500/20 transition-all font-mono text-sm'

  const cardClass = isDark ? 'glass p-6 space-y-5' : 'bg-white rounded-xl border border-gray-100 p-6 space-y-5 shadow-sm'

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className={`w-6 h-6 animate-spin ${isDark ? 'text-purple-400' : 'text-blue-500'}`} />
      </div>
    )
  }

  return (
    <div className="space-y-8 max-w-3xl">
      {/* Header */}
      <div>
        <motion.h1 className={`text-2xl font-bold ${isDark ? 'text-white' : 'text-gray-800'}`} initial={{ opacity: 0, x: -20 }} animate={{ opacity: 1, x: 0 }}>
          {t('configTitle')}
        </motion.h1>
        <motion.p className={`${isDark ? 'text-gray-400' : 'text-gray-400'} text-sm mt-0.5`} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.1 }}>
          {t('configSubtitle')}
        </motion.p>
      </div>

      {/* API Settings */}
      <motion.div className={cardClass} initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }}>
        <h2 className={`text-base font-semibold ${isDark ? 'text-white' : 'text-gray-700'} flex items-center gap-2`}>
          <Key className={`w-4 h-4 ${isDark ? 'text-purple-400' : 'text-blue-500'}`} />
          API {lang === 'zh' ? '设置' : 'Settings'}
        </h2>

        <div className="space-y-1.5">
          <label className={`text-sm ${isDark ? 'text-gray-400' : 'text-gray-500'}`}>API Key</label>
          <div className="relative">
            <input type={showKey ? 'text' : 'password'} value={apiKey} onChange={e => setApiKey(e.target.value)} className={inputClass} placeholder="sk-..." readOnly />
            <button onClick={() => setShowKey(!showKey)} className={`absolute right-3 top-1/2 -translate-y-1/2 ${isDark ? 'text-gray-400 hover:text-white' : 'text-gray-400 hover:text-gray-600'} transition-colors`}>
              {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
        </div>

        <div className="space-y-1.5">
          <label className={`text-sm ${isDark ? 'text-gray-400' : 'text-gray-500'} flex items-center gap-1`}>
            <Globe className="w-3.5 h-3.5" /> Base URL
          </label>
          <input type="text" value={baseUrl} onChange={e => setBaseUrl(e.target.value)} className={inputClass}
            placeholder="http://localhost:8080" />
          <p className={`text-xs ${isDark ? 'text-gray-500' : 'text-gray-400'}`}>
            {lang === 'zh' ? '留空则使用当前地址，跨域部署时填完整地址' : 'Leave empty to use current origin, set when deploying separately.'}
          </p>
        </div>

        <div className="space-y-1.5">
          <label className={`text-sm ${isDark ? 'text-gray-400' : 'text-gray-500'} flex items-center gap-1`}>
            <Cpu className="w-3.5 h-3.5" /> {lang === 'zh' ? '默认模型' : 'Default Model'}
          </label>
          <div className="grid grid-cols-2 gap-3">
            {[
              { id: 'mimo-v2.5', label: 'MiMo V2.5', desc: lang === 'zh' ? '全模态 · 图片/音频/文件' : 'Omni · Image/Audio/File' },
              { id: 'mimo-v2.5-pro', label: 'MiMo V2.5 Pro', desc: lang === 'zh' ? '推理增强 · 文本/图片' : 'Reasoning · Text/Image' },
            ].map(m => (
              <button key={m.id} onClick={() => setDefaultModel(m.id)} className={`p-4 rounded-xl border text-left transition-all ${
                defaultModel === m.id
                  ? (isDark ? 'border-purple-500/50 bg-purple-500/10' : 'border-blue-500/50 bg-blue-50')
                  : (isDark ? 'border-white/[0.08] bg-white/[0.03] hover:bg-white/[0.06]' : 'border-gray-200 bg-gray-50 hover:bg-gray-100')
              }`}>
                <p className={`text-sm font-medium ${isDark ? 'text-white' : 'text-gray-800'}`}>{m.label}</p>
                <p className={`text-xs ${isDark ? 'text-gray-400' : 'text-gray-500'} mt-1`}>{m.desc}</p>
              </button>
            ))}
          </div>
        </div>
      </motion.div>

      {/* Accounts */}
      <motion.div className={cardClass} initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.3 }}>
        <div className="flex items-center justify-between">
          <h2 className={`text-base font-semibold ${isDark ? 'text-white' : 'text-gray-700'} flex items-center gap-2`}>
            <Users className={`w-4 h-4 ${isDark ? 'text-purple-400' : 'text-blue-500'}`} />
            MiMo {lang === 'zh' ? '账号池' : 'Account Pool'}
          </h2>
          <div className="flex items-center gap-3">
            <span className={`text-sm ${isDark ? 'text-gray-400' : 'text-gray-500'}`}>
              {accounts.filter(a => a.active).length}/{accounts.length} active
            </span>
            <motion.button onClick={() => setShowAdd(!showAdd)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors ${
                isDark ? 'bg-purple-500/10 text-purple-400 hover:bg-purple-500/20' : 'bg-blue-50 text-blue-600 hover:bg-blue-100'
              }`} whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.98 }}>
              <Plus className="w-4 h-4" />
              {lang === 'zh' ? '添加' : 'Add'}
            </motion.button>
          </div>
        </div>

        {/* Add Form */}
        <AnimatePresence>
          {showAdd && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              exit={{ opacity: 0, height: 0 }}
              className="overflow-hidden"
            >
              <div className={`p-4 rounded-xl border space-y-3 ${isDark ? 'bg-white/[0.02] border-purple-500/20' : 'bg-blue-50/50 border-blue-200'}`}>
                <div className="grid grid-cols-2 gap-3">
                  <input type="text" placeholder="Account ID (唯一标识)" value={newId} onChange={e => setNewId(e.target.value)} className={inputClass} />
                  <input type="text" placeholder="user_id" value={newUserId} onChange={e => setNewUserId(e.target.value)} className={inputClass} />
                </div>
                <input type="password" placeholder="service_token (Cookie中的完整值)" value={newToken} onChange={e => setNewToken(e.target.value)} className={inputClass} />
                <input type="text" placeholder="xiaomichatbot_ph" value={newPh} onChange={e => setNewPh(e.target.value)} className={inputClass} />
                <div className="flex gap-2 justify-end">
                  <button onClick={() => setShowAdd(false)} className={`px-4 py-2 rounded-lg text-sm ${isDark ? 'text-gray-400 hover:text-white' : 'text-gray-500 hover:text-gray-700'} transition-colors`}>
                    {lang === 'zh' ? '取消' : 'Cancel'}
                  </button>
                  <motion.button onClick={handleAdd} disabled={adding || !newId.trim() || !newToken.trim()}
                    className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-500 to-indigo-500 text-white rounded-lg text-sm disabled:opacity-50"
                    whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.98 }}>
                    {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
                    {lang === 'zh' ? '确认添加' : 'Confirm'}
                  </motion.button>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Account List */}
        {accounts.length === 0 ? (
          <div className={`text-center py-8 ${isDark ? 'text-gray-500' : 'text-gray-400'}`}>
            <Users className="w-8 h-8 mx-auto mb-2 opacity-40" />
            <p className="text-sm">{lang === 'zh' ? '暂无账号，点击上方"添加"' : 'No accounts. Click "Add" above'}</p>
          </div>
        ) : (
          <div className="space-y-3">
            <AnimatePresence>
              {accounts.map((acc, idx) => (
                <motion.div key={acc.id || idx}
                  className={`p-4 rounded-xl border ${isDark ? 'bg-white/[0.02] border-white/[0.06]' : 'bg-gray-50 border-gray-100'}`}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, x: -20 }}
                  transition={{ delay: idx * 0.05 }}
                >
                  <div className="flex items-center gap-4">
                    <div className="w-10 h-10 rounded-full bg-gradient-to-br from-blue-400 to-indigo-500
                      flex items-center justify-center text-white text-xs font-bold shrink-0">
                      {(acc.user_id || String(idx + 1)).slice(-2)}
                    </div>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-medium ${isDark ? 'text-white' : 'text-gray-800'}`}>
                          {acc.id || `Account ${idx + 1}`}
                        </span>
                        <span className={`px-2 py-0.5 rounded-md text-[10px] font-semibold ${
                          acc.active ? 'bg-emerald-500/15 text-emerald-500' : 'bg-red-500/15 text-red-500'
                        }`}>
                          {acc.active ? 'Active' : 'Disabled'}
                        </span>
                      </div>
                      <div className={`flex flex-wrap gap-x-4 gap-y-0.5 mt-1 text-xs ${isDark ? 'text-gray-400' : 'text-gray-500'}`}>
                        <span className="font-mono">user_id: {acc.user_id || '-'}</span>
                        <span className="font-mono">ph: {acc.ph || '-'}</span>
                        <span className="font-mono">token: {maskToken(acc.service_token)}</span>
                      </div>
                    </div>

                    <motion.button onClick={() => handleDelete(acc.id)}
                      className={`p-2 rounded-lg transition-colors ${isDark ? 'text-gray-500 hover:text-red-400 hover:bg-red-500/10' : 'text-gray-400 hover:text-red-500 hover:bg-red-50'}`}
                      whileHover={{ scale: 1.1 }} whileTap={{ scale: 0.9 }}>
                      <Trash2 className="w-4 h-4" />
                    </motion.button>
                  </div>
                </motion.div>
              ))}
            </AnimatePresence>
          </div>
        )}
      </motion.div>

      {/* Save */}
      <motion.div className="flex justify-end" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.4 }}>
        <motion.button onClick={handleSave} disabled={saving}
          className={`flex items-center gap-2 px-6 py-3 rounded-xl font-medium text-sm transition-all ${
            saved ? (isDark ? 'bg-green-500/20 text-green-400 border border-green-500/30' : 'bg-green-50 text-green-600 border border-green-200')
              : 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white hover:shadow-lg hover:shadow-blue-500/25'
          } disabled:opacity-50`}
          whileHover={{ scale: 1.02 }} whileTap={{ scale: 0.98 }}>
          {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : saved ? <Check className="w-4 h-4" /> : <Save className="w-4 h-4" />}
          {saving ? (lang === 'zh' ? '保存中...' : 'Saving...') : saved ? (lang === 'zh' ? '已保存' : 'Saved') : (lang === 'zh' ? '保存配置' : 'Save Config')}
        </motion.button>
      </motion.div>
    </div>
  )
}
