'use client'

import { useEffect, useState } from 'react'
import { X, Copy, Check, AlertCircle, CheckCircle2, Clock, Cpu, Box, User } from 'lucide-react'
import { toast } from 'sonner'

type Detail = {
  id: string
  ts: number
  model: string
  provider: string
  feature: string
  account?: string
  latency_ms: number
  status: string
  http_status: number
  error_message?: string
  request_body?: string
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
  return d.toLocaleString()
}

const fmtBytes = (n: number) => {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}

const prettyJSON = (raw: string) => {
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

export function RequestDetailModal({ id, onClose }: { id: string; onClose: () => void }) {
  const [data, setData] = useState<Detail | null>(null)
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    fetch(`/api/admin/request-logs/${id}`)
      .then(r => r.json())
      .then(d => { if (!cancelled) setData(d) })
      .catch(() => {})
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [id])

  const copy = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      toast.success('Copied to clipboard')
      setTimeout(() => setCopied(false), 1500)
    })
  }

  const isError = data?.status === 'error'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4" onClick={onClose}>
      <div className="relative w-full max-w-3xl max-h-[90vh] flex flex-col bg-card border border-border/40 rounded-2xl shadow-2xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-border/30">
          <div className="flex items-center gap-3">
            {isError ? (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-semibold bg-rose-500/10 text-rose-400 ring-1 ring-rose-500/30">
                <AlertCircle className="w-3.5 h-3.5" /> error
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-semibold bg-emerald-500/10 text-emerald-400 ring-1 ring-emerald-500/30">
                <CheckCircle2 className="w-3.5 h-3.5" /> ok
              </span>
            )}
            <span className="font-mono text-sm text-foreground">{data?.model || '—'}</span>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/40 text-muted-foreground uppercase">{data?.provider || '—'}</span>
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/40 text-muted-foreground">{data?.feature || '—'}</span>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4 custom-scrollbar">
          {loading && <div className="text-sm text-muted-foreground">Loading…</div>}

          {data && (
            <>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
                <div className="space-y-1">
                  <div className="text-[10px] uppercase text-muted-foreground flex items-center gap-1"><Clock className="w-3 h-3" /> Time</div>
                  <div className="text-foreground">{fmtTime(data.ts)}</div>
                </div>
                <div className="space-y-1">
                  <div className="text-[10px] uppercase text-muted-foreground">HTTP</div>
                  <div className={`font-mono ${data.http_status >= 400 ? 'text-rose-400' : 'text-emerald-400'}`}>{data.http_status || '—'}</div>
                </div>
                <div className="space-y-1">
                  <div className="text-[10px] uppercase text-muted-foreground">Latency</div>
                  <div className="text-foreground">{data.latency_ms}ms</div>
                </div>
                <div className="space-y-1">
                  <div className="text-[10px] uppercase text-muted-foreground flex items-center gap-1"><User className="w-3 h-3" /> Account</div>
                  <div className="text-foreground truncate" title={data.account}>{data.account || '—'}</div>
                </div>
              </div>

              {isError && data.error_message && (
                <div>
                  <div className="text-[10px] uppercase text-muted-foreground mb-1.5">Error</div>
                  <div className="rounded-lg border border-rose-500/30 bg-rose-500/5 p-3 text-xs text-rose-300 font-mono whitespace-pre-wrap break-all">
                    {data.error_message}
                  </div>
                </div>
              )}

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <div className="text-[10px] uppercase text-muted-foreground flex items-center gap-1"><Box className="w-3 h-3" /> Request body</div>
                  <button
                    onClick={() => copy(data.request_body || '')}
                    className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-semibold bg-muted/40 hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                    {copied ? 'Copied' : 'Copy'}
                  </button>
                </div>
                <pre className="rounded-lg border border-border/40 bg-background/50 p-3 text-[11px] text-foreground/80 max-h-64 overflow-auto custom-scrollbar font-mono whitespace-pre-wrap break-all">
                  {prettyJSON(data.request_body || '') || '(empty body)'}
                </pre>
              </div>

              <div>
                <div className="text-[10px] uppercase text-muted-foreground mb-1.5 flex items-center gap-1"><Cpu className="w-3 h-3" /> Token breakdown</div>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
                  <div className="rounded-lg border border-border/40 bg-muted/20 p-2.5">
                    <div className="text-[10px] uppercase text-muted-foreground">Prompt (sent)</div>
                    <div className="text-base font-bold text-foreground">{data.prompt_tokens.toLocaleString()}</div>
                  </div>
                  <div className="rounded-lg border border-border/40 bg-muted/20 p-2.5">
                    <div className="text-[10px] uppercase text-muted-foreground">Completion</div>
                    <div className="text-base font-bold text-foreground">{data.completion_tokens.toLocaleString()}</div>
                  </div>
                  <div className="rounded-lg border border-border/40 bg-muted/20 p-2.5">
                    <div className="text-[10px] uppercase text-muted-foreground">Total</div>
                    <div className="text-base font-bold text-foreground">{(data.prompt_tokens + data.completion_tokens).toLocaleString()}</div>
                  </div>
                  {(() => {
                    const before = data.rtk_bytes_before || 0
                    const after = data.rtk_bytes_after || 0
                    const tokSaved = before > after ? Math.round((before - after) / 4) : 0
                    const active = !!data.compaction_mode && (tokSaved > 0 || !!data.caveman_level)
                    if (!active) {
                      return (
                        <div className="rounded-lg border border-border/40 bg-muted/20 p-2.5">
                          <div className="text-[10px] uppercase text-muted-foreground">Saver</div>
                          <div className="text-base font-bold text-muted-foreground">—</div>
                        </div>
                      )
                    }
                    return (
                      <div className="rounded-lg border border-violet-500/40 bg-violet-500/10 p-2.5 ring-1 ring-violet-400/30">
                        <div className="text-[10px] uppercase text-violet-300">Saver ({data.compaction_mode})</div>
                        <div className="text-base font-bold text-violet-300">
                          {tokSaved > 0 ? `−${tokSaved} tok` : (data.caveman_level ? `cv:${data.caveman_level}` : '—')}
                        </div>
                        {tokSaved > 0 && (
                          <div className="text-[10px] text-violet-300/70">−{data.saver_reduction_pct}% bytes</div>
                        )}
                      </div>
                    )
                  })()}
                </div>
                <div className="mt-2 text-[10px] text-muted-foreground italic">
                  Credit used (proxy: completion tokens) — providers don&apos;t expose cost yet.
                </div>
              </div>

              {(() => {
                const filters = (data.rtk_filters || '').split(',').map(s => s.trim()).filter(Boolean)
                const rtkBefore = data.rtk_bytes_before || 0
                const rtkAfter = data.rtk_bytes_after || 0
                const rtkSaved = rtkBefore - rtkAfter
                const rtkPct = rtkBefore > 0 ? Math.round((rtkSaved / rtkBefore) * 100) : 0
                const showRTK = filters.length > 0 || rtkBefore > 0
                const showCaveman = !!data.caveman_level
                if (!showRTK && !showCaveman) return null
                return (
                  <div>
                    <div className="text-[10px] uppercase text-muted-foreground mb-1.5">Compression</div>
                    <div className="space-y-2">
                      {showRTK && (
                        <div className="rounded-lg border border-violet-500/30 bg-violet-500/5 p-3">
                          <div className="flex items-center justify-between mb-1.5">
                            <span className="text-[11px] font-semibold text-violet-400">RTK</span>
                            {rtkBefore > 0 && (
                              <span className="text-[11px] font-mono text-violet-300">
                                {fmtBytes(rtkBefore)} → {fmtBytes(rtkAfter)} (−{rtkPct}%)
                              </span>
                            )}
                          </div>
                          {filters.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                              {filters.map(f => (
                                <span key={f} className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono bg-violet-500/15 text-violet-300 ring-1 ring-violet-500/30">
                                  {f}
                                </span>
                              ))}
                            </div>
                          ) : (
                            <div className="text-[10px] text-muted-foreground">No filters fired (input below safety threshold).</div>
                          )}
                        </div>
                      )}
                      {showCaveman && (
                        <div className="rounded-lg border border-fuchsia-500/30 bg-fuchsia-500/5 p-3 flex items-center gap-2">
                          <span className="text-[11px] font-semibold text-fuchsia-400">Caveman</span>
                          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono uppercase bg-fuchsia-500/15 text-fuchsia-300 ring-1 ring-fuchsia-500/30">
                            {data.caveman_level}
                          </span>
                          <span className="text-[10px] text-muted-foreground">
                            Terse-style instruction injected into system prompt.
                          </span>
                        </div>
                      )}
                    </div>
                  </div>
                )
              })()}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
