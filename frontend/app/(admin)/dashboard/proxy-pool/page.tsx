'use client'

import { useState, useEffect, useCallback } from 'react'
import { Globe, RefreshCw, Plus, Trash2, Play, CheckCircle2, XCircle, AlertCircle, Network, Skull } from 'lucide-react'
import { toast } from 'sonner'

type ProxyStatus = 'live' | 'dead' | 'checking' | 'unknown'

type Proxy = {
  id: string
  enabled: boolean
  type: string
  host: string
  port: number
  username: string
  password: string
  status: ProxyStatus
  region: string
  latency_ms: number
  last_checked: string
  created_at: string
}

type ProxyApplyTo = {
  batch_registration: boolean
  provider_call: boolean
}

type ProxyConfig = {
  enabled: boolean
  test_endpoint: string
  rotation_strategy: string
  auto_test_interval: number
  auto_delete_failed: boolean
  fallback_direct: boolean
  auto_login: boolean
  apply_to: ProxyApplyTo
}

const defaultConfig: ProxyConfig = {
  enabled: false,
  test_endpoint: 'https://google.com',
  rotation_strategy: 'fastest',
  auto_test_interval: 5,
  auto_delete_failed: false,
  fallback_direct: true,
  auto_login: false,
  apply_to: { batch_registration: true, provider_call: false },
}

