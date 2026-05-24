'use client';

import { useEffect, useState, useCallback, useRef } from "react"
import { createPortal } from "react-dom"
import {
  BarChart2, Zap,
  Activity, TrendingUp, RefreshCw, Shield,
  ChevronLeft, ChevronRight, Calendar, Clock,
  CheckCircle2, AlertTriangle, XCircle, Server
} from "lucide-react"
import { toast } from "sonner"
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid,
  Tooltip, ResponsiveContainer, Legend
} from "recharts"

function Sparkline({ data, color = "#6366f1", height = 52 }: { data: number[]; color?: string; height?: number }) {
  if (!data || data.length < 2 || data.every(v => v === 0)) {
    return (
      <svg viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" style={{ width: "100%", height, opacity: 0.15 }}>
        <line x1="0" y1={height} x2="100" y2={height} stroke={color} strokeWidth="1.5" strokeDasharray="4 3" />
      </svg>
    )
  }
  const max = Math.max(...data, 1)
  const pts = data.map((v, i) => `${(i / (data.length - 1)) * 100},${height - (v / max) * height * 0.85}`).join(" ")
  return (
    <svg viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" style={{ width: "100%", height }}>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      <polygon points={`0,${height} ${pts} 100,${height}`} fill={`${color}22`} />
    </svg>
  )
}

function StatusTimeline({
  segments, nextRefreshAt, historyItems
}: {
  segments: string[];
  nextRefreshAt?: number;
  historyItems?: { ts: number; valid_pct: number; seg: string; valid?: number; total?: number }[]
}) {
  const COLOR: Record<string, string> = { green: "#10b981", amber: "#f59e0b", red: "#f43f5e", empty: "rgba(180,180,200,0.15)" }
  const [msLeft, setMsLeft] = useState(() => nextRefreshAt ? Math.max(0, nextRefreshAt - Date.now()) : 0)
  const [tip, setTip] = useState<{ x: number; y: number; item: { ts: number; valid_pct: number; seg: string; valid?: number; total?: number } } | null>(null)

  useEffect(() => {
    if (!nextRefreshAt) return
    setMsLeft(Math.max(0, nextRefreshAt - Date.now()))
    const t = setInterval(() => setMsLeft(Math.max(0, nextRefreshAt - Date.now())), 1000)
    return () => clearInterval(t)
  }, [nextRefreshAt])

  const remaining = Math.ceil(msLeft / 1000)
  const label = remaining > 0
    ? (remaining >= 60 ? `${Math.floor(remaining / 60)}M ${remaining % 60}S` : `${remaining}S`)
    : null

  const fmtTs = (ts: number) => {
    const d = new Date(ts * 1000)
    const p = (n: number) => String(n).padStart(2, "0")
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
  }
  const segLabel = (s: string) => s === "green" ? "Healthy" : s === "amber" ? "Degraded" : "Down"

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6, alignItems: "center" }}>
        <span style={{ fontSize: 9, fontWeight: 700, letterSpacing: "0.12em", textTransform: "uppercase", opacity: 0.4 }}>HISTORY ({segments.length}PTS)</span>
        {label && (
          <span style={{ fontSize: 9, fontWeight: 700, color: "#6366f1", display: "flex", alignItems: "center", gap: 4 }}>
            <Clock style={{ width: 9, height: 9 }} /> NEXT UPDATE IN {label}
          </span>
        )}
      </div>
      <div style={{ position: "relative", height: 32, borderRadius: 4, overflow: "visible", background: "rgba(180,180,200,0.08)", padding: 2 }}>
        <div style={{ display: "flex", flexDirection: "row-reverse", height: "100%", gap: 2, padding: "0 1px" }}>
          {segments.map((c, i) => {
            const item = historyItems && i < historyItems.length ? historyItems[i] : null
            return (
              <div
                key={i}
                style={{
                  flex: 1,
                  height: c === "empty" ? "30%" : c === "green" ? "100%" : c === "amber" ? "65%" : "35%",
                  alignSelf: "flex-end",
                  borderRadius: 2,
                  background: COLOR[c] ?? COLOR.empty,
                  opacity: item && tip && tip.item === item ? 1 : 0.85,
                  cursor: item ? "crosshair" : "default",
                  transition: "opacity 0.12s",
                }}
                onMouseEnter={item ? (e) => {
                  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
                  setTip({ x: r.left + r.width / 2, y: r.top, item })
                } : undefined}
                onMouseLeave={item ? () => setTip(null) : undefined}
              />
            )
          })}
        </div>
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", marginTop: 4 }}>
        <span style={{ fontSize: 8, opacity: 0.3, letterSpacing: "0.1em" }}>PAST</span>
        <span style={{ fontSize: 8, opacity: 0.3, letterSpacing: "0.1em" }}>NOW</span>
      </div>

      {tip && createPortal(
        <div style={{
          position: "fixed",
          left: tip.x,
          top: tip.y - 10,
          transform: "translate(-50%, -100%)",
          zIndex: 99999,
          padding: "10px 14px",
          borderRadius: 12,
          background: "hsl(var(--popover))",
          color: "hsl(var(--popover-foreground))",
          border: "1px solid rgba(140,140,180,0.2)",
          boxShadow: "0 8px 32px rgba(0,0,0,0.35)",
          minWidth: 190,
          pointerEvents: "none",
          fontSize: 11,
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 7, marginBottom: 8 }}>
            <div style={{ width: 8, height: 8, borderRadius: "50%", background: COLOR[tip.item.seg], flexShrink: 0 }} />
            <span style={{ fontWeight: 800, color: COLOR[tip.item.seg] }}>{segLabel(tip.item.seg)}</span>
            <span style={{ marginLeft: "auto", opacity: 0.45, fontSize: 9, fontFamily: "monospace" }}>{fmtTs(tip.item.ts)}</span>
          </div>
          <div style={{ display: "flex", justifyContent: "space-between", padding: "6px 0", borderTop: "1px solid rgba(140,140,180,0.12)" }}>
            <span style={{ opacity: 0.55 }}>Availability</span>
            <span style={{ fontWeight: 700, fontFamily: "monospace" }}>{tip.item.valid_pct.toFixed(1)}&nbsp;%</span>
          </div>
          {tip.item.valid !== undefined && tip.item.total !== undefined && (
            <div style={{ display: "flex", justifyContent: "space-between", padding: "6px 0", borderTop: "1px solid rgba(140,140,180,0.12)" }}>
              <span style={{ opacity: 0.55 }}>Nodes online</span>
              <span style={{ fontWeight: 700, fontFamily: "monospace" }}>{tip.item.valid}&nbsp;/&nbsp;{tip.item.total}</span>
            </div>
          )}
        </div>,
        document.body
      )}
    </div>
  )
}

