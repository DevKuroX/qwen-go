'use client'

import { useState, useEffect, useRef, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Layers, Plus, Activity, AlertTriangle, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import ProviderCard from './components/ProviderCard'
import BatchModalQwen from './components/BatchModalQwen'
import ImportModalGeneric from './components/ImportModalGeneric'
import ProgressModal from './components/ProgressModal'

type ProviderStats = {
  name: string
  type: string
  total: number
  live: number
  error: number
  banned: number
}

export default function ProvidersPage() {
  return (
    <Suspense fallback={<div className="text-sm text-muted-foreground">Loading...</div>}>
      <ProvidersPageInner />
    </Suspense>
  )
}

function ProvidersPageInner() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [providers, setProviders] = useState<ProviderStats[]>([])
  const [selectedProvider, setSelectedProvider] = useState<ProviderStats | null>(null)
  const [showBatchModal, setShowBatchModal] = useState(false)
  const [showImportModal, setShowImportModal] = useState(false)
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const autoOpened = useRef(false)

  useEffect(() => {
    fetchProviders()
    const interval = setInterval(fetchProviders, 5000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    if (autoOpened.current) return
    const target = searchParams.get('bulkAdd')
    if (!target || providers.length === 0) return
    const match = providers.find(p => p.name === target)
    if (match) {
      autoOpened.current = true
      handleAddAccount(match)
      router.replace('/dashboard/providers')
    }
  }, [providers, searchParams, router])

  const fetchProviders = async () => {
    try {
      const res = await fetch('/api/admin/providers')
      const data = await res.json()
      setProviders(data.providers || [])
    } catch (err) {
      console.error('Failed to fetch providers:', err)
    }
  }

  const handleAddAccount = async (provider: ProviderStats) => {
    setSelectedProvider(provider)
    if (provider.name === 'qwen') {
      setShowBatchModal(true)
      return
    }
    if (provider.name === 'opencode-zen') {
      // Sessions are ephemeral UUIDs — backend mints one when no token is sent.
      try {
        const res = await fetch('/api/admin/accounts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ provider: 'opencode-zen' }),
        })
        const data = await res.json()
        if (res.ok && data.ok) {
          toast.success('Spawned new opencode-zen session')
          fetchProviders()
        } else {
          toast.error(data.error || 'Failed to spawn session')
        }
      } catch {
        toast.error('Network error')
      }
      return
    }
    setShowImportModal(true)
  }

  const handleBatchStart = async (config: { count: number; threads: number; mailProvider: string }) => {
    if (!selectedProvider) return

    try {
      const res = await fetch('/api/admin/batch/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: selectedProvider.name,
          count: config.count,
          threads: config.threads,
          mail_provider: config.mailProvider
        })
      })

      const data = await res.json()

      if (data.ok) {
        setActiveJobId(data.job_id)
        setShowBatchModal(false)
        toast.success('Batch registration started')
      } else {
        toast.error(data.error || 'Failed to start batch')
      }
    } catch (err) {
      toast.error('Request failed')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl md:text-2xl font-black tracking-tight">Providers</h1>
          <p className="text-xs md:text-sm text-muted-foreground mt-1">Manage account providers and batch registration</p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {providers.map(provider => (
          <ProviderCard
            key={provider.name}
            provider={provider}
            onAddAccount={() => handleAddAccount(provider)}
          />
        ))}
      </div>

      {showBatchModal && selectedProvider && (
        <BatchModalQwen
          provider={selectedProvider}
          onClose={() => setShowBatchModal(false)}
          onStart={handleBatchStart}
        />
      )}

      {showImportModal && selectedProvider && (
        <ImportModalGeneric
          provider={selectedProvider}
          onClose={() => setShowImportModal(false)}
          onImported={fetchProviders}
        />
      )}

      {activeJobId && (
        <ProgressModal
          jobId={activeJobId}
          onClose={() => {
            setActiveJobId(null)
            fetchProviders()
          }}
        />
      )}
    </div>
  )
}
