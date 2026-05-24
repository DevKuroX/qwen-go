'use client'

import { useState, useEffect, useCallback, useRef, memo } from "react"
import {
  Film, Sparkles, Download, Trash2, RefreshCw,
  X, History, Clock, Settings, Check, AlertCircle,
  Square as SquareIcon, RectangleHorizontal, RectangleVertical,
  Play, Pause, ChevronDown
} from "lucide-react"
import { toast } from "sonner"

const PROVIDER_PREFIX: Record<string, string> = {
  qwen: "qw/",
  kiro: "kiro/",
  "opencode-zen": "zen/",
  "gemini-web": "gw/",
  openai: "openai/",
}

type AspectRatio = "1:1" | "4:3" | "3:4" | "16:9" | "9:16"
const ASPECT_RATIOS: { value: AspectRatio; w: number; h: number }[] = [
  { value: "16:9", w: 28, h: 16 },
  { value: "9:16", w: 16, h: 28 },
  { value: "1:1",  w: 20, h: 20 },
  { value: "4:3",  w: 24, h: 18 },
  { value: "3:4",  w: 18, h: 24 },
]

interface GeneratedVideo {
  id: string
  url: string
  prompt: string
  timestamp: number
  aspectRatio: AspectRatio
}

const HISTORY_API = "/api/admin/history/videos"

