'use client'

import { useEffect, useState, useCallback } from 'react'
import { RefreshCw, Terminal, X } from 'lucide-react'

type Run = {
  id: string
  ts_start: number
  ts_end?: number
  trigger: string
  provider: string
  attempted: number
  succeeded: number
  failed: number
  status: string
}

type LogEntry = { timestamp?: string; level?: string; message?: string }
type RunDetail = Run & { logs?: LogEntry[]; logs_raw?: string }

const fmtTime = (ts: number) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

const fmtDuration = (start: number, end?: number) => {
  if (!start || !end) return '—'
  const ms = (end - start) * 1000
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

const statusColor = (s: string) => {
  if (s === 'completed') return 'bg-emerald-500/10 text-emerald-400'
  if (s === 'running') return 'bg-blue-500/10 text-blue-400'
  if (s === 'stopped') return 'bg-amber-500/10 text-amber-400'
  if (s === 'failed') return 'bg-rose-500/10 text-rose-400'
  return 'bg-muted/40 text-muted-foreground'
}

const levelColor = (lvl?: string) => {
  switch (lvl) {
    case 'error': return 'text-rose-400'
    case 'warning': return 'text-amber-400'
    case 'success': return 'text-emerald-400'
    default: return 'text-muted-foreground'
  }
}

function RunDetailModal({ id, onClose }: { id: string; onClose: () => void }) {
  const [data, setData] = useState<RunDetail | null>(null)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    fetch(`/api/admin/auto-script-runs/${id}`).then(r => r.json()).then(setData).finally(() => setLoading(false))
  }, [id])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4" onClick={onClose}>
      <div className="relative w-full max-w-3xl max-h-[90vh] flex flex-col bg-card border border-border/40 rounded-2xl shadow-2xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-border/30">
          <div className="flex items-center gap-3">
            <span className="font-mono text-sm">{data?.provider || '—'}</span>
            {data?.status && <span className={`text-[10px] px-1.5 py-0.5 rounded font-semibold ${statusColor(data.status)}`}>{data.status}</span>}
            <span className="text-[10px] text-muted-foreground">{data?.trigger}</span>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground"><X className="w-5 h-5" /></button>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4 custom-scrollbar">
          {loading && <div className="text-sm text-muted-foreground">Loading…</div>}
          {data && (
            <>
              <div className="grid grid-cols-4 gap-3 text-xs">
                <div><div className="text-[10px] uppercase text-muted-foreground">Started</div><div>{fmtTime(data.ts_start)}</div></div>
                <div><div className="text-[10px] uppercase text-muted-foreground">Duration</div><div>{fmtDuration(data.ts_start, data.ts_end)}</div></div>
                <div><div className="text-[10px] uppercase text-muted-foreground">Attempted</div><div className="font-bold">{data.attempted}</div></div>
                <div><div className="text-[10px] uppercase text-muted-foreground">Success / Fail</div><div className="font-bold"><span className="text-emerald-400">{data.succeeded}</span> / <span className="text-rose-400">{data.failed}</span></div></div>
              </div>

              <div>
                <div className="text-[10px] uppercase text-muted-foreground mb-1.5">Run log</div>
                <div className="rounded-lg border border-border/40 bg-background/50 p-3 text-[11px] max-h-96 overflow-auto custom-scrollbar font-mono space-y-0.5">
                  {data.logs && data.logs.length > 0 ? (
                    data.logs.map((l, i) => (
                      <div key={i} className="flex gap-2">
                        <span className="text-muted-foreground/60 shrink-0">{l.timestamp ? new Date(l.timestamp).toLocaleTimeString('en-US', { hour12: false }) : ''}</span>
                        <span className={`shrink-0 ${levelColor(l.level)}`}>[{l.level || 'info'}]</span>
                        <span className="text-foreground/90 break-all">{l.message}</span>
                      </div>
                    ))
                  ) : data.logs_raw ? (
                    <pre className="whitespace-pre-wrap break-all">{data.logs_raw}</pre>
                  ) : (
                    <div className="text-muted-foreground">No logs recorded.</div>
                  )}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

export default function AutoScriptPage() {
  const [rows, setRows] = useState<Run[]>([])
  const [loading, setLoading] = useState(false)
  const [openId, setOpenId] = useState<string | null>(null)

  const fetchRuns = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch('/api/admin/auto-script-runs?limit=50')
      const data = await res.json()
      setRows(data.items || [])
    } catch {}
    setLoading(false)
  }, [])

  useEffect(() => { fetchRuns() }, [fetchRuns])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500 to-fuchsia-500 flex items-center justify-center shadow-lg shadow-indigo-500/20">
            <Terminal className="w-5 h-5 text-white" />
          </div>
          <div>
            <h1 className="text-xl font-bold tracking-tight">Auto Script Log</h1>
            <p className="text-xs text-muted-foreground">Batch registration / auto-replenish runs. {rows.length} stored.</p>
          </div>
        </div>
        <button
          onClick={fetchRuns}
          disabled={loading}
          className="inline-flex items-center gap-1.5 px-3 h-9 rounded-lg bg-muted/40 hover:bg-muted/60 text-xs font-semibold transition-colors"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      <div className="rounded-xl border border-border/40 bg-card overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-muted/30 text-muted-foreground uppercase text-[10px] tracking-wider">
            <tr>
              <th className="text-left px-4 py-2.5 font-semibold">Started</th>
              <th className="text-left px-4 py-2.5 font-semibold">Provider</th>
              <th className="text-left px-4 py-2.5 font-semibold">Trigger</th>
              <th className="text-right px-4 py-2.5 font-semibold">Attempted</th>
              <th className="text-right px-4 py-2.5 font-semibold">Succeeded</th>
              <th className="text-right px-4 py-2.5 font-semibold">Failed</th>
              <th className="text-right px-4 py-2.5 font-semibold">Duration</th>
              <th className="text-center px-4 py-2.5 font-semibold">Status</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr><td colSpan={8} className="text-center py-10 text-muted-foreground">No auto-script runs yet.</td></tr>
            ) : rows.map(r => (
              <tr
                key={r.id}
                onClick={() => setOpenId(r.id)}
                className="border-t border-border/20 hover:bg-muted/20 cursor-pointer transition-colors"
              >
                <td className="px-4 py-2.5 text-muted-foreground font-mono">{fmtTime(r.ts_start)}</td>
                <td className="px-4 py-2.5">
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/40 text-muted-foreground uppercase">{r.provider || '—'}</span>
                </td>
                <td className="px-4 py-2.5 text-muted-foreground">{r.trigger}</td>
                <td className="px-4 py-2.5 text-right font-mono">{r.attempted}</td>
                <td className="px-4 py-2.5 text-right font-mono text-emerald-400">{r.succeeded}</td>
                <td className="px-4 py-2.5 text-right font-mono text-rose-400">{r.failed}</td>
                <td className="px-4 py-2.5 text-right font-mono text-muted-foreground">{fmtDuration(r.ts_start, r.ts_end)}</td>
                <td className="px-4 py-2.5 text-center">
                  <span className={`text-[10px] px-1.5 py-0.5 rounded font-semibold ${statusColor(r.status)}`}>{r.status}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {openId && <RunDetailModal id={openId} onClose={() => setOpenId(null)} />}
    </div>
  )
}
