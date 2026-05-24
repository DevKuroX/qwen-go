'use client'

import { useState, useEffect, useRef } from 'react'
import { X, StopCircle, CheckCircle2 } from 'lucide-react'

type LogEntry = {
  timestamp: string
  level: string
  message: string
}

type BatchJob = {
  id: string
  provider: string
  count: number
  progress: number
  status: string
  success: number
  failed: number
}

export default function ProgressModal({ jobId, onClose }: { jobId: string; onClose: () => void }) {
  const [job, setJob] = useState<BatchJob | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const logsRef = useRef<HTMLDivElement>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    const fetchStatus = async () => {
      try {
        const res = await fetch(`/api/admin/batch/${jobId}`)
        const data = await res.json()
        setJob(data)
      } catch (err) {
        console.error('Failed to fetch job status:', err)
      }
    }

    fetchStatus()
    const interval = setInterval(fetchStatus, 2000)

    const es = new EventSource(`/api/admin/batch/${jobId}/logs`)
    eventSourceRef.current = es

    es.onmessage = (e) => {
      try {
        const log = JSON.parse(e.data)
        setLogs(prev => [...prev, log])
      } catch (err) {
        console.error('Failed to parse log:', err)
      }
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      clearInterval(interval)
      es.close()
    }
  }, [jobId])

  useEffect(() => {
    if (logsRef.current) {
      logsRef.current.scrollTop = logsRef.current.scrollHeight
    }
  }, [logs])

  const handleStop = async () => {
    try {
      await fetch(`/api/admin/batch/${jobId}/stop`, { method: 'POST' })
    } catch (err) {
      console.error('Failed to stop job:', err)
    }
  }

  const progress = job ? (job.progress / job.count) * 100 : 0
  const isRunning = job?.status === 'running'
  const isCompleted = job?.status === 'completed' || job?.status === 'stopped'

  return (
    <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4">
      <div className="bg-card border border-border/50 rounded-2xl w-full max-w-2xl shadow-2xl">
        <div className="flex items-center justify-between p-6 border-b border-border/30">
          <div>
            <h2 className="text-lg font-black">Batch Registration Progress</h2>
            <p className="text-xs text-muted-foreground mt-1">
              {job?.provider?.toUpperCase()} • Job ID: {jobId}
            </p>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-4">
          <div>
            <div className="flex items-center justify-between text-sm mb-2">
              <span className="text-muted-foreground">Progress</span>
              <span className="font-bold">
                {job?.progress || 0} / {job?.count || 0} accounts
              </span>
            </div>
            <div className="h-3 bg-muted/30 rounded-full overflow-hidden">
              <div
                className="h-full bg-gradient-to-r from-indigo-500 to-fuchsia-500 transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>

          <div className="flex gap-4">
            <div className="flex items-center gap-2 text-sm">
              <CheckCircle2 className="w-4 h-4 text-emerald-500" />
              <span className="text-muted-foreground">Success:</span>
              <span className="font-bold text-emerald-500">{job?.success || 0}</span>
            </div>
            <div className="flex items-center gap-2 text-sm">
              <X className="w-4 h-4 text-rose-500" />
              <span className="text-muted-foreground">Failed:</span>
              <span className="font-bold text-rose-500">{job?.failed || 0}</span>
            </div>
          </div>

          <div>
            <label className="text-sm font-bold mb-2 block">Live Logs</label>
            <div
              ref={logsRef}
              className="h-64 bg-black/30 rounded-xl p-4 overflow-y-auto font-mono text-xs space-y-1"
            >
              {logs.length === 0 ? (
                <div className="text-muted-foreground">Waiting for logs...</div>
              ) : (
                logs.map((log, i) => (
                  <div
                    key={i}
                    className={`${
                      log.level === 'success' ? 'text-emerald-400' :
                      log.level === 'error' ? 'text-rose-400' :
                      log.level === 'warning' ? 'text-amber-400' :
                      'text-foreground/70'
                    }`}
                  >
                    <span className="text-muted-foreground">[{new Date(log.timestamp).toLocaleTimeString()}]</span> {log.message}
                  </div>
                ))
              )}
            </div>
          </div>
        </div>

        <div className="flex gap-3 p-6 border-t border-border/30">
          {isRunning && (
            <button
              onClick={handleStop}
              className="h-11 px-6 rounded-xl bg-rose-500 hover:bg-rose-400 text-white font-bold text-sm flex items-center gap-2 transition-all"
            >
              <StopCircle className="w-4 h-4" />
              Stop Registration
            </button>
          )}
          <button
            onClick={onClose}
            className="flex-1 h-11 rounded-xl border border-border/50 hover:bg-muted/20 font-bold text-sm transition-all"
          >
            {isCompleted ? 'Close' : 'Hide'}
          </button>
        </div>
      </div>
    </div>
  )
}
