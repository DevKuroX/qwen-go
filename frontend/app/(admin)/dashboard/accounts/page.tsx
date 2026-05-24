'use client'

import { useState, useEffect, useCallback } from 'react'
import { ChevronDown, ChevronRight, Trash2, Activity, AlertTriangle, XCircle, Clock, Search, ShieldCheck, Loader2, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'

type Account = {
  email: string
  provider: string
  status: string
  created_at: string
  username?: string
}

type ProviderStats = {
  name: string
  type: string
  total: number
  live: number
  error: number
  banned: number
}

const statusStyle = (status: string) => {
  const s = (status || '').toUpperCase()
  switch (s) {
    case 'VALID':
      return 'bg-emerald-500/10 text-emerald-500 ring-emerald-500/20'
    case 'RATE_LIMITED':
      return 'bg-orange-500/10 text-orange-500 ring-orange-500/20'
    case 'SOFT_ERROR':
    case 'CIRCUIT_OPEN':
      return 'bg-amber-500/10 text-amber-500 ring-amber-500/20'
    case 'BANNED':
      return 'bg-rose-500/10 text-rose-500 ring-rose-500/20'
    default:
      return 'bg-slate-500/10 text-slate-500 ring-slate-500/20'
  }
}

const statusIcon = (status: string) => {
  const s = (status || '').toUpperCase()
  switch (s) {
    case 'VALID':
      return <Activity className="w-3 h-3" />
    case 'RATE_LIMITED':
    case 'SOFT_ERROR':
    case 'CIRCUIT_OPEN':
      return <AlertTriangle className="w-3 h-3" />
    case 'BANNED':
      return <XCircle className="w-3 h-3" />
    default:
      return <Activity className="w-3 h-3" />
  }
}

const timeAgo = (dateStr: string) => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMins / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  return `${diffDays}d ago`
}

