'use client'

import { useState } from 'react'
import { X, Info } from 'lucide-react'
import { toast } from 'sonner'

type ProviderStats = {
  name: string
  type: string
  total: number
  live: number
  error: number
  banned: number
}

type FieldSpec = {
  hint: string
  emailLabel: string
  tokenLabel: string
  tokenHint: string
  refreshLabel?: string
  refreshHint?: string
  showRefresh: boolean
}

const SPECS: Record<string, FieldSpec> = {
  kiro: {
    hint: 'Paste an accessToken + refreshToken pair from the Kiro/AWS CodeWhisperer SSO flow.',
    emailLabel: 'Identifier',
    tokenLabel: 'Access Token',
    tokenHint: 'Bearer token used for chat calls.',
    refreshLabel: 'Refresh Token',
    refreshHint: 'Long-lived refresh credential.',
    showRefresh: true,
  },
  'gemini-web': {
    hint: 'Export __Secure-1PSID + __Secure-1PSIDTS cookies from a logged-in gemini.google.com session.',
    emailLabel: 'Identifier (Google email)',
    tokenLabel: '__Secure-1PSID',
    tokenHint: 'Primary cookie value.',
    refreshLabel: '__Secure-1PSIDTS',
    refreshHint: 'Refreshing cookie value.',
    showRefresh: true,
  },
}

export default function ImportModalGeneric({
  provider,
  onClose,
  onImported,
}: {
  provider: ProviderStats
  onClose: () => void
  onImported?: () => void
}) {
  const spec = SPECS[provider.name]
  const [email, setEmail] = useState('')
  const [token, setToken] = useState('')
  const [refreshToken, setRefreshToken] = useState('')
  const [busy, setBusy] = useState(false)

  if (!spec) return null

  const submit = async () => {
    if (!token.trim()) {
      toast.error('Token is required')
      return
    }
    if (spec.showRefresh && !refreshToken.trim()) {
      toast.error(`${spec.refreshLabel} is required`)
      return
    }
    setBusy(true)
    try {
      const res = await fetch('/api/admin/accounts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: provider.name,
          email: email.trim(),
          token: token.trim(),
          refresh_token: refreshToken.trim(),
        }),
      })
      const data = await res.json()
      if (res.ok && data.ok) {
        toast.success(`Account added to ${provider.name}`)
        onImported?.()
        onClose()
      } else {
        toast.error(data.error || 'Failed to add account')
      }
    } catch {
      toast.error('Network error')
    } finally {
      setBusy(false)
    }
  }

  const nameUpper = provider.name.charAt(0).toUpperCase() + provider.name.slice(1)

  return (
    <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-3">
      <div className="bg-card border border-border/50 rounded-xl w-full max-w-sm shadow-2xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-4 border-b border-border/30 sticky top-0 bg-card">
          <div>
            <h2 className="text-sm font-black">Add {nameUpper} Account</h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">Manual credential import</p>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground transition-colors">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-4 space-y-3">
          <div className="bg-indigo-500/10 border border-indigo-500/20 rounded-lg p-2.5 flex gap-2">
            <Info className="w-3.5 h-3.5 text-indigo-500 shrink-0 mt-0.5" />
            <p className="text-[11px] text-muted-foreground leading-snug">{spec.hint}</p>
          </div>

          <div>
            <label className="text-xs font-bold mb-1 block">{spec.emailLabel}</label>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="optional (auto-generated)"
              className="w-full h-9 px-3 rounded-lg bg-muted/30 border border-border/50 text-xs"
            />
          </div>

          <div>
            <label className="text-xs font-bold mb-1 block">{spec.tokenLabel}</label>
            <textarea
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="paste here..."
              className="w-full h-16 px-2.5 py-1.5 rounded-lg bg-muted/30 border border-border/50 text-[11px] font-mono resize-none"
            />
            <p className="text-[10px] text-muted-foreground mt-0.5">{spec.tokenHint}</p>
          </div>

          {spec.showRefresh && (
            <div>
              <label className="text-xs font-bold mb-1 block">{spec.refreshLabel}</label>
              <textarea
                value={refreshToken}
                onChange={(e) => setRefreshToken(e.target.value)}
                placeholder="paste here..."
                className="w-full h-14 px-2.5 py-1.5 rounded-lg bg-muted/30 border border-border/50 text-[11px] font-mono resize-none"
              />
              <p className="text-[10px] text-muted-foreground mt-0.5">{spec.refreshHint}</p>
            </div>
          )}
        </div>

        <div className="flex gap-2 p-4 border-t border-border/30 sticky bottom-0 bg-card">
          <button
            onClick={onClose}
            disabled={busy}
            className="flex-1 h-9 rounded-lg border border-border/50 hover:bg-muted/20 font-bold text-xs transition-all disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={submit}
            disabled={busy}
            className="flex-1 h-9 rounded-lg bg-indigo-500 hover:bg-indigo-400 text-white font-bold text-xs transition-all disabled:opacity-50"
          >
            {busy ? 'Adding...' : 'Add Account'}
          </button>
        </div>
      </div>
    </div>
  )
}