const StatusBadge = ({ status }: { status: ProxyStatus }) => {
  const config = {
    live: { icon: CheckCircle2, color: 'text-emerald-400', bg: 'bg-emerald-500/10', label: 'Live' },
    dead: { icon: XCircle, color: 'text-rose-400', bg: 'bg-rose-500/10', label: 'Dead' },
    checking: { icon: RefreshCw, color: 'text-amber-400', bg: 'bg-amber-500/10', label: 'Checking' },
    unknown: { icon: AlertCircle, color: 'text-muted-foreground', bg: 'bg-muted/20', label: 'Unknown' },
  }[status]

  const Icon = config.icon
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold ${config.bg} ${config.color}`}>
      <Icon className={`w-3 h-3 ${status === 'checking' ? 'animate-spin' : ''}`} />
      {config.label}
    </span>
  )
}

export default function ProxyPoolPage() {
  const [proxies, setProxies] = useState<Proxy[]>([])
  const [config, setConfig] = useState<ProxyConfig>(defaultConfig)
  const [loading, setLoading] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const fetchProxies = useCallback(async () => {
    try {
      const res = await fetch('/api/admin/proxy')
      const data = await res.json()
      setProxies(data.proxies || [])
    } catch {}
  }, [])

  const fetchConfig = useCallback(async () => {
    try {
      const res = await fetch('/api/admin/proxy/config')
      const data = await res.json()
      if (data.config) {
        setConfig({
          ...defaultConfig,
          ...data.config,
          apply_to: { ...defaultConfig.apply_to, ...(data.config.apply_to || {}) },
        })
      }
    } catch {}
  }, [])

  useEffect(() => { fetchProxies(); fetchConfig() }, [fetchProxies, fetchConfig])

  const persistConfig = useCallback(async (next: ProxyConfig) => {
    try {
      const res = await fetch('/api/admin/proxy/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(next),
      })
      const data = await res.json()
      if (!data.ok) throw new Error('save failed')
    } catch {
      toast.error('Failed to save config')
    }
  }, [])

  const toggleApplyTo = (key: keyof ProxyApplyTo, value: boolean) => {
    const next: ProxyConfig = { ...config, apply_to: { ...config.apply_to, [key]: value } }
    setConfig(next)
    void persistConfig(next)
  }

  const toggleConfigFlag = (key: 'enabled' | 'auto_delete_failed' | 'fallback_direct' | 'auto_login', value: boolean) => {
    const next: ProxyConfig = { ...config, [key]: value }
    setConfig(next)
    void persistConfig(next)
  }

  const runImport = useCallback(async (raw: string) => {
    const urls = raw.split('\n').map(s => s.trim()).filter(Boolean)
    if (urls.length === 0) {
      toast.error('No proxies to import')
      return
    }
    setLoading(true)
    try {
      const res = await fetch('/api/admin/proxy/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ raw_urls: urls }),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Imported ${data.imported} proxies${data.failed > 0 ? ` (${data.failed} failed)` : ''}`)
        fetchProxies()
      } else {
        toast.error(data.error || 'Import failed')
      }
    } catch {
      toast.error('Import request failed')
    } finally {
      setLoading(false)
    }
  }, [fetchProxies])

  const openImportToast = () => {
    toast.custom((id) => (
      <ImportToast
        onSubmit={async (raw) => { toast.dismiss(id); await runImport(raw) }}
        onCancel={() => toast.dismiss(id)}
      />
    ), { duration: Infinity })
  }

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      await fetch(`/api/admin/proxy/${id}/toggle`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      })
      fetchProxies()
    } catch {
      toast.error('Failed to toggle proxy')
    }
  }

  const handleTest = async (id: string) => {
    try {
      const res = await fetch(`/api/admin/proxy/${id}/test`, { method: 'POST' })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Test completed: ${data.result.status} (${data.result.latency_ms}ms)`)
        fetchProxies()
      }
    } catch {
      toast.error('Test failed')
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await fetch(`/api/admin/proxy/${id}`, { method: 'DELETE' })
      toast.success('Proxy deleted')
      setSelectedIds(prev => { const next = new Set(prev); next.delete(id); return next })
      fetchProxies()
    } catch {
      toast.error('Delete failed')
    }
  }

  const handleTestAll = async () => {
    setLoading(true)
    try {
      const res = await fetch('/api/admin/proxy/test-all', { method: 'POST' })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Tested all proxies: ${data.results.filter((r: any) => r.status === 'live').length} live`)
        fetchProxies()
      }
    } catch {
      toast.error('Test all failed')
    } finally {
      setLoading(false)
    }
  }

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return
    try {
      const res = await fetch('/api/admin/proxy/batch', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: Array.from(selectedIds) }),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Deleted ${data.deleted} proxies`)
        setSelectedIds(new Set())
        fetchProxies()
      }
    } catch {
      toast.error('Batch delete failed')
    }
  }

  const handleDeleteDead = async () => {
    setLoading(true)
    try {
      const res = await fetch('/api/admin/proxy/dead', { method: 'DELETE' })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Deleted ${data.deleted} dead proxies`)
        fetchProxies()
      } else {
        toast.error(data.error || 'Delete dead failed')
      }
    } catch {
      toast.error('Delete dead failed')
    } finally {
      setLoading(false)
    }
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === proxies.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(proxies.map(p => p.id)))
    }
  }

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const liveCount = proxies.filter(p => p.status === 'live').length

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500/20 to-fuchsia-500/20 flex items-center justify-center">
            <Network className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h1 className="text-2xl font-black tracking-tight">Proxy Pool</h1>
            <p className="text-sm text-muted-foreground mt-0.5">{proxies.length} total &middot; {liveCount} live</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={openImportToast}
            disabled={loading}
            className="h-9 px-4 rounded-xl bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold transition-all flex items-center gap-2 disabled:opacity-50"
          >
            <Plus className="w-3.5 h-3.5" />
            Import
          </button>
          <button
            onClick={handleTestAll}
            disabled={loading || proxies.length === 0}
            className="h-9 px-4 rounded-xl border border-border/50 hover:bg-muted/20 text-xs font-bold transition-all flex items-center gap-2 disabled:opacity-50"
          >
            <Play className="w-3.5 h-3.5" />
            Test All
          </button>
          {!config.auto_delete_failed && (
            <button
              onClick={handleDeleteDead}
              disabled={loading || proxies.length === 0}
              className="h-9 px-4 rounded-xl border border-border/50 hover:bg-rose-500/10 hover:text-rose-400 text-xs font-bold transition-all flex items-center gap-2 disabled:opacity-50"
            >
              <Skull className="w-3.5 h-3.5" />
              Delete Dead
            </button>
          )}
        </div>
      </div>

      <div className="flex items-center gap-x-6 gap-y-2 px-1 flex-wrap">
        <span className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Config</span>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.enabled}
            onChange={e => toggleConfigFlag('enabled', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Enable Pool</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.auto_delete_failed}
            onChange={e => toggleConfigFlag('auto_delete_failed', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Auto-Delete Failed</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.fallback_direct}
            onChange={e => toggleConfigFlag('fallback_direct', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Fallback Direct</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.auto_login}
            onChange={e => toggleConfigFlag('auto_login', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Auto-Login</span>
        </label>
        <span className="text-xs font-bold text-muted-foreground uppercase tracking-wider ml-4">Apply To</span>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.apply_to.batch_registration}
            onChange={e => toggleApplyTo('batch_registration', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Batch Registration</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.apply_to.provider_call}
            onChange={e => toggleApplyTo('provider_call', e.target.checked)}
            className="w-3.5 h-3.5"
          />
          <span className="text-xs font-medium">Provider Calls</span>
        </label>
      </div>

      {selectedIds.size > 0 && (
        <div className="flex items-center gap-2 px-1">
          <span className="text-xs text-muted-foreground">{selectedIds.size} selected</span>
          <button onClick={handleBatchDelete} className="h-8 px-3 rounded-lg bg-rose-500/10 text-rose-400 text-xs font-bold hover:bg-rose-500/20 transition-all flex items-center gap-1.5">
            <Trash2 className="w-3 h-3" />
            Delete Selected
          </button>
        </div>
      )}

      <div className="bg-card border border-border/50 rounded-2xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/30">
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider w-10">
                  <input type="checkbox" checked={proxies.length > 0 && selectedIds.size === proxies.length} onChange={toggleSelectAll} className="w-3.5 h-3.5" />
                </th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">On/Off</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Status</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Type</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Region</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Host</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Port</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Latency</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Last Checked</th>
                <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {proxies.length === 0 ? (
                <tr>
                  <td colSpan={10} className="p-12 text-center">
                    <Globe className="w-8 h-8 mx-auto text-muted-foreground/40 mb-3" />
                    <p className="text-sm text-muted-foreground font-medium">No proxies yet</p>
                    <p className="text-xs text-muted-foreground/60 mt-1">Click Import to paste proxy URLs</p>
                  </td>
                </tr>
              ) : (
                proxies.map(proxy => (
                  <tr key={proxy.id} className="border-b border-border/10 hover:bg-muted/5 transition-colors">
                    <td className="p-3">
                      <input type="checkbox" checked={selectedIds.has(proxy.id)} onChange={() => toggleSelect(proxy.id)} className="w-3.5 h-3.5" />
                    </td>
                    <td className="p-3">
                      <button
                        onClick={() => handleToggle(proxy.id, !proxy.enabled)}
                        className={`w-9 h-5 rounded-full transition-all relative ${proxy.enabled ? 'bg-indigo-500' : 'bg-muted/30'}`}
                      >
                        <div className={`w-3.5 h-3.5 rounded-full bg-white absolute top-0.5 transition-all ${proxy.enabled ? 'left-[18px]' : 'left-[3px]'}`} />
                      </button>
                    </td>
                    <td className="p-3"><StatusBadge status={proxy.status} /></td>
                    <td className="p-3">
                      <span className="text-xs font-mono font-semibold uppercase">{proxy.type}</span>
                    </td>
                    <td className="p-3">
                      <span className="text-xs font-semibold">{proxy.region || '--'}</span>
                    </td>
                    <td className="p-3">
                      <span className="text-xs font-mono">{proxy.host}</span>
                    </td>
                    <td className="p-3">
                      <span className="text-xs font-mono">{proxy.port}</span>
                    </td>
                    <td className="p-3">
                      <span className={`text-xs font-mono font-semibold ${proxy.latency_ms > 0 && proxy.latency_ms < 500 ? 'text-emerald-400' : proxy.latency_ms >= 500 ? 'text-amber-400' : 'text-muted-foreground'}`}>
                        {proxy.latency_ms > 0 ? `${proxy.latency_ms}ms` : '--'}
                      </span>
                    </td>
                    <td className="p-3">
                      <span className="text-xs text-muted-foreground">
                        {proxy.last_checked ? new Date(proxy.last_checked).toLocaleTimeString() : '--'}
                      </span>
                    </td>
                    <td className="p-3">
                      <div className="flex items-center gap-1">
                        <button onClick={() => handleTest(proxy.id)} className="h-7 w-7 rounded-lg hover:bg-indigo-500/10 text-indigo-400 flex items-center justify-center transition-all" title="Test">
                          <Play className="w-3 h-3" />
                        </button>
                        <button onClick={() => handleDelete(proxy.id)} className="h-7 w-7 rounded-lg hover:bg-rose-500/10 text-rose-400 flex items-center justify-center transition-all" title="Delete">
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function ImportToast({ onSubmit, onCancel }: { onSubmit: (raw: string) => void; onCancel: () => void }) {
  const [value, setValue] = useState('')
  return (
    <div className="w-[420px] max-w-[90vw] bg-card border border-border/60 rounded-2xl p-4 shadow-2xl space-y-3">
      <div>
        <p className="text-sm font-bold">Import Proxies</p>
        <p className="text-[11px] text-muted-foreground mt-0.5">One per line. Schemes: http://, https://, socks5://</p>
      </div>
      <textarea
        autoFocus
        value={value}
        onChange={e => setValue(e.target.value)}
        placeholder={`http://user:pass@1.2.3.4:8080\nsocks5://9.10.11.12:1080`}
        className="w-full h-28 px-3 py-2 rounded-xl bg-muted/20 border border-border/50 text-xs font-mono resize-none outline-none focus:border-indigo-500/50"
      />
      <div className="flex justify-end gap-2">
        <button
          onClick={onCancel}
          className="h-8 px-3 rounded-lg border border-border/50 text-xs font-bold hover:bg-muted/20 transition-all"
        >
          Cancel
        </button>
        <button
          onClick={() => onSubmit(value)}
          disabled={!value.trim()}
          className="h-8 px-4 rounded-lg bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold transition-all disabled:opacity-50"
        >
          Import
        </button>
      </div>
    </div>
  )
}
