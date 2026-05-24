'use client'

import { useEffect, useState, useCallback } from 'react'
import { RefreshCw, Search, Terminal, AlertCircle, CheckCircle2 } from 'lucide-react'
import { RequestDetailModal } from '../components/RequestDetailModal'

type LogRow = {
  id: string
  ts: number
  model: string
  provider: string
  feature: string
  latency_ms: number
  status: string
  http_status: number
  prompt_tokens: number
  completion_tokens: number
  prompt_tokens_pre: number
  compaction_mode?: string
  saver_reduction_pct: number
  caveman_level?: string
  rtk_bytes_before?: number
  rtk_bytes_after?: number
  rtk_filters?: string
}

const fmtTime = (ts: number) => {
  if (!ts) return '-'
  const d = new Date(ts * 1000)
  return d.toLocaleTimeString('en-US', { hour12: false })
}

export default function RequestLogPage() {
  const [rows, setRows] = useState<LogRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [statusFilter, setStatusFilter] = useState('all')
  const [modelFilter, setModelFilter] = useState('')
  const [auto, setAuto] = useState(false)
  const [openId, setOpenId] = useState<string | null>(null)

  const fetchLogs = useCallback(async () => {
    setLoading(true)
    try {
      const qs = new URLSearchParams({ limit: '100' })
      if (statusFilter !== 'all') qs.set('status', statusFilter)
      if (modelFilter) qs.set('model', modelFilter)
      const res = await fetch(`/api/admin/request-logs?${qs.toString()}`)
      const data = await res.json()
      setRows(data.items || [])
      setTotal(data.total || 0)
    } catch {}
    setLoading(false)
  }, [statusFilter, modelFilter])

  useEffect(() => { fetchLogs() }, [fetchLogs])

  useEffect(() => {
    if (!auto) return
    const t = setInterval(fetchLogs, 5000)
    return () => clearInterval(t)
  }, [auto, fetchLogs])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500 to-fuchsia-500 flex items-center justify-center shadow-lg shadow-indigo-500/20">
            <Terminal className="w-5 h-5 text-white" />
          </div>
          <div>
            <h1 className="text-xl font-bold tracking-tight">Request Log</h1>
            <p className="text-xs text-muted-foreground">Chat / image / video requests. Newest first. {total} stored.</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <label className="inline-flex items-center gap-2 text-xs text-muted-foreground select-none cursor-pointer">
            <input type="checkbox" checked={auto} onChange={e => setAuto(e.target.checked)} className="accent-indigo-500" />
            Auto-refresh
          </label>
          <button
            onClick={fetchLogs}
            disabled={loading}
            className="inline-flex items-center gap-1.5 px-3 h-9 rounded-lg bg-muted/40 hover:bg-muted/60 text-xs font-semibold transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="h-9 px-3 rounded-lg bg-card border border-border/40 text-xs font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
        >
          <option value="all">All status</option>
          <option value="ok">OK</option>
          <option value="error">Error</option>
        </select>
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
          <input
            value={modelFilter}
            onChange={e => setModelFilter(e.target.value)}
            placeholder="Filter by model…"
            className="w-full h-9 pl-9 pr-3 rounded-lg bg-card border border-border/40 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500/30"
          />
        </div>
      </div>

      <div className="rounded-xl border border-border/40 bg-card overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-muted/30 text-muted-foreground uppercase text-[10px] tracking-wider">
            <tr>
              <th className="text-left px-4 py-2.5 font-semibold">Time</th>
              <th className="text-left px-4 py-2.5 font-semibold">Model</th>
              <th className="text-left px-4 py-2.5 font-semibold">Provider</th>
              <th className="text-left px-4 py-2.5 font-semibold">Feature</th>
              <th className="text-right px-4 py-2.5 font-semibold">Latency</th>
              <th className="text-center px-4 py-2.5 font-semibold">Status</th>
              <th className="text-right px-4 py-2.5 font-semibold">Tokens</th>
              <th className="text-right px-4 py-2.5 font-semibold">Saved</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr><td colSpan={8} className="text-center py-10 text-muted-foreground">No requests yet.</td></tr>
            ) : rows.map(r => (
              <tr
                key={r.id}
                onClick={() => setOpenId(r.id)}
                className="border-t border-border/20 hover:bg-muted/20 cursor-pointer transition-colors"
              >
                <td className="px-4 py-2.5 text-muted-foreground font-mono">{fmtTime(r.ts)}</td>
                <td className="px-4 py-2.5 font-mono text-foreground">{r.model || '—'}</td>
                <td className="px-4 py-2.5">
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/40 text-muted-foreground uppercase">{r.provider || '—'}</span>
                </td>
                <td className="px-4 py-2.5 text-muted-foreground">{r.feature || '—'}</td>
                <td className="px-4 py-2.5 text-right font-mono text-muted-foreground">{r.latency_ms}ms</td>
                <td className="px-4 py-2.5 text-center">
                  {r.status === 'ok' ? (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/10 text-emerald-400">
                      <CheckCircle2 className="w-3 h-3" /> ok
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold bg-rose-500/10 text-rose-400">
                      <AlertCircle className="w-3 h-3" /> error
                    </span>
                  )}
                </td>
                <td className="px-4 py-2.5 text-right font-mono text-muted-foreground">{r.prompt_tokens}→{r.completion_tokens}</td>
                <td className="px-4 py-2.5 text-right font-mono text-muted-foreground">
                  {(() => {
                    const before = r.rtk_bytes_before || 0
                    const after = r.rtk_bytes_after || 0
                    const tokSaved = before > after ? Math.round((before - after) / 4) : 0
                    if (!r.compaction_mode) return '—'
                    if (tokSaved > 0) return `−${tokSaved}`
                    if (r.caveman_level) return `cv:${r.caveman_level}`
                    return '—'
                  })()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {openId && <RequestDetailModal id={openId} onClose={() => setOpenId(null)} />}
    </div>
  )
}
