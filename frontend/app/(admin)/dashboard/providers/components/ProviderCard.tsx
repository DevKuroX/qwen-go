'use client'

import { Activity, AlertTriangle, XCircle, Plus, Layers } from 'lucide-react'

type ProviderStats = {
  name: string
  type: string
  total: number
  live: number
  error: number
  banned: number
}

const gradientFor = (name: string) => {
  switch (name) {
    case 'qwen':
      return 'from-indigo-500 to-fuchsia-500'
    case 'kiro':
      return 'from-emerald-500 to-cyan-500'
    case 'openai-zen':
      return 'from-sky-500 to-violet-500'
    case 'opencode-zen':
      return 'from-amber-500 to-rose-500'
    case 'gemini-web':
      return 'from-blue-500 to-emerald-500'
    default:
      return 'from-slate-500 to-zinc-500'
  }
}

const ringFor = (name: string) => {
  switch (name) {
    case 'qwen':
      return 'hover:shadow-indigo-500/10'
    case 'kiro':
      return 'hover:shadow-emerald-500/10'
    case 'openai-zen':
      return 'hover:shadow-sky-500/10'
    case 'opencode-zen':
      return 'hover:shadow-amber-500/10'
    case 'gemini-web':
      return 'hover:shadow-blue-500/10'
    default:
      return 'hover:shadow-slate-500/10'
  }
}

export default function ProviderCard({ provider, onAddAccount }: { provider: ProviderStats; onAddAccount: () => void }) {
  const nameUpper = provider.name.charAt(0).toUpperCase() + provider.name.slice(1)
  const grad = gradientFor(provider.name)
  const ring = ringFor(provider.name)
  const isActive = provider.live > 0
  const typeLabel = provider.type === 'temp_mail' ? 'Temp Mail' : provider.type === 'oauth' ? 'OAuth' : provider.type

  return (
    <div
      className={`group relative overflow-hidden rounded-2xl border border-border/40 bg-card/50 backdrop-blur p-4 md:p-5 transition-all hover:border-border/80 hover:shadow-lg ${ring}`}
    >
      <div className={`pointer-events-none absolute inset-x-0 -top-24 h-32 bg-gradient-to-br ${grad} opacity-[0.06] group-hover:opacity-[0.12] transition-opacity blur-2xl`} />

      <div className="relative flex items-start justify-between mb-4">
        <div className="flex items-center gap-2.5">
          <div className={`h-10 w-10 rounded-xl bg-gradient-to-br ${grad} flex items-center justify-center shadow-md`}>
            <span className="text-white font-black text-sm">{nameUpper[0]}</span>
          </div>
          <div>
            <h3 className="text-sm font-black tracking-tight">{nameUpper}</h3>
            <p className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">{typeLabel}</p>
          </div>
        </div>
        <span
          className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[9px] font-black uppercase tracking-wider ring-1 ${
            isActive
              ? 'bg-emerald-500/10 text-emerald-500 ring-emerald-500/20'
              : 'bg-slate-500/10 text-slate-500 ring-slate-500/20'
          }`}
        >
          <span className={`h-1.5 w-1.5 rounded-full ${isActive ? 'bg-emerald-500 animate-pulse' : 'bg-slate-500'}`} />
          {isActive ? 'Active' : 'Idle'}
        </span>
      </div>

      <div className="relative grid grid-cols-2 gap-1.5 mb-4">
        <div className="rounded-lg border border-border/30 bg-muted/10 p-2">
          <div className="flex items-center gap-1 text-[9px] uppercase tracking-wider text-muted-foreground font-bold">
            <Layers className="w-2.5 h-2.5" />
            Total
          </div>
          <p className="text-lg font-black tabular-nums mt-0.5">{provider.total}</p>
        </div>
        <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-2">
          <div className="flex items-center gap-1 text-[9px] uppercase tracking-wider text-emerald-500 font-bold">
            <Activity className="w-2.5 h-2.5" />
            Live
          </div>
          <p className="text-lg font-black tabular-nums text-emerald-500 mt-0.5">{provider.live}</p>
        </div>
        <div className="rounded-lg border border-amber-500/20 bg-amber-500/5 p-2">
          <div className="flex items-center gap-1 text-[9px] uppercase tracking-wider text-amber-500 font-bold">
            <AlertTriangle className="w-2.5 h-2.5" />
            Error
          </div>
          <p className="text-lg font-black tabular-nums text-amber-500 mt-0.5">{provider.error}</p>
        </div>
        <div className="rounded-lg border border-rose-500/20 bg-rose-500/5 p-2">
          <div className="flex items-center gap-1 text-[9px] uppercase tracking-wider text-rose-500 font-bold">
            <XCircle className="w-2.5 h-2.5" />
            Banned
          </div>
          <p className="text-lg font-black tabular-nums text-rose-500 mt-0.5">{provider.banned}</p>
        </div>
      </div>

      <button
        onClick={onAddAccount}
        className={`relative w-full h-9 rounded-lg bg-gradient-to-br ${grad} text-white font-bold text-xs flex items-center justify-center gap-1.5 transition-all hover:opacity-90 shadow-md hover:shadow-lg`}
      >
        <Plus className="w-3.5 h-3.5" />
        Add Account
      </button>
    </div>
  )
}