const VideoCard = memo(function VideoCard({ video, onClick, onDelete }: {
  video: GeneratedVideo
  onClick?: () => void
  onDelete?: () => void
}) {
  const ref = useRef<HTMLVideoElement>(null)
  const [playing, setPlaying] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const ratio = video.aspectRatio || "16:9"

  return (
    <div
      className="group relative rounded-2xl overflow-hidden bg-card border border-border/30 shadow-sm hover:shadow-xl hover:shadow-fuchsia-500/5 cursor-pointer transition-all duration-300 hover:-translate-y-0.5"
      style={{ aspectRatio: ratio.replace(":", " / ") }}
      onClick={onClick}
      onMouseEnter={() => { ref.current?.play().then(() => setPlaying(true)).catch(() => {}) }}
      onMouseLeave={() => { ref.current?.pause(); ref.current && (ref.current.currentTime = 0); setPlaying(false) }}
    >
      {!loaded && (
        <div className="absolute inset-0 bg-muted/20 animate-pulse flex items-center justify-center">
          <Film className="w-8 h-8 text-muted-foreground/20" />
        </div>
      )}
      <video
        ref={ref} src={video.url} muted loop playsInline preload="metadata"
        onLoadedData={() => setLoaded(true)}
        className={`w-full h-full object-cover transition-opacity ${loaded ? "opacity-100" : "opacity-0"}`}
      />
      <div className="absolute top-3 left-3 px-2 py-0.5 rounded-md bg-black/60 backdrop-blur-sm text-[9px] font-bold text-white/80 opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1">
        <Film className="w-2.5 h-2.5" /> {ratio}
      </div>
      {!playing && loaded && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/20 transition-opacity group-hover:opacity-0">
          <div className="w-12 h-12 rounded-full bg-white/15 backdrop-blur-md flex items-center justify-center">
            <Play className="w-5 h-5 text-white ml-0.5" fill="currentColor" />
          </div>
        </div>
      )}
      <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-all duration-300 p-4 flex flex-col justify-end pointer-events-none">
        <p className="text-white text-[11px] leading-snug line-clamp-2 font-medium mb-2">{video.prompt}</p>
        <div className="flex items-center justify-between pointer-events-auto">
          <span className="text-[9px] text-white/50 font-mono">{ratio}</span>
          <div className="flex gap-1.5">
            <a href={video.url} download onClick={e => e.stopPropagation()} className="p-1.5 rounded-lg bg-white/10 hover:bg-white/90 text-white hover:text-black transition-all">
              <Download className="w-3 h-3" />
            </a>
            {onDelete && (
              <button onClick={e => { e.stopPropagation(); onDelete() }} className="p-1.5 rounded-lg bg-white/10 hover:bg-rose-500 text-white transition-all">
                <Trash2 className="w-3 h-3" />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
})

function VideoLightbox({ video, onClose }: { video: GeneratedVideo; onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [playing, setPlaying] = useState(true)

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose()
      if (e.key === " ") { e.preventDefault(); togglePlay() }
    }
    window.addEventListener("keydown", h)
    return () => window.removeEventListener("keydown", h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const togglePlay = () => {
    if (!videoRef.current) return
    if (videoRef.current.paused) { videoRef.current.play(); setPlaying(true) }
    else { videoRef.current.pause(); setPlaying(false) }
  }

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 backdrop-blur-md" onClick={onClose}>
      <button onClick={onClose} className="absolute top-6 right-6 p-2 rounded-xl bg-white/10 hover:bg-white/20 text-white z-10">
        <X className="w-6 h-6" />
      </button>
      <div className="max-w-[90vw] max-h-[90vh] flex flex-col items-center gap-4" onClick={e => e.stopPropagation()}>
        <div className="relative">
          <video
            ref={videoRef}
            src={video.url}
            autoPlay
            loop
            controls
            className="max-w-full max-h-[78vh] rounded-2xl shadow-2xl"
            onPlay={() => setPlaying(true)}
            onPause={() => setPlaying(false)}
          />
          {!playing && (
            <button
              onClick={togglePlay}
              className="absolute inset-0 flex items-center justify-center bg-black/30 rounded-2xl transition-opacity hover:opacity-100"
            >
              <Play className="w-16 h-16 text-white" fill="currentColor" />
            </button>
          )}
        </div>
        <div className="max-w-2xl text-center space-y-2">
          <p className="text-white/90 text-sm leading-relaxed">{video.prompt}</p>
          <div className="flex items-center justify-center gap-3">
            <span className="text-[10px] font-black text-white/40">{video.aspectRatio}</span>
            <a href={video.url} download className="px-3 py-1 rounded-lg bg-white/10 hover:bg-white/20 text-white text-[11px] font-medium flex items-center gap-1.5">
              <Download className="w-3 h-3" /> Download
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}

const HistorySidebar = memo(function HistorySidebar({ items, onClear, onLightbox, onRemove }: {
  items: GeneratedVideo[]
  onClear: () => void
  onLightbox: (v: GeneratedVideo) => void
  onRemove: (id: string) => void
}) {
  if (items.length === 0) return (
    <div className="flex flex-col items-center justify-center h-full text-center gap-3 p-8">
      <Film className="w-12 h-12 text-muted-foreground/20" />
      <p className="text-xs font-black text-muted-foreground/30">No history yet</p>
    </div>
  )

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {items.map(v => (
          <div key={v.id}
            className="group relative rounded-xl overflow-hidden bg-muted/30 border border-border/40 cursor-pointer"
            style={{ aspectRatio: v.aspectRatio.replace(":", " / ") }}
            onClick={() => onLightbox(v)}>
            <video src={v.url} muted preload="metadata" className="w-full h-full object-cover" />
            <div className="absolute inset-0 bg-gradient-to-t from-black/80 to-transparent opacity-0 group-hover:opacity-100 transition-all p-2 flex flex-col justify-end">
              <p className="text-white text-[9px] leading-tight line-clamp-2 font-medium">{v.prompt}</p>
              <div className="flex items-center justify-between mt-1.5">
                <span className="text-white/60 text-[8px] font-mono">{v.aspectRatio}</span>
                <div className="flex gap-1">
                  <a href={v.url} download onClick={e => e.stopPropagation()} className="p-1 rounded bg-white/10 hover:bg-white text-white hover:text-black transition-all">
                    <Download className="w-2.5 h-2.5" />
                  </a>
                  <button onClick={e => { e.stopPropagation(); onRemove(v.id) }} className="p-1 rounded bg-white/10 hover:bg-rose-500 text-white transition-all">
                    <Trash2 className="w-2.5 h-2.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
      <div className="px-4 py-3 border-t border-border/30 shrink-0">
        <button onClick={onClear}
          className="w-full py-2 rounded-xl text-[10px] font-black text-rose-500/70 hover:text-rose-500 hover:bg-rose-500/10 transition-all border border-transparent hover:border-rose-500/20 flex items-center justify-center gap-1.5">
          <Trash2 className="w-3 h-3" /> Clear all history
        </button>
      </div>
    </div>
  )
})

export default function VideoPage() {
  const [prompt, setPrompt] = useState("")
  const [generating, setGenerating] = useState(false)
  const [batchSize, setBatchSize] = useState(1)
  const [aspectRatio, setAspectRatio] = useState<AspectRatio>("16:9")
  const [aspectOpen, setAspectOpen] = useState(false)
  const aspectRef = useRef<HTMLDivElement>(null)

  const [sessionVideos, setSessionVideos] = useState<GeneratedVideo[]>([])
  const [history, setHistory] = useState<GeneratedVideo[]>([])

  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [lightbox, setLightbox] = useState<GeneratedVideo | null>(null)

  const [galleryOpen, setGalleryOpen] = useState(false)
  const [tempMaxItems, setTempMaxItems] = useState<number>(50)
  const [maxItems, setMaxItems] = useState(50)
  const gearRef = useRef<HTMLDivElement>(null)

  const [model, setModel] = useState("")
  const [models, setModels] = useState<string[]>([])
  const [modelOpen, setModelOpen] = useState(false)
  const modelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    fetch("/api/admin/providers/configs")
      .then(r => r.json())
      .then(data => {
        const list: string[] = []
        for (const cfg of data.configs || []) {
          const prefix = PROVIDER_PREFIX[cfg.name] ?? `${cfg.name}/`
          const names: string[] = cfg.available_models?.length
            ? cfg.available_models
            : Object.keys(cfg.models || {})
          for (const n of names) list.push(prefix + n)
        }
        setModels(list)
        const saved = localStorage.getItem("qwenpi_video_model") || ""
        if (saved && list.includes(saved)) setModel(saved)
        else if (list.length > 0) setModel(list[0])
      })
      .catch(err => toast.error(`Load models: ${err.message}`))
  }, [])
  useEffect(() => { if (model) localStorage.setItem("qwenpi_video_model", model) }, [model])

  useEffect(() => {
    if (!modelOpen) return
    const h = (e: MouseEvent) => { if (modelRef.current && !modelRef.current.contains(e.target as Node)) setModelOpen(false) }
    document.addEventListener("mousedown", h); return () => document.removeEventListener("mousedown", h)
  }, [modelOpen])

  useEffect(() => {
    fetch(HISTORY_API).then(r => r.ok && r.json()).then(d => { if (d?.videos) setHistory(d.videos) }).catch(() => {})
    const savedRatio = localStorage.getItem("qwenpi_video_aspect") as AspectRatio
    if (savedRatio) setAspectRatio(savedRatio)
    const savedMax = parseInt(localStorage.getItem("video_gallery_max_items") || "50", 10)
    setTempMaxItems(savedMax)
    setMaxItems(savedMax)
  }, [])
  useEffect(() => { localStorage.setItem("qwenpi_video_aspect", aspectRatio) }, [aspectRatio])

  useEffect(() => {
    if (!aspectOpen) return
    const handler = (e: MouseEvent) => {
      if (aspectRef.current && !aspectRef.current.contains(e.target as Node)) setAspectOpen(false)
    }
    document.addEventListener("mousedown", handler)
    return () => document.removeEventListener("mousedown", handler)
  }, [aspectOpen])

  useEffect(() => {
    if (!galleryOpen) return
    const handler = (e: MouseEvent) => {
      if (gearRef.current && !gearRef.current.contains(e.target as Node)) setGalleryOpen(false)
    }
    document.addEventListener("mousedown", handler)
    return () => document.removeEventListener("mousedown", handler)
  }, [galleryOpen])

  const SIDEBAR_W = 380
  const ADMIN_SIDEBAR_W = 288
  const GRID_GAP = 16
  const GRID_PAD = 48
  const MIN_COL_W = 220
  const [cols, setCols] = useState(2)

  useEffect(() => {
    const calc = () => {
      const narrowW = window.innerWidth - ADMIN_SIDEBAR_W - SIDEBAR_W - GRID_PAD
      const c = Math.max(1, Math.floor((narrowW + GRID_GAP) / (MIN_COL_W + GRID_GAP)))
      setCols(c)
    }
    calc()
    window.addEventListener("resize", calc)
    return () => window.removeEventListener("resize", calc)
  }, [])

  const handleRemoveHistory = useCallback((id: string) => {
    setHistory(p => { const next = p.filter(v => v.id !== id); return next })
    setSessionVideos(p => p.filter(v => v.id !== id))
    fetch(`${HISTORY_API}/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(() => {})
  }, [])

  const generateRef = useRef<() => void>(() => {})

  const handleGenerate = useCallback(async () => {
    if (!prompt.trim()) { toast.error("Please enter a prompt"); return }
    setGenerating(true)
    const toastId = toast.loading(
      `Composing ${batchSize} video(s) at ${aspectRatio} — this may take 30s–3min...`,
      { duration: 10_000 }
    )
    try {
      const response = await fetch("/api/v1/videos/generations", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt, n: batchSize, aspect_ratio: aspectRatio, ...(model ? { model } : {}) }),
      })
      if (!response.ok) {
        const err = await response.json().catch(() => ({}))
        throw new Error(err.detail || "Generation failed")
      }
      const result = await response.json()
      const newVideos: GeneratedVideo[] = result.data.map((item: { url: string }) => ({
        id: Math.random().toString(36).substr(2, 9),
        url: item.url,
        prompt,
        timestamp: Date.now(),
        aspectRatio,
      }))
      setSessionVideos(prev => [...newVideos, ...prev])
      setPrompt("")
      fetch(HISTORY_API, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ videos: newVideos, max_items: maxItems })
      }).then(r => { if (r.ok) r.json().then(() => { fetch(HISTORY_API).then(r2 => r2.ok && r2.json()).then(d2 => { if (d2?.videos) setHistory(d2.videos) }) }) })
      toast.success(`Generated ${newVideos.length} video(s)`, { id: toastId })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Generation failed; please retry", { id: toastId })
    } finally {
      setGenerating(false)
    }
  }, [prompt, batchSize, aspectRatio, maxItems, model])

  useEffect(() => { generateRef.current = handleGenerate }, [handleGenerate])

  return (
    <div className="flex h-[calc(100vh-80px)] overflow-hidden">

      <div className="flex flex-col flex-1 min-w-0" style={{ transition: "flex 0.3s cubic-bezier(0.4,0,0.2,1)" }}>

        <div className="flex items-center justify-between px-6 pt-4 pb-3 shrink-0">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-fuchsia-500/10 flex items-center justify-center border border-fuchsia-500/20">
              <Film className="w-5 h-5 text-fuchsia-500" />
            </div>
            <div>
              <h1 className="text-3xl font-black tracking-tighter text-foreground flex items-center gap-2">
                Video Studio
                <span className="text-[9px] font-black px-2 py-0.5 rounded-md bg-amber-500/15 text-amber-500 border border-amber-500/20">BETA</span>
              </h1>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={() => setSidebarOpen(v => !v)}
              className={`flex items-center gap-2 px-3 py-1.5 rounded-xl border transition-all ${sidebarOpen
                ? "bg-fuchsia-500/20 border-fuchsia-500/40 text-fuchsia-500"
                : "bg-muted/30 border-border/40 text-muted-foreground hover:text-foreground hover:bg-muted/50"
                }`}
            >
              <History className="w-3.5 h-3.5" />
              <span className="text-[10px] font-black">History</span>
              {history.length > 0 && (
                <span className="px-1.5 py-0.5 rounded-md bg-fuchsia-500/20 text-fuchsia-500 text-[9px] font-black">
                  {history.length}
                </span>
              )}
            </button>

            <div ref={gearRef} className="relative">
              <button
                onClick={() => setGalleryOpen(v => !v)}
                className={`p-1.5 rounded-xl border transition-all ${galleryOpen
                  ? "bg-fuchsia-500/20 border-fuchsia-500/40 text-fuchsia-500"
                  : "bg-muted/30 border-border/40 text-muted-foreground hover:text-foreground hover:bg-muted/50"
                  }`}
              >
                <Settings className="w-3.5 h-3.5" />
              </button>
              {galleryOpen && (
                <div className="absolute right-0 top-10 z-50 w-64 p-5 rounded-2xl bg-background border border-border/60 shadow-2xl space-y-4">
                  <p className="text-[11px] font-black text-foreground">Gallery settings</p>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-[10px] text-muted-foreground font-black">Max stored videos</span>
                      <span className="text-sm font-black text-fuchsia-500 tabular-nums">{tempMaxItems}</span>
                    </div>
                    <input type="range" min={5} max={200} step={5} value={tempMaxItems}
                      onChange={e => setTempMaxItems(parseInt(e.target.value))}
                      className="w-full h-1.5 bg-muted/40 rounded-full appearance-none cursor-pointer accent-fuchsia-500" />
                    <div className="flex justify-between text-[9px] text-muted-foreground/50 font-black">
                      <span>5</span><span>50</span><span>200</span>
                    </div>
                  </div>
                  <button
                    onClick={() => {
                      const v = Math.max(5, Math.min(200, tempMaxItems))
                      localStorage.setItem("video_gallery_max_items", String(v))
                      setTempMaxItems(v)
                      setGalleryOpen(false)
                      toast.success(`Gallery cap set to ${v} videos`)
                    }}
                    className="w-full h-9 bg-fuchsia-500 text-white font-semibold rounded-xl text-sm hover:opacity-90 transition-all">
                    Save
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>

        {sessionVideos.length === 0 && (
          <div className="mx-6 mb-2 p-3 rounded-2xl bg-amber-500/5 border border-amber-500/15 flex items-start gap-3">
            <AlertCircle className="w-4 h-4 text-amber-500 shrink-0 mt-0.5" />
            <div className="text-[11px] text-amber-500/90 leading-relaxed">
              <span className="font-black">Heads-up:</span> Video generation is experimental.
              Availability depends on whether the upstream Qwen account has video features enabled, and
              renders typically take 30 seconds to 3 minutes. If a request fails repeatedly, the feature
              may not be available on your current accounts.
            </div>
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-6 pb-2">
          {generating && sessionVideos.length === 0 ? (
            <div style={{ display: "grid", gridTemplateColumns: `repeat(${cols}, 1fr)`, gap: `${GRID_GAP}px` }}>
              {Array.from({ length: batchSize }).map((_, i) => (
                <div key={i} className="rounded-2xl overflow-hidden bg-muted/20 border border-border/30 animate-pulse"
                  style={{ aspectRatio: aspectRatio.replace(":", " / ") }}>
                  <div className="w-full h-full flex flex-col items-center justify-center gap-3">
                    <div className="w-10 h-10 rounded-xl bg-muted/30 flex items-center justify-center">
                      <Film className="w-5 h-5 text-fuchsia-500/40 animate-spin" style={{ animationDuration: "4s" }} />
                    </div>
                    <div className="space-y-1.5 text-center">
                      <div className="h-2 w-24 bg-muted/30 rounded-full mx-auto" />
                      <div className="h-1.5 w-16 bg-muted/20 rounded-full mx-auto" />
                      <p className="text-[9px] text-muted-foreground/40 mt-2">This may take 30s–3min</p>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : sessionVideos.length > 0 ? (
            <div style={{
              display: "grid",
              gridTemplateColumns: `repeat(${cols}, 1fr)`,
              gap: `${GRID_GAP}px`,
              alignItems: "start",
            }}>
              {sessionVideos.map(v => (
                <VideoCard
                  key={v.id}
                  video={v}
                  onClick={() => setLightbox(v)}
                />
              ))}
            </div>
          ) : (
            <div className="flex items-center justify-center h-full min-h-[200px]">
              <div className="text-center space-y-3">
                <Film className="w-16 h-16 mx-auto text-muted-foreground/10" />
                <p className="text-sm font-black text-muted-foreground/30">Type a prompt below to start creating videos</p>
                <p className="text-[10px] text-muted-foreground/20">History is auto-saved; click the icon in the top-right to view.</p>
              </div>
            </div>
          )}
        </div>

        <div className="px-6 pt-2 pb-4 shrink-0">
          <div className="glass-card p-4 rounded-[1.5rem] border border-fuchsia-500/15">
            <div className="space-y-2.5">
              <textarea
                value={prompt}
                onChange={e => setPrompt(e.target.value)}
                onKeyDown={e => {
                  if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                    e.preventDefault()
                    e.stopPropagation()
                    generateRef.current()
                  }
                }}
                placeholder="Describe the video you want to generate... (Ctrl+Enter to submit)"
                className="w-full h-14 bg-transparent border-none outline-none text-foreground placeholder:text-muted-foreground/30 resize-none text-sm leading-relaxed"
              />
              <div className="flex items-center gap-2 border-t border-border/30 pt-2.5">
                <div ref={aspectRef} className="relative">
                  <button
                    onClick={() => setAspectOpen(v => !v)}
                    className={`h-7 px-3 rounded-lg text-[11px] font-black transition-all border flex items-center gap-1.5 ${aspectOpen
                      ? "bg-fuchsia-500/20 border-fuchsia-500/40 text-fuchsia-500"
                      : "bg-muted/30 text-muted-foreground border-border/40 hover:bg-muted/50"
                      }`}
                  >
                    {aspectRatio === "1:1" && <SquareIcon className="w-3 h-3" />}
                    {(aspectRatio === "16:9" || aspectRatio === "4:3") && <RectangleHorizontal className="w-3 h-3" />}
                    {(aspectRatio === "9:16" || aspectRatio === "3:4") && <RectangleVertical className="w-3 h-3" />}
                    <span>{aspectRatio}</span>
                  </button>
                  {aspectOpen && (
                    <div className="absolute bottom-full left-0 mb-2 z-50 bg-popover/95 backdrop-blur-xl border border-border/60 rounded-2xl shadow-2xl p-1.5 min-w-[120px]">
                      {ASPECT_RATIOS.map(r => (
                        <button
                          key={r.value}
                          onClick={() => { setAspectRatio(r.value); setAspectOpen(false) }}
                          className={`w-full px-3 py-2 rounded-xl text-[11px] font-black transition-all flex items-center justify-between gap-2 ${aspectRatio === r.value
                            ? "bg-fuchsia-500 text-white shadow-md shadow-fuchsia-500/20"
                            : "text-foreground/70 hover:bg-muted/50"
                            }`}
                        >
                          <span className="flex items-center gap-2">
                            <span
                              className={`block border-2 rounded-sm ${aspectRatio === r.value ? "border-white" : "border-current opacity-50"}`}
                              style={{ width: r.w, height: r.h }}
                            />
                            <span>{r.value}</span>
                          </span>
                          {aspectRatio === r.value && <Check className="w-3 h-3" />}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                <span className="text-[9px] font-black text-muted-foreground/40 ml-1">Count</span>
                {[1, 2].map(s => (
                  <button key={s} onClick={() => setBatchSize(s)}
                    className={`w-7 h-7 rounded-lg text-[11px] font-black transition-all border ${batchSize === s
                      ? "bg-fuchsia-500 text-white border-fuchsia-400"
                      : "bg-muted/30 text-muted-foreground border-border/40 hover:bg-muted/50"
                      }`}>
                    {s}
                  </button>
                ))}
                <div ref={modelRef} className="relative">
                  <button onClick={() => setModelOpen(v => !v)}
                    className={`h-7 px-2.5 rounded-lg text-[10px] font-bold border flex items-center gap-1.5 transition-all max-w-[180px] ${modelOpen ? "bg-fuchsia-500/15 border-fuchsia-500/30 text-fuchsia-400" : "bg-muted/30 border-border/40 text-muted-foreground hover:text-foreground"}`}>
                    <span className="truncate">{model || "Loading…"}</span>
                    <ChevronDown className={`w-3 h-3 shrink-0 transition-transform ${modelOpen ? "rotate-180" : ""}`} />
                  </button>
                  {modelOpen && (
                    <div className="absolute bottom-full left-0 mb-2 z-50 bg-popover border border-border/40 rounded-xl shadow-2xl p-1 min-w-[220px] max-h-72 overflow-y-auto custom-scrollbar">
                      {models.length === 0 && <div className="px-3 py-2 text-[10px] text-muted-foreground">No models</div>}
                      {models.map(m => (
                        <button key={m} onClick={() => { setModel(m); setModelOpen(false) }}
                          className={`w-full px-3 py-2 rounded-lg text-[11px] font-mono font-semibold flex items-center justify-between transition-all ${model === m ? "bg-fuchsia-500 text-white" : "text-foreground/70 hover:bg-muted/40"}`}>
                          <span className="truncate">{m}</span>
                          {model === m && <Check className="w-3 h-3 shrink-0" />}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                <div className="flex-1" />
                <button
                  onClick={() => generateRef.current()}
                  disabled={generating}
                  className="h-7 px-4 rounded-lg bg-foreground text-background font-black text-[11px] hover:scale-[1.02] active:scale-[0.98] transition-all disabled:opacity-50 flex items-center gap-1.5"
                >
                  {generating ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Sparkles className="w-3 h-3" />}
                  {generating ? "Rendering..." : "Generate"}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div
        className="shrink-0 overflow-hidden border-l border-border/40 bg-background"
        style={{
          width: sidebarOpen ? SIDEBAR_W : 0,
          transition: "width 0.3s cubic-bezier(0.4, 0, 0.2, 1)",
        }}
      >
        <div style={{ width: SIDEBAR_W }} className="h-full flex flex-col">
          <div className="flex items-center justify-between px-4 pt-4 pb-3 border-b border-border/30 shrink-0">
            <div className="flex items-center gap-2">
              <Clock className="w-3.5 h-3.5 text-muted-foreground" />
              <span className="text-sm font-semibold text-foreground">History</span>
              <span className="text-[9px] text-muted-foreground/50 font-mono">{history.length}</span>
            </div>
            <button onClick={() => setSidebarOpen(false)}
              className="p-1.5 rounded-lg bg-muted/30 hover:bg-muted/60 text-muted-foreground transition-all">
              <X className="w-4 h-4" />
            </button>
          </div>
          <HistorySidebar
            items={history}
            onClear={() => { setHistory([]); fetch(HISTORY_API, { method: "DELETE" }).catch(() => {}) }}
            onLightbox={v => setLightbox(v)}
            onRemove={handleRemoveHistory}
          />
        </div>
      </div>

      {lightbox && <VideoLightbox video={lightbox} onClose={() => setLightbox(null)} />}
    </div>
  )
}