export default function AccountsPage() {
  const [providers, setProviders] = useState<ProviderStats[]>([])
  const [expandedProvider, setExpandedProvider] = useState<string | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [loading, setLoading] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  const [searchInput, setSearchInput] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<Account[]>([])
  const [searching, setSearching] = useState(false)

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkConfirmDelete, setBulkConfirmDelete] = useState(false)
  const [verifying, setVerifying] = useState<string | null>(null)
  const [refreshingCookies, setRefreshingCookies] = useState<string | null>(null)

  const inSearchMode = searchQuery.trim().length > 0
  const visibleAccounts = inSearchMode ? searchResults : accounts

  useEffect(() => {
    fetchProviders()
    const interval = setInterval(fetchProviders, 10000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    const t = setTimeout(() => setSearchQuery(searchInput), 300)
    return () => clearTimeout(t)
  }, [searchInput])

  useEffect(() => {
    if (!inSearchMode) {
      setSearchResults([])
      return
    }
    setSearching(true)
    const ac = new AbortController()
    fetch(`/api/admin/accounts/search?q=${encodeURIComponent(searchQuery)}`, { signal: ac.signal })
      .then(r => r.json())
      .then(data => setSearchResults(data.accounts || []))
      .catch(() => {})
      .finally(() => setSearching(false))
    return () => ac.abort()
  }, [searchQuery, inSearchMode])

  const fetchProviders = async () => {
    try {
      const res = await fetch('/api/admin/providers')
      const data = await res.json()
      setProviders(data.providers || [])
    } catch (err) {
      console.error('Failed to fetch providers:', err)
    }
  }

  const fetchAccounts = useCallback(async (providerName: string) => {
    setLoading(true)
    try {
      const res = await fetch(`/api/admin/providers/${providerName}/accounts`)
      const data = await res.json()
      setAccounts(data.accounts || [])
    } catch (err) {
      console.error('Failed to fetch accounts:', err)
    }
    setLoading(false)
  }, [])

  const refreshSearch = useCallback(async () => {
    if (!inSearchMode) return
    const res = await fetch(`/api/admin/accounts/search?q=${encodeURIComponent(searchQuery)}`)
    const data = await res.json()
    setSearchResults(data.accounts || [])
  }, [inSearchMode, searchQuery])

  const refreshCurrent = useCallback(async () => {
    if (inSearchMode) {
      await refreshSearch()
    } else if (expandedProvider) {
      await fetchAccounts(expandedProvider)
    }
    fetchProviders()
  }, [inSearchMode, refreshSearch, expandedProvider, fetchAccounts])

  const handleExpand = (providerName: string) => {
    if (expandedProvider === providerName) {
      setExpandedProvider(null)
      setAccounts([])
      setSelected(new Set())
    } else {
      setExpandedProvider(providerName)
      setSelected(new Set())
      fetchAccounts(providerName)
    }
  }

  const handleDelete = async (email: string) => {
    try {
      const res = await fetch(`/api/admin/accounts/${encodeURIComponent(email)}`, { method: 'DELETE' })
      const data = await res.json()
      if (data.ok) {
        toast.success('Account deleted')
        setSelected(prev => { const next = new Set(prev); next.delete(email); return next })
        await refreshCurrent()
      } else {
        toast.error(data.error || 'Failed to delete')
      }
    } catch {
      toast.error('Request failed')
    }
    setDeleteConfirm(null)
  }

  const handleVerify = async (email: string) => {
    setVerifying(email)
    try {
      const res = await fetch(`/api/admin/accounts/${encodeURIComponent(email)}/verify`, { method: 'POST' })
      const data = await res.json()
      if (data.valid) {
        toast.success(`${email}: token valid`)
      } else {
        toast.warning(`${email}: verification failed`)
      }
      await refreshCurrent()
    } catch {
      toast.error('Verify failed')
    } finally {
      setVerifying(null)
    }
  }

  const handleRefreshCookies = async (email: string) => {
    setRefreshingCookies(email)
    try {
      const res = await fetch(`/api/admin/accounts/${encodeURIComponent(email)}/refresh`, { method: 'POST' })
      const data = await res.json()
      if (data.ok) {
        toast.success(`${email}: cookies refreshed`)
      } else {
        toast.error(data.error || 'Refresh failed')
      }
      await refreshCurrent()
    } catch {
      toast.error('Refresh request failed')
    } finally {
      setRefreshingCookies(null)
    }
  }

  const toggleSelect = (email: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(email)) next.delete(email)
      else next.add(email)
      return next
    })
  }

  const toggleSelectAll = () => {
    if (selected.size === visibleAccounts.length) {
      setSelected(new Set())
    } else {
      setSelected(new Set(visibleAccounts.map(a => a.email)))
    }
  }

  const handleBulkVerify = async () => {
    if (selected.size === 0) return
    const emails = Array.from(selected)
    try {
      const res = await fetch('/api/admin/accounts/batch-verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emails }),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Verified ${data.verified}/${data.total} accounts`)
        await refreshCurrent()
      } else {
        toast.error(data.error || 'Batch verify failed')
      }
    } catch {
      toast.error('Batch verify request failed')
    }
  }

  const handleBulkDelete = async () => {
    if (selected.size === 0) return
    const emails = Array.from(selected)
    try {
      const res = await fetch('/api/admin/accounts/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emails }),
      })
      const data = await res.json()
      if (data.ok) {
        toast.success(`Deleted ${data.deleted}/${data.total} accounts`)
        setSelected(new Set())
        await refreshCurrent()
      } else {
        toast.error(data.error || 'Batch delete failed')
      }
    } catch {
      toast.error('Batch delete request failed')
    }
    setBulkConfirmDelete(false)
  }

  const renderAccountRow = (acc: Account) => (
    <div key={acc.email} className="p-4 flex items-center justify-between hover:bg-muted/5">
      <div className="flex items-center gap-4">
        <input
          type="checkbox"
          checked={selected.has(acc.email)}
          onChange={() => toggleSelect(acc.email)}
          className="w-4 h-4"
        />
        <div className="h-8 w-8 rounded-lg bg-muted/30 flex items-center justify-center">
          <span className="text-xs font-bold">{acc.email[0]?.toUpperCase()}</span>
        </div>
        <div>
          <p className="text-sm font-medium">{acc.email}</p>
          <div className="flex items-center gap-3 mt-1 text-xs text-muted-foreground">
            {inSearchMode && <span className="font-mono uppercase text-[10px]">{acc.provider}</span>}
            <span className="flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {timeAgo(acc.created_at)}
            </span>
          </div>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <span className={`px-3 py-1 rounded-full text-xs font-bold flex items-center gap-1.5 ring-1 ${statusStyle(acc.status)}`}>
          {statusIcon(acc.status)}
          {acc.status || 'VALID'}
        </span>
        <button
          onClick={() => handleVerify(acc.email)}
          disabled={verifying === acc.email}
          className="h-8 w-8 rounded-lg hover:bg-emerald-500/10 text-muted-foreground hover:text-emerald-500 flex items-center justify-center transition-all disabled:opacity-50"
          title="Verify token"
        >
          {verifying === acc.email ? <Loader2 className="w-4 h-4 animate-spin" /> : <ShieldCheck className="w-4 h-4" />}
        </button>
        {acc.provider === 'gemini-web' && (
          <button
            onClick={() => handleRefreshCookies(acc.email)}
            disabled={refreshingCookies === acc.email}
            className="h-8 w-8 rounded-lg hover:bg-blue-500/10 text-muted-foreground hover:text-blue-500 flex items-center justify-center transition-all disabled:opacity-50"
            title="Refresh cookies (rotate __Secure-1PSIDTS)"
          >
            {refreshingCookies === acc.email ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCw className="w-4 h-4" />}
          </button>
        )}
        {deleteConfirm === acc.email ? (
          <div className="flex gap-2">
            <button onClick={() => handleDelete(acc.email)} className="h-8 px-3 rounded-lg bg-rose-500 text-white text-xs font-bold">Confirm</button>
            <button onClick={() => setDeleteConfirm(null)} className="h-8 px-3 rounded-lg bg-muted/30 text-xs font-bold">Cancel</button>
          </div>
        ) : (
          <button
            onClick={() => setDeleteConfirm(acc.email)}
            className="h-8 w-8 rounded-lg hover:bg-rose-500/10 text-muted-foreground hover:text-rose-500 flex items-center justify-center transition-all"
            title="Delete"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        )}
      </div>
    </div>
  )

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-2xl font-black tracking-tight">Accounts by Provider</h1>
          <p className="text-sm text-muted-foreground mt-1">View and manage accounts grouped by provider</p>
        </div>
        <div className="relative w-full md:w-80">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground/60" />
          <input
            type="text"
            value={searchInput}
            onChange={e => setSearchInput(e.target.value)}
            placeholder="Search by email, status..."
            className="w-full h-10 pl-9 pr-3 rounded-xl bg-muted/20 border border-border/50 text-sm outline-none focus:border-indigo-500/50"
          />
        </div>
      </div>

      {selected.size > 0 && (
        <div className="flex items-center gap-3 px-4 py-3 rounded-xl bg-indigo-500/5 border border-indigo-500/20">
          <span className="text-xs font-bold text-indigo-400">{selected.size} selected</span>
          <button
            onClick={handleBulkVerify}
            className="h-8 px-3 rounded-lg bg-emerald-500/10 text-emerald-500 text-xs font-bold hover:bg-emerald-500/20 transition-all flex items-center gap-1.5"
          >
            <ShieldCheck className="w-3 h-3" />
            Verify selected
          </button>
          {bulkConfirmDelete ? (
            <div className="flex gap-2">
              <button onClick={handleBulkDelete} className="h-8 px-3 rounded-lg bg-rose-500 text-white text-xs font-bold">Confirm delete</button>
              <button onClick={() => setBulkConfirmDelete(false)} className="h-8 px-3 rounded-lg bg-muted/30 text-xs font-bold">Cancel</button>
            </div>
          ) : (
            <button
              onClick={() => setBulkConfirmDelete(true)}
              className="h-8 px-3 rounded-lg bg-rose-500/10 text-rose-400 text-xs font-bold hover:bg-rose-500/20 transition-all flex items-center gap-1.5"
            >
              <Trash2 className="w-3 h-3" />
              Delete selected
            </button>
          )}
          <button
            onClick={() => setSelected(new Set())}
            className="ml-auto text-xs text-muted-foreground hover:text-foreground"
          >
            Clear
          </button>
        </div>
      )}

      {inSearchMode ? (
        <div className="glass-card rounded-xl border border-border/30 overflow-hidden">
          <div className="px-5 py-3 border-b border-border/30 flex items-center justify-between">
            <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
              Search · &quot;{searchQuery}&quot; · {visibleAccounts.length} results
            </span>
            {visibleAccounts.length > 0 && (
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={selected.size > 0 && selected.size === visibleAccounts.length}
                  onChange={toggleSelectAll}
                  className="w-3.5 h-3.5"
                />
                Select all
              </label>
            )}
          </div>
          {searching ? (
            <div className="p-8 text-center text-muted-foreground">Searching...</div>
          ) : visibleAccounts.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">No matches</div>
          ) : (
            <div className="divide-y divide-border/20">
              {visibleAccounts.map(renderAccountRow)}
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {providers.map(provider => (
            <div key={provider.name} className="glass-card rounded-xl border border-border/30 overflow-hidden">
              <button
                onClick={() => handleExpand(provider.name)}
                className="w-full p-5 flex items-center justify-between hover:bg-muted/10 transition-colors"
              >
                <div className="flex items-center gap-4">
                  <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500 to-fuchsia-500 flex items-center justify-center">
                    <span className="text-white font-black text-sm">{provider.name[0].toUpperCase()}</span>
                  </div>
                  <div className="text-left">
                    <h3 className="font-black">{provider.name.charAt(0).toUpperCase() + provider.name.slice(1)}</h3>
                    <div className="flex items-center gap-4 mt-1 text-xs text-muted-foreground">
                      <span>{provider.total} total</span>
                      <span className="text-emerald-500">{provider.live} live</span>
                      <span className="text-amber-500">{provider.error} error</span>
                      <span className="text-rose-500">{provider.banned} banned</span>
                    </div>
                  </div>
                </div>
                {expandedProvider === provider.name ? (
                  <ChevronDown className="w-5 h-5 text-muted-foreground" />
                ) : (
                  <ChevronRight className="w-5 h-5 text-muted-foreground" />
                )}
              </button>

              {expandedProvider === provider.name && (
                <div className="border-t border-border/30">
                  {accounts.length > 0 && (
                    <div className="px-4 py-2 border-b border-border/20 flex items-center gap-2 bg-muted/5">
                      <input
                        type="checkbox"
                        checked={selected.size > 0 && selected.size === accounts.length}
                        onChange={toggleSelectAll}
                        className="w-3.5 h-3.5"
                      />
                      <span className="text-xs text-muted-foreground">Select all in {provider.name}</span>
                    </div>
                  )}
                  {loading ? (
                    <div className="p-8 text-center text-muted-foreground">Loading...</div>
                  ) : accounts.length === 0 ? (
                    <div className="p-8 text-center text-muted-foreground">No accounts found</div>
                  ) : (
                    <div className="divide-y divide-border/20">
                      {accounts.map(renderAccountRow)}
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
