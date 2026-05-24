'use client'

import { useState } from 'react'
import { X, Zap } from 'lucide-react'

type ProviderStats = {
  name: string
  type: string
  total: number
  live: number
  error: number
  banned: number
}

const MAIL_PROVIDERS = [
  { id: 'local', name: 'Local Mail (snapsave.my.id)', desc: 'Fastest and most reliable' },
  { id: 'guerrilla', name: 'GuerrillaMail', desc: 'Battle-tested, recommended' },
  { id: 'default', name: 'Official Helper', desc: 'ChatGPT.org.uk' },
]

const BATCH_SIZES = [3, 10, 25, 50, 100]

export default function BatchModalQwen({
  provider,
  onClose,
  onStart
}: {
  provider: ProviderStats
  onClose: () => void
  onStart: (config: { count: number; threads: number; mailProvider: string }) => void
}) {
  const [count, setCount] = useState(3)
  const [customCount, setCustomCount] = useState('')
  const [threads, setThreads] = useState(3)
  const [mailProvider, setMailProvider] = useState('guerrilla')
  const [showCustom, setShowCustom] = useState(false)

  const handleSubmit = () => {
    const finalCount = showCustom ? parseInt(customCount) || 3 : count
    onStart({
      count: finalCount,
      threads,
      mailProvider
    })
  }

  return (
    <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4">
      <div className="bg-card border border-border/50 rounded-2xl w-full max-w-md shadow-2xl">
        <div className="flex items-center justify-between p-6 border-b border-border/30">
          <div>
            <h2 className="text-lg font-black">Batch Register Qwen Accounts</h2>
            <p className="text-xs text-muted-foreground mt-1">Create multiple accounts with temp mail</p>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-6">
          <div>
            <label className="text-sm font-bold mb-3 block">Batch Size</label>
            <div className="flex gap-2 flex-wrap">
              {BATCH_SIZES.map(size => (
                <button
                  key={size}
                  onClick={() => { setCount(size); setShowCustom(false) }}
                  className={`px-4 py-2 rounded-lg text-sm font-bold transition-all ${
                    !showCustom && count === size
                      ? 'bg-indigo-500 text-white shadow-lg shadow-indigo-500/20'
                      : 'bg-muted/30 hover:bg-muted/50 text-foreground'
                  }`}
                >
                  {size}
                </button>
              ))}
              <button
                onClick={() => setShowCustom(true)}
                className={`px-4 py-2 rounded-lg text-sm font-bold transition-all ${
                  showCustom
                    ? 'bg-indigo-500 text-white shadow-lg shadow-indigo-500/20'
                    : 'bg-muted/30 hover:bg-muted/50 text-foreground'
                }`}
              >
                Custom
              </button>
            </div>
            {showCustom && (
              <input
                type="number"
                value={customCount}
                onChange={(e) => setCustomCount(e.target.value)}
                placeholder="Enter amount..."
                className="w-full mt-3 h-10 px-4 rounded-lg bg-muted/30 border border-border/50 text-sm"
                min="1"
                max="1000"
              />
            )}
          </div>

          <div>
            <label className="text-sm font-bold mb-3 block">Mail Provider</label>
            <div className="space-y-2">
              {MAIL_PROVIDERS.map(mp => (
                <button
                  key={mp.id}
                  onClick={() => setMailProvider(mp.id)}
                  className={`w-full text-left p-3 rounded-xl border transition-all ${
                    mailProvider === mp.id
                      ? 'border-indigo-500 bg-indigo-500/10'
                      : 'border-border/30 hover:border-border/60'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <span className="font-bold text-sm">{mp.name}</span>
                    <div className={`w-4 h-4 rounded-full border-2 ${
                      mailProvider === mp.id ? 'border-indigo-500 bg-indigo-500' : 'border-border'
                    }`} />
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">{mp.desc}</p>
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="text-sm font-bold mb-3 block">Concurrent Threads</label>
            <select
              value={threads}
              onChange={(e) => setThreads(parseInt(e.target.value))}
              className="w-full h-10 px-4 rounded-lg bg-muted/30 border border-border/50 text-sm"
            >
              {[1, 2, 3, 4, 5, 6, 7, 8].map(t => (
                <option key={t} value={t}>{t} thread{t > 1 ? 's' : ''}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="flex gap-3 p-6 border-t border-border/30">
          <button
            onClick={onClose}
            className="flex-1 h-11 rounded-xl border border-border/50 hover:bg-muted/20 font-bold text-sm transition-all"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            className="flex-1 h-11 rounded-xl bg-indigo-500 hover:bg-indigo-400 text-white font-bold text-sm flex items-center justify-center gap-2 transition-all"
          >
            <Zap className="w-4 h-4" />
            Start Registration
          </button>
        </div>
      </div>
    </div>
  )
}