function DonutChart({ slices, size = 100, thickness = 20 }: { slices: { value: number; color: string; label: string }[]; size?: number; thickness?: number }) {
  const r = (size - thickness) / 2
  const cx = size / 2, cy = size / 2
  const circumference = 2 * Math.PI * r
  const total = slices.reduce((s, v) => s + v.value, 0) || 1
  let offset = 0
  const paths = slices.map(s => {
    const pct = s.value / total
    const el = <circle key={s.label} cx={cx} cy={cy} r={r} fill="none" stroke={s.color} strokeWidth={thickness} strokeDasharray={`${pct * circumference} ${(1 - pct) * circumference}`} strokeDashoffset={-offset * circumference} style={{ transition: "stroke-dasharray 0.5s" }} />
    offset += pct
    return el
  })
  return (
    <svg width={size} height={size} style={{ transform: "rotate(-90deg)" }}>
      <circle cx={cx} cy={cy} r={r} fill="none" stroke="rgba(180,180,200,0.12)" strokeWidth={thickness} />
      {paths}
    </svg>
  )
}

const MONTHS_LABEL = ["01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"]
const WEEK_DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

function DateTimePicker({ value, onChange, onClear, placeholder }: { value: Date | null; onChange: (d: Date) => void; onClear?: () => void; placeholder: string }) {
  const [open, setOpen] = useState(false)
  const [viewYear, setViewYear] = useState(() => value?.getFullYear() ?? new Date().getFullYear())
  const [viewMonth, setViewMonth] = useState(() => value?.getMonth() ?? new Date().getMonth())
  const [selDate, setSelDate] = useState<Date | null>(value)
  const [hour, setHour] = useState(value?.getHours() ?? 0)
  const [minute, setMinute] = useState(value?.getMinutes() ?? 0)
  const ref = useRef<HTMLDivElement>(null)
  const popupRef = useRef<HTMLDivElement>(null)
  const hourRef = useRef<HTMLDivElement>(null)
  const minRef = useRef<HTMLDivElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)
  const [popupPos, setPopupPos] = useState({ top: 0, right: 0 })

  useEffect(() => {
    const handler = (e: MouseEvent) => { const t = e.target as Node; if (!ref.current?.contains(t) && !popupRef.current?.contains(t)) setOpen(false) }
    document.addEventListener("mousedown", handler); return () => document.removeEventListener("mousedown", handler)
  }, [])

  const handleOpen = () => {
    if (btnRef.current) { const r = btnRef.current.getBoundingClientRect(); setPopupPos({ top: r.bottom + 6, right: window.innerWidth - r.right }) }
    setOpen(v => !v)
  }
  useEffect(() => { if (open) { setTimeout(() => { hourRef.current?.children[hour]?.scrollIntoView({ block: "center", behavior: "instant" }); minRef.current?.children[minute]?.scrollIntoView({ block: "center", behavior: "instant" }) }, 30) } }, [open, hour, minute])

  const daysInMonth = (y: number, m: number) => new Date(y, m + 1, 0).getDate()
  const firstDayOfMonth = (y: number, m: number) => { const d = new Date(y, m, 1).getDay(); return d === 0 ? 6 : d - 1 }
  const confirmDate = (d: Date | null, h: number, mi: number) => { if (!d) return; const out = new Date(d); out.setHours(h, mi, 0, 0); onChange(out) }
  const fmt = (d: Date | null) => !d ? placeholder : `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}  ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`

  const cells: (number | null)[] = []
  const fd = firstDayOfMonth(viewYear, viewMonth), dim = daysInMonth(viewYear, viewMonth)
  for (let i = 0; i < fd; i++) cells.push(null)
  for (let i = 1; i <= dim; i++) cells.push(i)
  while (cells.length % 7 !== 0) cells.push(null)

  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button ref={btnRef} onClick={handleOpen}
        style={{ display: "flex", alignItems: "center", gap: 7, padding: "5px 11px", borderRadius: 10, border: `1px solid ${open ? "rgba(99,102,241,0.5)" : "rgba(180,180,200,0.3)"}`, background: open ? "rgba(99,102,241,0.08)" : "rgba(120,120,140,0.06)", color: open ? "#6366f1" : undefined, fontSize: 11, fontFamily: "monospace", minWidth: 180, cursor: "pointer" }}>
        <Calendar style={{ width: 12, height: 12, flexShrink: 0, opacity: 0.5 }} />
        <span style={{ color: value ? undefined : "rgba(120,120,140,0.4)", fontSize: 10 }}>{fmt(value)}</span>
      </button>
      {open && createPortal(
        <div ref={popupRef} style={{ position: "fixed", top: popupPos.top, right: popupPos.right, zIndex: 99999, display: "flex", borderRadius: 16, border: "1px solid rgba(100,100,130,0.18)", boxShadow: "0 20px 60px rgba(0,0,0,0.22)", background: "#ffffff", backdropFilter: "none", isolation: "isolate", overflow: "hidden" }}>
          <div style={{ padding: 16, minWidth: 234 }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
              <button onClick={() => { viewMonth === 0 ? (setViewMonth(11), setViewYear(y => y - 1)) : setViewMonth(m => m - 1) }} style={{ padding: 4, border: "none", background: "transparent", cursor: "pointer", display: "flex", borderRadius: 8 }}><ChevronLeft style={{ width: 16, height: 16 }} /></button>
              <span style={{ fontSize: 14, fontWeight: 900 }}>{viewYear}-{MONTHS_LABEL[viewMonth]}</span>
              <button onClick={() => { viewMonth === 11 ? (setViewMonth(0), setViewYear(y => y + 1)) : setViewMonth(m => m + 1) }} style={{ padding: 4, border: "none", background: "transparent", cursor: "pointer", display: "flex", borderRadius: 8 }}><ChevronRight style={{ width: 16, height: 16 }} /></button>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(7,1fr)", marginBottom: 4 }}>
              {WEEK_DAYS.map(d => <div key={d} style={{ textAlign: "center", fontSize: 9, fontWeight: 900, opacity: 0.4, padding: "4px 0" }}>{d}</div>)}
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(7,1fr)", gap: "2px 0" }}>
              {cells.map((day, i) => {
                const isSel = selDate && selDate.getDate() === day && selDate.getMonth() === viewMonth && selDate.getFullYear() === viewYear
                const isToday = day !== null && new Date().getDate() === day && new Date().getMonth() === viewMonth && new Date().getFullYear() === viewYear
                return <button key={i} disabled={!day} onClick={() => { if (!day) return; const nd = new Date(viewYear, viewMonth, day); setSelDate(nd); confirmDate(nd, hour, minute) }}
                  style={{ width: 28, height: 28, margin: "0 auto", borderRadius: 10, border: isSel ? "none" : isToday ? "1px solid rgba(99,102,241,0.4)" : "none", background: isSel ? "#6366f1" : "transparent", color: isSel ? "#fff" : isToday ? "#6366f1" : undefined, fontSize: 12, fontWeight: 700, cursor: day ? "pointer" : "default", visibility: day ? "visible" : "hidden", display: "flex", alignItems: "center", justifyContent: "center" }}>{day}</button>
              })}
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", marginTop: 12, paddingTop: 12, borderTop: "1px solid rgba(180,180,200,0.2)" }}>
              <button onClick={() => { setSelDate(null); setOpen(false); onClear?.() }} style={{ fontSize: 11, fontWeight: 800, color: "#6366f1", background: "none", border: "none", cursor: "pointer" }}>Clear</button>
              <button onClick={() => { const t = new Date(); setSelDate(t); setViewYear(t.getFullYear()); setViewMonth(t.getMonth()); confirmDate(t, hour, minute) }} style={{ fontSize: 11, fontWeight: 800, color: "#6366f1", background: "none", border: "none", cursor: "pointer" }}>Today</button>
            </div>
          </div>
          <div style={{ width: 1, background: "rgba(180,180,200,0.2)", margin: "12px 0" }} />
          <div style={{ display: "flex" }}>
            <div ref={hourRef} style={{ overflowY: "auto", padding: "8px 4px", height: 240, width: 48, scrollbarWidth: "none" }}>
              {Array.from({ length: 24 }, (_, i) => <button key={i} onClick={() => { setHour(i); confirmDate(selDate, i, minute) }} style={{ width: "100%", padding: "4px 0", borderRadius: 8, border: "none", background: hour === i ? "#6366f1" : "transparent", color: hour === i ? "#fff" : undefined, fontSize: 12, fontWeight: 700, cursor: "pointer", textAlign: "center" }}>{String(i).padStart(2, "0")}</button>)}
            </div>
            <div ref={minRef} style={{ overflowY: "auto", padding: "8px 4px", height: 240, width: 48, scrollbarWidth: "none" }}>
              {Array.from({ length: 60 }, (_, i) => <button key={i} onClick={() => { setMinute(i); confirmDate(selDate, hour, i) }} style={{ width: "100%", padding: "4px 0", borderRadius: 8, border: "none", background: minute === i ? "#6366f1" : "transparent", color: minute === i ? "#fff" : undefined, fontSize: 12, fontWeight: 700, cursor: "pointer", textAlign: "center" }}>{String(i).padStart(2, "0")}</button>)}
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}

const PRESETS = [{ label: "1H", hours: 1 }, { label: "24H", hours: 24 }, { label: "7D", hours: 168 }, { label: "30D", hours: 720 }]
function formatNum(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M"
  if (n >= 1_000) return (n / 1_000).toFixed(1) + "K"
  return String(n)
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const p = (n: number) => String(n).padStart(2, "0")
  const now = new Date()
  if (d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate()) {
    return `${p(d.getHours())}:${p(d.getMinutes())}`
  }
  return `${p(d.getMonth() + 1)}/${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function GatewayHealth({ metrics }: { metrics: any }) {
  const { total, valid, soft_err, down, health_pct, total_rpm, avail_pct, overall_status, health_history, history_segments } = metrics
  const avgScore = health_pct
  const statusMap: Record<string, { label: string; color: string; icon: any; bg: string }> = {
    operational: { label: "Operational", color: "#10b981", icon: CheckCircle2, bg: "rgba(16,185,129,0.08)" },
    degraded: { label: "Partially Degraded", color: "#f59e0b", icon: AlertTriangle, bg: "rgba(245,158,11,0.08)" },
    critical: { label: "Critical Outage", color: "#f43f5e", icon: XCircle, bg: "rgba(244,63,94,0.08)" },
  }
  const statusCfg = statusMap[String(overall_status || "critical")]
  const StatusIcon = statusCfg.icon
  const segments: string[] = history_segments || []
  const histNewest: any[] = health_history ? [...health_history].reverse() : []

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "12px 16px", borderRadius: 14, background: statusCfg.bg, border: `1px solid ${statusCfg.color}25` }}>
        <StatusIcon style={{ width: 18, height: 18, color: statusCfg.color, flexShrink: 0 }} />
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 900, color: statusCfg.color }}>{statusCfg.label}</div>
          <div style={{ fontSize: 10, opacity: 0.55, marginTop: 1 }}>qwen3.6-plus · Web Gateway</div>
        </div>
        <div style={{ textAlign: "right" }}>
          <div style={{ fontSize: 22, fontWeight: 900, color: statusCfg.color, lineHeight: 1 }}>{avail_pct}<span style={{ fontSize: 12 }}>%</span></div>
          <div style={{ fontSize: 9, opacity: 0.55, marginTop: 1 }}>Availability</div>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(4,1fr)", gap: 10 }}>
        {[
          { v: `${valid}/${total}`, l: "Online nodes", c: "#10b981" },
          { v: String(soft_err || 0), l: "Soft errors", c: "#f59e0b" },
          { v: String(down || 0), l: "Tripped/banned", c: "#f43f5e" },
          { v: (total_rpm || 0).toFixed(1), l: "Total RPM", c: "#6366f1" },
        ].map(m => (
          <div key={m.l} style={{ textAlign: "center", padding: "10px 6px", borderRadius: 12, background: `${m.c}0a`, border: `1px solid ${m.c}20` }}>
            <div style={{ fontSize: 18, fontWeight: 900, color: m.c, lineHeight: 1 }}>{m.v}</div>
            <div style={{ fontSize: 9, fontWeight: 500, opacity: 0.5, marginTop: 4 }}>{m.l}</div>
          </div>
        ))}
      </div>

      <div>
        <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
          <span style={{ fontSize: 11, fontWeight: 700, opacity: 0.5 }}>Node distribution</span>
          <span style={{ fontSize: 11, fontWeight: 700, opacity: 0.4 }}>{total} node(s)</span>
        </div>
        {total > 0 && (
          <div style={{ height: 6, borderRadius: 6, overflow: "hidden", display: "flex", gap: 1 }}>
            <div style={{ flex: Math.max(valid, 0.01), background: "#10b981", transition: "flex 0.5s" }} />
            <div style={{ flex: Math.max(soft_err || 0, 0.01), background: "#f59e0b", transition: "flex 0.5s" }} />
            <div style={{ flex: Math.max(down || 0, 0.01), background: "#f43f5e", transition: "flex 0.5s" }} />
          </div>
        )}
        <div style={{ display: "flex", gap: 14, marginTop: 6 }}>
          {[["#10b981", "Healthy", valid], ["#f59e0b", "Degraded", soft_err || 0], ["#f43f5e", "Down", down || 0]].map(([c, l, v]) => (
            <div key={l as string} style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <div style={{ width: 7, height: 7, borderRadius: "50%", background: c as string }} />
              <span style={{ fontSize: 10, fontWeight: 700, opacity: 0.55 }}>{l} {v}</span>
            </div>
          ))}
        </div>
      </div>

      <div>
        <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
          <span style={{ fontSize: 11, fontWeight: 700, opacity: 0.5 }}>Avg health score</span>
          <span style={{ fontSize: 13, fontWeight: 900, color: avgScore >= 80 ? "#10b981" : avgScore >= 50 ? "#f59e0b" : "#f43f5e" }}>{avgScore.toFixed(1)}<span style={{ fontSize: 10, opacity: 0.5 }}> / 100</span></span>
        </div>
        <div style={{ height: 5, borderRadius: 5, background: "rgba(180,180,200,0.15)", overflow: "hidden" }}>
          <div style={{ height: "100%", width: `${Math.min(avgScore, 100)}%`, background: avgScore >= 80 ? "#10b981" : avgScore >= 50 ? "#f59e0b" : "#f43f5e", borderRadius: 5, transition: "width 0.7s" }} />
        </div>
      </div>

      <StatusTimeline segments={segments} nextRefreshAt={Date.now() + 30000} historyItems={histNewest} />
    </div>
  )
}

export default function Dashboard() {
  const [stats, setStats] = useState<any>(null)
  const [healthMetrics, setHealthMetrics] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [preset, setPreset] = useState(24)
  const [startDate, setStartDate] = useState<Date | null>(null)
  const [endDate, setEndDate] = useState<Date | null>(null)
  const [useCustom, setUseCustom] = useState(false)
  const [modelFilter, setModelFilter] = useState("")

  const fetchStats = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (useCustom && startDate) params.set("start", String(Math.floor(startDate.getTime() / 1000)))
      else params.set("start", String(Math.floor(Date.now() / 1000 - preset * 3600)))
      if (useCustom && endDate) params.set("end", String(Math.floor(endDate.getTime() / 1000)))
      if (modelFilter) params.set("model", modelFilter)
      const [sRes, hRes] = await Promise.all([
        fetch(`/api/admin/stats/usage?${params}`),
        fetch(`/api/admin/health-metrics`),
      ])
      if (sRes.ok) setStats(await sRes.json())
      if (hRes.ok) setHealthMetrics(await hRes.json())
    } catch (e: any) { toast.error(e.message) }
    finally { setLoading(false) }
  }, [preset, useCustom, startDate, endDate, modelFilter])

  useEffect(() => {
    fetchStats()
    if (!useCustom) { const t = setInterval(fetchStats, 30000); return () => clearInterval(t) }
  }, [fetchStats, useCustom])

  const tlReqs = (stats?.timeline || []).map((p: any) => p.requests)
  const tlToks = (stats?.timeline || []).map((p: any) => p.tokens)
  const chatReqs = stats?.by_feature?.chat?.requests ?? 0
  const t2iReqs = stats?.by_feature?.t2i?.requests ?? 0
  const totalReqs = stats?.total_requests ?? 0
  const models = stats?.models || []

  const chartData = (stats?.timeline || []).map((p: any) => ({
    time: p.timestamp,
    label: formatTime(p.timestamp),
    requests: p.requests || 0,
    tokens: p.tokens || 0,
    prompt_tokens: p.prompt_tokens || 0,
    completion_tokens: p.completion_tokens || 0,
  }))

  const CARD_COLORS = ["#10b981", "#f59e0b", "#6366f1", "#f43f5e"]
  const cards = [
    { label: "Requests", icon: BarChart2, key: 0, main: formatNum(totalReqs), s1l: "Chat", s1v: formatNum(chatReqs), s2l: "Image", s2v: formatNum(t2iReqs), spark: tlReqs },
    { label: "Tokens used", icon: Zap, key: 1, main: formatNum(stats?.total_tokens ?? 0), s1l: "Input", s1v: formatNum(stats?.total_prompt_tokens ?? 0), s2l: "Output", s2v: formatNum(stats?.total_completion_tokens ?? 0), spark: tlToks },
    { label: "Performance", icon: TrendingUp, key: 2, main: `${(stats?.rpm ?? 0).toFixed(2)}`, s1l: "RPM", s1v: (stats?.rpm ?? 0).toFixed(2), s2l: "TPM", s2v: formatNum(stats?.tpm ?? 0), spark: tlReqs },
    { label: "Risk analysis", icon: Shield, key: 3, main: formatNum(totalReqs), s1l: "Success", s1v: formatNum(stats?.success_count ?? 0), s2l: "Failed", s2v: formatNum(stats?.error_count ?? 0), spark: tlReqs },
  ]

  const pieSlices = [
    { value: chatReqs, color: "#6366f1", label: "Chat" },
    { value: t2iReqs, color: "#a855f7", label: "Image" },
  ]

  return (
    <div className="animate-fade-in-up" style={{ maxWidth: 1400, margin: "0 auto", display: "flex", flexDirection: "column", gap: 24 }}>

      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ width: 42, height: 42, borderRadius: 14, background: "rgba(99,102,241,0.1)", border: "1px solid rgba(99,102,241,0.2)", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <Activity style={{ width: 20, height: 20, color: "#6366f1" }} />
          </div>
          <div>
            <h2 style={{ fontSize: 27, fontWeight: 900, letterSpacing: "-0.03em", margin: 0 }}>Usage Statistics</h2>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <button onClick={fetchStats} disabled={loading}
            style={{ display: "flex", alignItems: "center", gap: 7, padding: "7px 14px", borderRadius: 12, border: "1px solid rgba(180,180,200,0.3)", background: "rgba(120,120,140,0.06)", fontSize: 12, fontWeight: 600, cursor: "pointer", opacity: loading ? 0.5 : 1 }}>
            <RefreshCw style={{ width: 13, height: 13, animation: loading ? "spin 1s linear infinite" : undefined }} />Refresh
          </button>
        </div>
      </div>

      <div className="glass-card" style={{ padding: "10px 18px", borderRadius: 22, border: "1px solid rgba(180,180,200,0.18)", display: "flex", alignItems: "center", gap: 10 }}>
        <span style={{ fontSize: 10, fontWeight: 500, opacity: 0.45, flexShrink: 0 }}>Time range</span>
        <div style={{ display: "flex", gap: 5 }}>
          {PRESETS.map(p => (
            <button key={p.hours}
              onClick={() => { setPreset(p.hours); setUseCustom(false); setStartDate(null); setEndDate(null) }}
              style={{ padding: "5px 12px", borderRadius: 9, border: `1px solid ${!useCustom && preset === p.hours ? "#6366f1" : "rgba(180,180,200,0.22)"}`, background: !useCustom && preset === p.hours ? "#6366f1" : "rgba(120,120,140,0.06)", color: !useCustom && preset === p.hours ? "#fff" : undefined, fontSize: 11, fontWeight: 800, letterSpacing: "0.04em", cursor: "pointer" }}>
              {p.label === "1H" ? "1 hour" : p.label === "24H" ? "24 hours" : p.label === "7D" ? "7 days" : "30 days"}
            </button>
          ))}
        </div>
        <div style={{ flex: 1 }} />
        <DateTimePicker value={startDate} onChange={d => { setStartDate(d); setUseCustom(true) }} onClear={() => { setStartDate(null); if (!endDate) setUseCustom(false) }} placeholder="Start time" />
        <span style={{ fontSize: 12, opacity: 0.3, fontFamily: "monospace" }}>→</span>
        <DateTimePicker value={endDate} onChange={d => { setEndDate(d); setUseCustom(true) }} onClear={() => { setEndDate(null); if (!startDate) setUseCustom(false) }} placeholder="End time" />
        {useCustom && (
          <button onClick={fetchStats} style={{ padding: "6px 14px", borderRadius: 9, background: "#6366f1", color: "#fff", border: "none", fontSize: 11, fontWeight: 800, cursor: "pointer", flexShrink: 0 }}>Query</button>
        )}
      </div>

      {models.length > 0 && (
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ fontSize: 10, fontWeight: 500, opacity: 0.45, flexShrink: 0 }}>Model filter</span>
          <select
            value={modelFilter}
            onChange={e => setModelFilter(e.target.value)}
            style={{
              padding: "6px 12px", borderRadius: 9, border: "1px solid rgba(180,180,200,0.22)",
              background: "rgba(120,120,140,0.06)", fontSize: 11, fontWeight: 800,
              cursor: "pointer", outline: "none", minWidth: 180,
              color: "inherit"
            }}
          >
            <option value="">All models</option>
            {models.map((m: string) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
          {modelFilter && (
            <button onClick={() => setModelFilter("")}
              style={{ padding: "4px 10px", borderRadius: 8, border: "1px solid rgba(244,63,94,0.3)", background: "rgba(244,63,94,0.08)", color: "#f43f5e", fontSize: 10, fontWeight: 800, cursor: "pointer" }}>
              Clear filter
            </button>
          )}
        </div>
      )}

      <div style={{ display: "grid", gap: 18, gridTemplateColumns: "repeat(4, 1fr)" }}>
        {cards.map(card => (
          <div key={card.key} className="glass-card"
            style={{ borderRadius: 24, padding: "24px 24px 20px", border: "1px solid rgba(180,180,200,0.18)", position: "relative", overflow: "hidden", display: "flex", flexDirection: "column" }}>
            <div style={{ position: "absolute", top: -20, right: -20, width: 90, height: 90, background: `${CARD_COLORS[card.key]}07`, borderRadius: "50%", pointerEvents: "none" }} />
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 16 }}>
              <div style={{ padding: 10, borderRadius: 14, background: `${CARD_COLORS[card.key]}14`, border: `1px solid ${CARD_COLORS[card.key]}28` }}>
                <card.icon style={{ width: 20, height: 20, color: CARD_COLORS[card.key] }} />
              </div>
              <span style={{ fontSize: 12, fontWeight: 500, opacity: 0.5, paddingTop: 2 }}>{card.label}</span>
            </div>
            <div style={{ fontSize: 44, fontWeight: 900, letterSpacing: "-0.04em", lineHeight: 1, marginBottom: 14 }}>
              {loading ? <span style={{ opacity: 0.15 }}>—</span> : card.main}
            </div>
            <div style={{ display: "flex", gap: 18 }}>
              {[{ l: card.s1l, v: card.s1v }, { l: card.s2l, v: card.s2v }].map(sub => (
                <div key={sub.l}>
                  <div style={{ fontSize: 11, fontWeight: 500, opacity: 0.45 }}>{sub.l}</div>
                  <div style={{ fontSize: 15, fontWeight: 800, marginTop: 2 }}>{loading ? "—" : sub.v}</div>
                </div>
              ))}
            </div>
            <div style={{ marginTop: "auto", paddingTop: 14, opacity: 0.5 }}>
              <Sparkline data={card.spark} color={CARD_COLORS[card.key]} height={48} />
            </div>
          </div>
        ))}
      </div>

      {chartData.length > 0 && (
        <div className="glass-card" style={{ borderRadius: 24, overflow: "hidden", border: "1px solid rgba(180,180,200,0.18)" }}>
          <div style={{ padding: "16px 22px", borderBottom: "1px solid rgba(180,180,200,0.12)", display: "flex", alignItems: "center", gap: 10 }}>
            <BarChart2 style={{ width: 16, height: 16, opacity: 0.5 }} />
            <h3 style={{ fontSize: 13, fontWeight: 700, margin: 0 }}>
              {modelFilter ? `Usage trend — ${modelFilter}` : "Daily usage — all models"}
            </h3>
            <div style={{ flex: 1 }} />
            <span style={{ fontSize: 10, opacity: 0.4 }}>
              {stats?.total_tokens ? `${formatNum(stats.total_tokens)} total tokens · ${formatNum(totalReqs)} requests` : ""}
            </span>
          </div>
          <div style={{ padding: "12px 16px 8px" }}>
            <ResponsiveContainer width="100%" height={280}>
              <AreaChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="gradTokens" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="#6366f1" stopOpacity={0.35}/><stop offset="95%" stopColor="#6366f1" stopOpacity={0.02}/></linearGradient>
                  <linearGradient id="gradPrompt" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="#10b981" stopOpacity={0.35}/><stop offset="95%" stopColor="#10b981" stopOpacity={0.02}/></linearGradient>
                  <linearGradient id="gradCompletion" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="#f59e0b" stopOpacity={0.35}/><stop offset="95%" stopColor="#f59e0b" stopOpacity={0.02}/></linearGradient>
                  <linearGradient id="gradRequests" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="#f43f5e" stopOpacity={0.35}/><stop offset="95%" stopColor="#f43f5e" stopOpacity={0.02}/></linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="rgba(180,180,200,0.12)" />
                <XAxis dataKey="label" tick={{ fontSize: 10, opacity: 0.5 }} axisLine={false} tickLine={false} interval="preserveStartEnd" />
                <YAxis tick={{ fontSize: 10, opacity: 0.5 }} axisLine={false} tickLine={false} width={60} />
                <Tooltip
                  contentStyle={{
                    borderRadius: 12, border: "1px solid rgba(100,100,130,0.18)",
                    background: "hsl(var(--popover))", color: "hsl(var(--popover-foreground))",
                    fontSize: 11, boxShadow: "0 8px 32px rgba(0,0,0,0.35)",
                  }}
                  formatter={(value: any, name: any) => [formatNum(Number(value) || 0), name]}
                  labelFormatter={(label: any) => String(label || "")}
                />
                <Legend
                  wrapperStyle={{ fontSize: 10, paddingTop: 4 }}
                  formatter={(value: any) => {
                    const names: Record<string, string> = {
                      tokens: "Total tokens", prompt_tokens: "Prompt tokens",
                      completion_tokens: "Completion tokens", requests: "Requests"
                    }
                    return names[value] || value
                  }}
                />
                <Area type="monotone" dataKey="tokens" stroke="#6366f1" fill="url(#gradTokens)" strokeWidth={2} name="tokens" />
                <Area type="monotone" dataKey="prompt_tokens" stroke="#10b981" fill="url(#gradPrompt)" strokeWidth={2} name="prompt_tokens" />
                <Area type="monotone" dataKey="completion_tokens" stroke="#f59e0b" fill="url(#gradCompletion)" strokeWidth={2} name="completion_tokens" />
                <Area type="monotone" dataKey="requests" stroke="#f43f5e" fill="url(#gradRequests)" strokeWidth={2} name="requests" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      <div className="glass-card" style={{ borderRadius: 24, overflow: "hidden", border: "1px solid rgba(180,180,200,0.18)" }}>
        <div style={{ padding: "16px 22px", borderBottom: "1px solid rgba(180,180,200,0.12)", display: "flex", alignItems: "center", gap: 10 }}>
          <Server style={{ width: 16, height: 16, opacity: 0.5 }} />
          <h3 style={{ fontSize: 13, fontWeight: 700, margin: 0 }}>Monitoring overview</h3>
          <div style={{ flex: 1 }} />
          <span style={{ fontSize: 10, opacity: 0.4 }}>Request analytics · Gateway health · Real-time timeline</span>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "300px 1fr", gap: 0 }}>
          <div style={{ padding: "20px 22px", borderRight: "1px solid rgba(180,180,200,0.12)", display: "flex", flexDirection: "column", gap: 18 }}>
            <div style={{ fontSize: 11, fontWeight: 600, opacity: 0.45 }}>Request analytics</div>

            <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
              <div style={{ position: "relative", flexShrink: 0 }}>
                <DonutChart slices={pieSlices} size={96} thickness={19} />
                <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>
                  <div style={{ fontSize: 15, fontWeight: 900, lineHeight: 1 }}>{formatNum(totalReqs)}</div>
                  <div style={{ fontSize: 8, opacity: 0.4, marginTop: 2 }}>Total requests</div>
                </div>
              </div>
              <div style={{ flex: 1 }}>
                {[
                  { label: "Chat", color: "#6366f1", reqs: chatReqs, toks: stats?.by_feature?.chat?.tokens ?? 0 },
                  { label: "Image", color: "#a855f7", reqs: t2iReqs, toks: stats?.by_feature?.t2i?.tokens ?? 0 },
                ].map(row => {
                  const pct = totalReqs ? Math.round(row.reqs / totalReqs * 100) : 0
                  return (
                    <div key={row.label} style={{ marginBottom: 10 }}>
                      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 3 }}>
                        <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                          <div style={{ width: 6, height: 6, borderRadius: "50%", background: row.color }} />
                          <span style={{ fontSize: 11, fontWeight: 700 }}>{row.label}</span>
                        </div>
                        <span style={{ fontSize: 11, fontWeight: 900, color: row.color }}>{pct}%</span>
                      </div>
                      <div style={{ fontSize: 9, opacity: 0.4, marginBottom: 4 }}>{formatNum(row.reqs)} reqs · {formatNum(row.toks)} tok</div>
                      <div style={{ height: 3, borderRadius: 3, background: "rgba(180,180,200,0.12)", overflow: "hidden" }}>
                        <div style={{ height: "100%", width: `${pct}%`, background: row.color, borderRadius: 3, transition: "width 0.7s" }} />
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>

            <div style={{ padding: "12px 14px", borderRadius: 14, border: "1px solid rgba(180,180,200,0.15)", background: "rgba(99,102,241,0.04)" }}>
              <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 8 }}>
                <span style={{ fontSize: 10, fontWeight: 600, opacity: 0.5 }}>Request trend</span>
                <span style={{ fontSize: 10, fontWeight: 700, opacity: 0.45 }}>{(stats?.timeline?.length ?? 0)} bucket(s)</span>
              </div>
              <Sparkline data={tlReqs} color="#6366f1" height={48} />
            </div>

            <div style={{ padding: "12px 14px", borderRadius: 14, border: "1px solid rgba(180,180,200,0.15)", background: "rgba(245,158,11,0.04)" }}>
              <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 8 }}>
                <span style={{ fontSize: 10, fontWeight: 600, opacity: 0.5 }}>Token trend</span>
                <span style={{ fontSize: 10, fontWeight: 700, color: "#f59e0b", opacity: 0.8 }}>{formatNum(stats?.total_tokens ?? 0)}</span>
              </div>
              <Sparkline data={tlToks} color="#f59e0b" height={48} />
            </div>

          </div>

          <div style={{ padding: "20px 22px" }}>
            <div style={{ fontSize: 11, fontWeight: 600, opacity: 0.45, marginBottom: 16, display: "flex", justifyContent: "space-between" }}>
              <span style={{ fontWeight: 600, fontSize: 11, opacity: 0.45 }}>Gateway health</span>
              <span style={{ fontFamily: "monospace", fontSize: 10, opacity: 0.6 }}>{(healthMetrics?.total ?? 0)} nodes · qwen3.6-plus</span>
            </div>
            {!healthMetrics?.total
              ? <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: 100, fontSize: 12, opacity: 0.25 }}>No node data</div>
               : <GatewayHealth metrics={healthMetrics || {}} />
            }
          </div>
        </div>
      </div>

    </div>
  )
}
