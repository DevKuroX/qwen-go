'use client'

import { useState, useEffect } from "react"
import { Plus, RefreshCw, Copy, Check, Trash2, Key } from "lucide-react"
import { toast } from "sonner"

function ShieldCheck(props: any) {
  return (
    <svg
      {...props}
      xmlns="http://www.w3.org/2000/svg"
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  )
}

export default function TokensPage() {
  const [keys, setKeys] = useState<string[]>([])
  const [copied, setCopied] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const fetchKeys = () => {
    setLoading(true)
    fetch("/api/admin/keys")
      .then(res => {
        if (!res.ok) throw new Error("Unauthorized")
        return res.json()
      })
      .then(data => setKeys(data.keys || []))
      .catch(() => toast.error("Refresh failed; please verify the admin key"))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchKeys()
  }, [])

  const handleGenerate = () => {
    fetch("/api/admin/keys", {
      method: "POST"
    }).then(res => {
      if (res.ok) {
        toast.success("New distribution key generated")
        fetchKeys()
      } else {
        toast.error("Generation failed: permission denied")
      }
    })
  }

  const handleDelete = (key: string) => {
    fetch(`/api/admin/keys/${encodeURIComponent(key)}`, {
      method: "DELETE"
    }).then(res => {
      if (res.ok) {
        toast.success("Key revoked")
        fetchKeys()
      } else {
        toast.error("Revocation failed")
      }
    })
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    setCopied(text)
    toast.success("Key copied")
    setTimeout(() => setCopied(null), 2000)
  }

  return (
    <div className="space-y-10 animate-fade-in-up max-w-[1400px] mx-auto">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-6">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-indigo-500/10 flex items-center justify-center border border-indigo-500/20">
            <Key className="w-5 h-5 text-indigo-500" />
          </div>
          <h2 className="text-3xl font-black tracking-tighter text-foreground">API Key Management</h2>
        </div>
        <div className="flex gap-3">
          <button
            onClick={() => { fetchKeys(); toast.success("Data synced"); }}
            className="h-12 px-5 rounded-2xl bg-muted/20 border border-border/40 hover:bg-muted/40 transition-all flex items-center gap-2 font-semibold text-sm"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> Refresh
          </button>
          <button
            onClick={handleGenerate}
            className="h-12 px-6 rounded-2xl bg-foreground text-background font-semibold flex items-center gap-2 hover:scale-[1.02] active:scale-95 transition-all text-sm shadow-xl shadow-black/5"
          >
            <Plus className="h-4 w-4" /> Generate new key
          </button>
        </div>
      </div>

      <div className="glass-card rounded-[2.5rem] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="bg-muted/10 border-b border-border/40">
                <th className="px-8 py-5 text-[11px] font-medium text-muted-foreground w-20">#</th>
                <th className="px-8 py-5 text-[11px] font-medium text-muted-foreground">Downstream API key</th>
                <th className="px-8 py-5 text-[11px] font-medium text-muted-foreground text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/20">
              {keys.length === 0 && !loading && (
                <tr>
                  <td colSpan={3} className="px-8 py-16 text-center text-muted-foreground font-medium">
                    No keys yet. Click the button above to generate one.
                  </td>
                </tr>
              )}
              {keys.map((k, i) => (
                <tr key={k} className="group hover:bg-muted/20 transition-colors">
                  <td className="px-8 py-6">
                    <span className="text-xs font-black text-muted-foreground/60 font-mono tracking-tighter">#{(i + 1).toString().padStart(2, '0')}</span>
                  </td>
                  <td className="px-8 py-6">
                    <div className="flex items-center gap-4">
                      <code className="text-[13px] font-mono font-bold text-foreground/80 bg-muted/30 px-3 py-1.5 rounded-lg border border-border/20 group-hover:bg-background transition-colors">
                        {k}
                      </code>
                    </div>
                  </td>
                  <td className="px-8 py-6 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button
                        onClick={() => copyToClipboard(k)}
                        className="p-3 rounded-xl bg-indigo-500/10 text-indigo-500 border border-indigo-500/20 hover:bg-indigo-500 hover:text-white transition-all shadow-lg shadow-indigo-500/5"
                        title="Copy key"
                      >
                        {copied === k ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                      </button>
                      <button
                        onClick={() => handleDelete(k)}
                        className="p-3 rounded-xl bg-muted/40 text-muted-foreground border border-border/40 hover:bg-rose-500/10 hover:text-rose-500 transition-all font-medium text-[10px]"
                        title="Revoke key"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="p-8 rounded-[2rem] bg-indigo-500/5 border border-indigo-500/10 space-y-3">
        <div className="flex items-center gap-2">
          <ShieldCheck className="w-4 h-4 text-indigo-500" />
          <span className="text-[10px] font-medium text-indigo-500">Security tip</span>
        </div>
        <p className="text-xs text-muted-foreground/80 font-medium leading-relaxed">
          API keys authenticate downstream clients (NextChat, LobeChat, the OpenAI SDK, etc.). Each key has independent quota and statistics. Keep them safe — leaked keys may be abused.
        </p>
      </div>
    </div>
  )
}
