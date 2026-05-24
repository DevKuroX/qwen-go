'use client'

import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import {
  Bot, Globe, RefreshCw, Plus, Trash2, Play, StopCircle, Settings2,
  CheckCircle2, XCircle, AlertCircle, Network, Globe2, Filter, Upload, Download, ArrowRight,
} from 'lucide-react'
import { toast } from 'sonner'

type ScrapedProxy = {
  id: string
  type: string
  host: string
  port: number
  status: string
  latency_ms: number
  country: string
  isp: string
  source: string
}

type ProxySource = {
  url: string
  enabled: boolean
}

type ScrapeJob = {
  id: string
  status: 'running' | 'completed' | 'stopped' | 'failed'
  sources: string[]
  total: number
  found: number
  alive: number
  imported: number
  failed: number
  logs: { level: string; message: string; time: string }[]
  started_at: string
  completed_at?: string
}

function lsGet(key: string, fallback: string = ''): string {
  if (typeof window === 'undefined') return fallback
  return localStorage.getItem(key) || fallback
}

const StatusBadge = ({ status }: { status: string }) => {
  const config = {
    live: { icon: CheckCircle2, color: 'text-emerald-400', bg: 'bg-emerald-500/10', label: 'Live' },
    dead: { icon: XCircle, color: 'text-rose-400', bg: 'bg-rose-500/10', label: 'Dead' },
    checking: { icon: RefreshCw, color: 'text-amber-400', bg: 'bg-amber-500/10', label: 'Checking' },
  }[status] || { icon: AlertCircle, color: 'text-muted-foreground', bg: 'bg-muted/20', label: status }

  const Icon = config.icon
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold ${config.bg} ${config.color}`}>
      <Icon className={`w-3 h-3 ${status === 'checking' ? 'animate-spin' : ''}`} />
      {config.label}
    </span>
  )
}

export default function ProxyScraperPage() {
  const [sources, setSources] = useState<ProxySource[]>([])
  const [customUrl, setCustomUrl] = useState('')
  const [showSources, setShowSources] = useState(false)
  const [sourcesLoading, setSourcesLoading] = useState(false)

  const [batchCount, setBatchCount] = useState(() => Number(lsGet('proxy_scraper_batch_count', '10')))
  const [running, setRunning] = useState(false)
  const [starting, setStarting] = useState(false)
  const [jobId, setJobId] = useState<string | null>(null)
  const [job, setJob] = useState<ScrapeJob | null>(null)

  const [logs, setLogs] = useState<{ time: string; message: string; level: string }[]>([])
  const [autoScroll, setAutoScroll] = useState(true)
  const logContainerRef = useRef<HTMLDivElement>(null)

  const [stagingProxies, setStagingProxies] = useState<ScrapedProxy[]>([])
  const [stagingTotal, setStagingTotal] = useState(0)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [filterType, setFilterType] = useState('')
  const [filterCountry, setFilterCountry] = useState('')
  const [filterMaxLatency, setFilterMaxLatency] = useState('')
  const [transferring, setTransferring] = useState(false)
  const [stagingLoading, setStagingLoading] = useState(false)

  const countries = useMemo(() => {
    const c = new Set<string>()
    stagingProxies.forEach(p => { if (p.country) c.add(p.country) })
    return Array.from(c).sort()
  }, [stagingProxies])

  const geoEnriched = useMemo(() => stagingProxies.filter(p => p.country).length, [stagingProxies])
  const aliveCount = useMemo(() => stagingProxies.filter(p => p.status === 'live').length, [stagingProxies])

  const fetchSources = useCallback(async () => {
    try {
      const res = await fetch('/api/admin/proxy-scraper/sources')
      const data = await res.json()
      setSources(data.sources || [])
    } catch { }
  }, [])

  const fetchStaging = useCallback(async () => {
    setStagingLoading(true)
    try {
      const params = new URLSearchParams()
      if (filterType) params.set('type', filterType)
      if (filterCountry) params.set('country', filterCountry)
      if (filterMaxLatency) params.set('max_latency', filterMaxLatency)
      const res = await fetch(`/api/admin/proxy-scraper/staging?${params}`)
      const data = await res.json()
      setStagingProxies(data.proxies || [])
      setStagingTotal(data.total || 0)
    } catch {
      toast.error('Failed to fetch staging data')
    } finally {
      setStagingLoading(false)
    }
  }, [filterType, filterCountry, filterMaxLatency])

  useEffect(() => { fetchSources() }, [fetchSources])

  useEffect(() => { fetchStaging() }, [fetchStaging])

  useEffect(() => {
    localStorage.setItem('proxy_scraper_batch_count', batchCount.toString())
  }, [batchCount])

  useEffect(() => {
    if (autoScroll && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [logs, autoScroll])

  useEffect(() => {
    if (!jobId || !running) return
    const intv = setInterval(async () => {
      try {
        const res = await fetch(`/api/admin/proxy-scraper/${jobId}`)
        const data = await res.json()
        setJob(data)
        if (data.status !== 'running') {
          setRunning(false)
          toast.success(`Scrape ${data.status}: ${data.found} found, ${data.alive} alive, ${data.imported} imported`)
          fetchStaging()
        }
      } catch { }
    }, 2000)
    return () => clearInterval(intv)
  }, [jobId, running, fetchStaging])

  useEffect(() => {
    if (!jobId || !running) return
    const es = new EventSource(`/api/admin/proxy-scraper/${jobId}/logs`)
    es.onmessage = (e) => {
      try {
        const parsed = JSON.parse(e.data)
        setLogs(prev => [...prev, {
          time: parsed.time || new Date().toISOString(),
          message: parsed.message || e.data,
          level: parsed.level || 'info',
        }])
      } catch {
        setLogs(prev => [...prev, { time: new Date().toISOString(), message: e.data, level: 'info' }])
      }
    }
    es.onerror = () => { }
    return () => es.close()
  }, [jobId, running])

  const handleStart = async () => {
    setStarting(true)
    setRunning(true)
    setLogs([])
    setJob(null)
    setJobId(null)
    try {
      const enabledUrls = sources.filter(s => s.enabled).map(s => s.url)
      const res = await fetch('/api/admin/proxy-scraper/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sources: enabledUrls, count: batchCount }),
      })
      const data = await res.json()
      if (data.ok) {
        setJobId(data.job_id)
        toast.success('Scrape started')
      } else {
        toast.error(data.error || 'Failed to start scrape')
        setRunning(false)
      }
    } catch {
      toast.error('Failed to start scrape')
      setRunning(false)
    } finally {
      setStarting(false)
    }
  }

  const handleStop = async () => {
    if (!jobId) return
    try {
      await fetch(`/api/admin/proxy-scraper/${jobId}/stop`, { method: 'POST' })
      toast.success('Scrape stopped')
    } catch {
      toast.error('Failed to stop scrape')
    }
  }

  const handleSaveSources = async () => {
    setSourcesLoading(true)
    try {
      const res = await fetch('/api/admin/proxy-scraper/sources', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sources }),
      })
      const data = await res.json()
      if (data.ok) toast.success('Sources saved')
      else toast.error(data.error || 'Failed to save sources')
    } catch {
      toast.error('Failed to save sources')
    } finally {
      setSourcesLoading(false)
    }
  }

  const handleAddSource = () => {
    const url = customUrl.trim()
    if (!url) return
    if (sources.some(s => s.url === url)) {
      toast.error('URL already exists')
      return
    }
    setSources(prev => [...prev, { url, enabled: true }])
    setCustomUrl('')
  }

  const handleToggleSource = (url: string) => {
    setSources(prev => prev.map(s => s.url === url ? { ...s, enabled: !s.enabled } : s))
  }

  const handleRemoveSource = (url: string) => {
    setSources(prev => prev.filter(s => s.url !== url))
  }

  const handleTransferSelected = async () => {
    if (selectedIds.size === 0) return
    setTransferring(true)
    try {
      const res = await fetch('/api/admin/proxy-scraper/transfer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: Array.from(selectedIds) }),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Transferred ${data.imported} proxies to pool`)
        setSelectedIds(new Set())
        fetchStaging()
      } else {
        toast.error(data.error || 'Transfer failed')
      }
    } catch {
      toast.error('Transfer request failed')
    } finally {
      setTransferring(false)
    }
  }

  const handleTransferAll = async () => {
    setTransferring(true)
    try {
      const body: Record<string, string | number> = {}
      if (filterType) body.type = filterType
      if (filterCountry) body.country = filterCountry
      if (filterMaxLatency) body.max_latency = parseInt(filterMaxLatency)
      const res = await fetch('/api/admin/proxy-scraper/transfer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Transferred ${data.imported} proxies to pool`)
        setSelectedIds(new Set())
        fetchStaging()
      } else {
        toast.error(data.error || 'Transfer failed')
      }
    } catch {
      toast.error('Transfer request failed')
    } finally {
      setTransferring(false)
    }
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === stagingProxies.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(stagingProxies.map(p => p.id)))
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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500/20 to-fuchsia-500/20 flex items-center justify-center">
            <Globe2 className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h1 className="text-2xl font-black tracking-tight">Proxy Scraper</h1>
            <p className="text-sm text-muted-foreground mt-0.5">
              {stagingTotal} found &middot; {aliveCount} alive &middot; {geoEnriched} geo-enriched
            </p>
          </div>
        </div>
        <button
          onClick={() => setShowSources(!showSources)}
          className="h-9 px-4 rounded-xl border border-border/50 hover:bg-muted/20 text-xs font-bold transition-all flex items-center gap-2"
        >
          <Settings2 className="w-3.5 h-3.5" />
          Sources
        </button>
      </div>

      {showSources && (
        <div className="bg-card border border-border/50 rounded-2xl p-5 space-y-4">
          <h3 className="font-bold text-sm">Proxy Sources</h3>
          <div className="space-y-2 max-h-60 overflow-y-auto custom-scrollbar">
            {sources.length === 0 && (
              <p className="text-xs text-muted-foreground py-4 text-center">No sources configured</p>
            )}
            {sources.map(src => (
              <div key={src.url} className="flex items-center gap-3 p-3 rounded-xl bg-muted/10 border border-border/30">
                <button
                  onClick={() => handleToggleSource(src.url)}
                  className={`w-9 h-5 rounded-full transition-all relative shrink-0 ${src.enabled ? 'bg-indigo-500' : 'bg-muted/30'}`}
                >
                  <div className={`w-3.5 h-3.5 rounded-full bg-white absolute top-0.5 transition-all ${src.enabled ? 'left-[18px]' : 'left-[3px]'}`} />
                </button>
                <span className="text-xs font-mono flex-1 truncate">{src.url}</span>
                <button
                  onClick={() => handleRemoveSource(src.url)}
                  className="h-7 w-7 rounded-lg hover:bg-rose-500/10 text-rose-400 flex items-center justify-center transition-all shrink-0"
                >
                  <Trash2 className="w-3 h-3" />
                </button>
              </div>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={customUrl}
              onChange={e => setCustomUrl(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleAddSource()}
              placeholder="https://example.com/proxies.txt"
              className="flex-1 h-9 px-4 rounded-xl bg-muted/10 border border-border/50 text-xs outline-none"
            />
            <button
              onClick={handleAddSource}
              disabled={!customUrl.trim()}
              className="h-9 px-4 rounded-xl bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold transition-all flex items-center gap-2 disabled:opacity-50 shrink-0"
            >
              <Plus className="w-3.5 h-3.5" />
              Add
            </button>
          </div>
          <div className="flex justify-end pt-1">
            <button
              onClick={handleSaveSources}
              disabled={sourcesLoading}
              className="h-9 px-5 rounded-xl bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold transition-all disabled:opacity-50 flex items-center gap-2"
            >
              <Download className="w-3.5 h-3.5" />
              Save Sources
            </button>
          </div>
        </div>
      )}

      <div className="bg-card border border-border/50 rounded-2xl p-5 space-y-4">
        <div className="flex items-end gap-4">
          <div className="space-y-2">
            <label className="text-[11px] font-medium text-muted-foreground ml-1">Batch Count</label>
            <input
              type="number"
              value={batchCount}
              onChange={e => setBatchCount(parseInt(e.target.value) || 1)}
              disabled={running}
              className="w-28 h-10 bg-muted/10 border border-border/50 rounded-xl px-4 text-sm font-bold outline-none disabled:opacity-50"
            />
          </div>
          <button
            onClick={handleStart}
            disabled={running || starting}
            className="h-10 px-5 rounded-xl bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold transition-all flex items-center gap-2 disabled:opacity-50"
          >
            {starting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
            Start Scrape
          </button>
          {running && (
            <button
              onClick={handleStop}
              className="h-10 px-5 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-500 hover:bg-rose-500 hover:text-white text-xs font-bold transition-all flex items-center gap-2"
            >
              <StopCircle className="w-3.5 h-3.5" />
              Stop
            </button>
          )}
        </div>
        {running && (
          <div className="space-y-1.5">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span className="font-medium">
                {job ? `${job.found} found · ${job.alive} alive · ${job.imported} imported` : 'Starting...'}
              </span>
              <span className="text-[11px]">{job ? `${job.failed} failed` : ''}</span>
            </div>
            <div className="h-2 rounded-full bg-muted/30 overflow-hidden">
              <div className={`h-full rounded-full bg-indigo-500 ${job ? 'transition-all duration-300' : 'animate-pulse'}`} style={{ width: job ? `${Math.min(100, (job.found / Math.max(job.total, 1)) * 100)}%` : '30%' }} />
            </div>
          </div>
        )}
      </div>

      <div className="bg-card border border-border/50 rounded-2xl overflow-hidden">
        <div className="px-5 py-4 border-b border-border/30 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-2.5 h-2.5 rounded-full bg-indigo-500" />
            <h3 className="text-sm font-bold">Live Logs</h3>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={() => setAutoScroll(!autoScroll)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full transition-all border text-[10px] font-medium ${autoScroll
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-500'
                : 'bg-muted/30 border-border/40 text-muted-foreground'
              }`}
            >
              <div className={`w-1.5 h-1.5 rounded-full ${autoScroll ? 'bg-emerald-500 animate-pulse' : 'bg-muted-foreground/40'}`} />
              {autoScroll ? 'Auto-scroll: on' : 'Auto-scroll: locked'}
            </button>
            <button onClick={() => setLogs([])} className="text-[11px] font-medium text-muted-foreground hover:text-rose-500 transition-colors">
              Reset
            </button>
          </div>
        </div>
        <div
          ref={logContainerRef}
          className="p-5 overflow-y-auto font-mono text-[13px] leading-[1.9] space-y-0.5 custom-scrollbar h-[320px]"
        >
          {logs.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-muted-foreground/30 space-y-4">
              <Bot className="w-12 h-12 opacity-10" />
              <p className="font-black text-sm opacity-60">Waiting for scrape to start...</p>
            </div>
          ) : (
            logs.map((log, i) => {
              const isSuccess = log.level === 'success' || log.message.includes('live') || log.message.includes('imported')
              const isError = log.level === 'error' || log.message.includes('[ERROR]') || log.message.includes('Failed') || log.message.includes('Error')
              const isWarning = log.level === 'warning' || log.message.includes('[WARNING]') || log.message.includes('Warning')

              let icon = <Globe className="w-3 h-3 mt-0.5 opacity-40" />
              let colorClass = 'text-foreground/70'

              if (isSuccess) {
                icon = <CheckCircle2 className="w-3.5 h-3.5 mt-0.5 text-emerald-500" />
                colorClass = 'text-emerald-400'
              } else if (isError) {
                icon = <XCircle className="w-3.5 h-3.5 mt-0.5 text-rose-500" />
                colorClass = 'text-rose-400'
              } else if (isWarning) {
                icon = <AlertCircle className="w-3.5 h-3.5 mt-0.5 text-amber-500" />
                colorClass = 'text-amber-400'
              }

              return (
                <div key={i} className={`flex gap-3 transition-all hover:bg-white/5 p-1 rounded-lg ${colorClass}`}>
                  <span className="opacity-20 shrink-0 font-black tabular-nums w-8 text-right">{i + 1}</span>
                  <span className="shrink-0 mt-0.5">{icon}</span>
                  <span className="break-all font-medium tracking-tight">
                    {log.time && <span className="opacity-40">[{new Date(log.time).toLocaleTimeString()}] </span>}
                    {log.message}
                  </span>
                </div>
              )
            })
          )}
        </div>
      </div>

      <div className="space-y-4">
          <div className="flex items-center gap-3 flex-wrap">
            <div className="flex items-center gap-2">
              <Filter className="w-3.5 h-3.5 text-muted-foreground" />
              <span className="text-[11px] font-bold text-muted-foreground">Filters</span>
            </div>
            <select
              value={filterType}
              onChange={e => setFilterType(e.target.value)}
              className="h-8 px-3 rounded-lg bg-muted/10 border border-border/50 text-xs outline-none"
            >
              <option value="">All Types</option>
              <option value="http">HTTP</option>
              <option value="socks5">SOCKS5</option>
              <option value="socks4">SOCKS4</option>
            </select>
            <select
              value={filterCountry}
              onChange={e => setFilterCountry(e.target.value)}
              className="h-8 px-3 rounded-lg bg-muted/10 border border-border/50 text-xs outline-none"
            >
              <option value="">All Countries</option>
              {countries.map(c => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
            <input
              type="number"
              value={filterMaxLatency}
              onChange={e => setFilterMaxLatency(e.target.value)}
              placeholder="Max Latency (ms)"
              className="h-8 w-36 px-3 rounded-lg bg-muted/10 border border-border/50 text-xs outline-none"
            />
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            {selectedIds.size > 0 && (
              <>
                <span className="text-xs text-muted-foreground">{selectedIds.size} selected</span>
                <button
                  onClick={handleTransferSelected}
                  disabled={transferring}
                  className="h-8 px-3 rounded-lg bg-indigo-500/10 text-indigo-400 text-xs font-bold hover:bg-indigo-500/20 transition-all flex items-center gap-1.5 disabled:opacity-50"
                >
                  <ArrowRight className="w-3 h-3" />
                  Transfer Selected to Pool
                </button>
              </>
            )}
            {stagingProxies.length > 0 && selectedIds.size === 0 && (
              <button
                onClick={handleTransferAll}
                disabled={transferring}
                className="h-8 px-3 rounded-lg bg-emerald-500/10 text-emerald-400 text-xs font-bold hover:bg-emerald-500/20 transition-all flex items-center gap-1.5 disabled:opacity-50"
              >
                <Upload className="w-3 h-3" />
                Transfer All Matching
              </button>
            )}
          </div>

          <div className="bg-card border border-border/50 rounded-2xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border/30">
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider w-10">
                      <input type="checkbox" checked={stagingProxies.length > 0 && selectedIds.size === stagingProxies.length} onChange={toggleSelectAll} className="w-3.5 h-3.5" />
                    </th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Type</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Host</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Port</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Status</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Latency</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Country</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">ISP</th>
                    <th className="text-left p-3 text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Source</th>
                  </tr>
                </thead>
                <tbody>
                  {stagingLoading && stagingProxies.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="p-12 text-center">
                        <RefreshCw className="w-6 h-6 mx-auto text-muted-foreground/40 animate-spin mb-2" />
                        <p className="text-xs text-muted-foreground font-medium">Loading...</p>
                      </td>
                    </tr>
                  ) : stagingProxies.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="p-12 text-center">
                        <Globe className="w-8 h-8 mx-auto text-muted-foreground/40 mb-3" />
                        <p className="text-sm text-muted-foreground font-medium">No staging proxies</p>
                        <p className="text-xs text-muted-foreground/60 mt-1">Run a scrape to find proxies</p>
                      </td>
                    </tr>
                  ) : (
                    stagingProxies.map(proxy => (
                      <tr key={proxy.id} className="border-b border-border/10 hover:bg-muted/5 transition-colors">
                        <td className="p-3">
                          <input type="checkbox" checked={selectedIds.has(proxy.id)} onChange={() => toggleSelect(proxy.id)} className="w-3.5 h-3.5" />
                        </td>
                        <td className="p-3">
                          <span className="text-xs font-mono font-semibold uppercase">{proxy.type}</span>
                        </td>
                        <td className="p-3">
                          <span className="text-xs font-mono">{proxy.host}</span>
                        </td>
                        <td className="p-3">
                          <span className="text-xs font-mono">{proxy.port}</span>
                        </td>
                        <td className="p-3"><StatusBadge status={proxy.status} /></td>
                        <td className="p-3">
                          <span className={`text-xs font-mono font-semibold ${proxy.latency_ms > 0 && proxy.latency_ms < 500 ? 'text-emerald-400' : proxy.latency_ms >= 500 ? 'text-amber-400' : 'text-muted-foreground'}`}>
                            {proxy.latency_ms > 0 ? `${proxy.latency_ms}ms` : '--'}
                          </span>
                        </td>
                        <td className="p-3">
                          <span className="text-xs font-semibold">{proxy.country || '--'}</span>
                        </td>
                        <td className="p-3">
                          <span className="text-xs text-muted-foreground">{proxy.isp || '--'}</span>
                        </td>
                        <td className="p-3">
                          <span className="text-[10px] text-muted-foreground font-mono truncate block max-w-[120px]" title={proxy.source}>
                            {proxy.source || '--'}
                          </span>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
    </div>
  )
}
