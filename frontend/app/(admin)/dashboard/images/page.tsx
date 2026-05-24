'use client'

import { useState, useEffect, useCallback, useRef, useMemo, memo } from "react"
import {
  Image as ImageIcon, Sparkles, Download, Trash2, RefreshCw,
  ChevronLeft, ChevronRight, X, History, Clock, Settings, Check,
  Square as SquareIcon, RectangleHorizontal, RectangleVertical, Maximize2,
  ChevronDown
} from "lucide-react"
import { toast } from "sonner"

const STORAGE_KEY = "qwenpi_image_history"

const PROVIDER_PREFIX: Record<string, string> = {
  qwen: "qw/",
  kiro: "kiro/",
  "opencode-zen": "zen/",
  "gemini-web": "gw/",
  openai: "openai/",
}

type AspectRatio = "1:1" | "4:3" | "3:4" | "16:9" | "9:16"
const ASPECT_RATIOS: { value: AspectRatio; label: string; w: number; h: number }[] = [
  { value: "1:1",  label: "Square",     w: 16, h: 16 },
  { value: "4:3",  label: "Landscape",  w: 20, h: 15 },
  { value: "3:4",  label: "Portrait",   w: 15, h: 20 },
  { value: "16:9", label: "Wide",       w: 24, h: 14 },
  { value: "9:16", label: "Tall",       w: 14, h: 24 },
]

const RATIO_TO_RES: Record<AspectRatio, string> = {
  "1:1": "1024×1024", "4:3": "1024×768", "3:4": "768×1024",
  "16:9": "1280×720", "9:16": "720×1280",
}

interface GeneratedImage {
  id: string; url: string; prompt: string; timestamp: number; aspectRatio?: AspectRatio
}

const HISTORY_API = "/api/admin/history/images"

function SkeletonCard({ ratio }: { ratio: AspectRatio }) {
  return (
    <div className="rounded-2xl overflow-hidden bg-muted/20 border border-border/30 animate-pulse"
      style={{ aspectRatio: ratio.replace(":", " / ") }}>
      <div className="w-full h-full flex flex-col items-center justify-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-muted/30 flex items-center justify-center">
          <Sparkles className="w-5 h-5 text-indigo-500/40 animate-spin" style={{ animationDuration: "3s" }} />
        </div>
        <div className="space-y-1.5 text-center">
          <div className="h-2 w-20 bg-muted/30 rounded-full mx-auto" />
          <div className="h-2 w-14 bg-muted/20 rounded-full mx-auto" />
        </div>
      </div>
    </div>
  )
}

function ImageCard({ img, onClick }: { img: GeneratedImage; onClick: () => void }) {
  const [loaded, setLoaded] = useState(false)
  const ratio = img.aspectRatio || "1:1"
  return (
    <div
      className="group relative rounded-2xl overflow-hidden bg-card border border-border/30 cursor-pointer shadow-sm hover:shadow-xl hover:shadow-indigo-500/5 transition-all duration-300 hover:-translate-y-0.5"
      style={{ aspectRatio: ratio.replace(":", " / ") }}
      onClick={onClick}
    >
      {!loaded && (
        <div className="absolute inset-0 bg-muted/20 animate-pulse flex items-center justify-center">
          <ImageIcon className="w-8 h-8 text-muted-foreground/20" />
        </div>
      )}
      <img
        src={img.url} alt={img.prompt} loading="lazy"
        onLoad={() => setLoaded(true)}
        className={`w-full h-full object-cover transition-all duration-500 group-hover:scale-[1.03] ${loaded ? "opacity-100" : "opacity-0"}`}
      />
      <div className="absolute top-3 left-3 px-2 py-0.5 rounded-md bg-black/60 backdrop-blur-sm text-[9px] font-bold text-white/80 opacity-0 group-hover:opacity-100 transition-opacity">
        {RATIO_TO_RES[ratio]} · {ratio}
      </div>
      <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-all duration-300 flex flex-col justify-end p-4">
        <p className="text-white text-[11px] leading-snug line-clamp-2 font-medium mb-2">{img.prompt}</p>
        <div className="flex items-center justify-between">
          <span className="text-[9px] text-white/50 font-mono">
            {new Date(img.timestamp).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" })}
          </span>
          <div className="flex gap-1.5">
            <a href={img.url} download onClick={e => e.stopPropagation()}
              className="p-1.5 rounded-lg bg-white/10 hover:bg-white/90 text-white hover:text-black transition-all">
              <Download className="w-3 h-3" />
            </a>
            <button onClick={e => { e.stopPropagation(); onClick() }}
              className="p-1.5 rounded-lg bg-white/10 hover:bg-white/90 text-white hover:text-black transition-all">
              <Maximize2 className="w-3 h-3" />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function Lightbox({ images, currentIndex, onClose, onNav }: {
  images: GeneratedImage[]; currentIndex: number; onClose: () => void; onNav: (i: number) => void
}) {
  const img = images[currentIndex]
  const hasPrev = currentIndex > 0, hasNext = currentIndex < images.length - 1
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose()
      if (e.key === "ArrowLeft" && hasPrev) onNav(currentIndex - 1)
      if (e.key === "ArrowRight" && hasNext) onNav(currentIndex + 1)
    }
    window.addEventListener("keydown", h, true)
    return () => window.removeEventListener("keydown", h, true)
  }, [currentIndex, hasPrev, hasNext, onClose, onNav])

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/85 backdrop-blur-xl" onClick={onClose}>
      <button onClick={onClose} className="absolute top-5 right-5 p-2 rounded-xl bg-white/10 hover:bg-white/20 text-white"><X className="w-5 h-5" /></button>
      {hasPrev && <button onClick={e => { e.stopPropagation(); onNav(currentIndex - 1) }} className="absolute left-4 p-3 rounded-2xl bg-white/10 hover:bg-white/20 text-white"><ChevronLeft className="w-6 h-6" /></button>}
      <div className="max-w-[90vw] max-h-[90vh] flex flex-col items-center gap-4" onClick={e => e.stopPropagation()}>
        <img src={img.url} alt={img.prompt} className="max-w-full max-h-[80vh] object-contain rounded-2xl shadow-2xl" />
        <div className="text-center space-y-2 max-w-xl">
          <p className="text-white/90 text-sm">{img.prompt}</p>
          <div className="flex items-center justify-center gap-3 text-[10px] text-white/40">
            <span>{currentIndex + 1}/{images.length}</span>
            <span className="px-2 py-0.5 rounded bg-white/10">{RATIO_TO_RES[img.aspectRatio || "1:1"]}</span>
            <a href={img.url} download className="px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 flex items-center gap-1"><Download className="w-3 h-3" /> Download</a>
          </div>
        </div>
      </div>
      {hasNext && <button onClick={e => { e.stopPropagation(); onNav(currentIndex + 1) }} className="absolute right-4 p-3 rounded-2xl bg-white/10 hover:bg-white/20 text-white"><ChevronRight className="w-6 h-6" /></button>}
    </div>
  )
}

const HistorySidebar = memo(function HistorySidebar({ images, onClear, onLightbox, onRemove }: {
  images: GeneratedImage[]; onClear: () => void
  onLightbox: (imgs: GeneratedImage[], idx: number) => void; onRemove: (id: string) => void
}) {
  if (images.length === 0) return (
    <div className="flex flex-col items-center justify-center h-full text-center gap-3 p-8">
      <ImageIcon className="w-10 h-10 text-muted-foreground/15" />
      <p className="text-xs text-muted-foreground/40 font-medium">No history yet</p>
    </div>
  )
  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-3 space-y-2 custom-scrollbar">
        {images.map((img, i) => (
          <div key={img.id} className="group relative rounded-xl overflow-hidden bg-muted/20 border border-border/30 cursor-pointer"
            style={{ aspectRatio: (img.aspectRatio || "1:1").replace(":", " / ") }}
            onClick={() => onLightbox(images, i)}>
            <img src={img.url} alt={img.prompt} loading="lazy" className="w-full h-full object-cover" />
            <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-transparent opacity-0 group-hover:opacity-100 transition-all p-2 flex flex-col justify-end">
              <p className="text-white text-[9px] line-clamp-1">{img.prompt}</p>
              <div className="flex justify-between mt-1">
                <span className="text-[8px] text-white/40">{img.aspectRatio || "1:1"}</span>
                <button onClick={e => { e.stopPropagation(); onRemove(img.id) }} className="p-0.5 rounded bg-white/10 hover:bg-rose-500 text-white transition-all">
                  <Trash2 className="w-2.5 h-2.5" />
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
      <div className="px-3 py-2.5 border-t border-border/20 shrink-0">
        <button onClick={onClear} className="w-full py-1.5 rounded-lg text-[10px] font-bold text-rose-500/60 hover:text-rose-500 hover:bg-rose-500/10 transition-all flex items-center justify-center gap-1">
          <Trash2 className="w-3 h-3" /> Clear all
        </button>
      </div>
    </div>
  )
})

export default function ImagePage() {
  const [prompt, setPrompt] = useState("")
  const [generating, setGenerating] = useState(false)
  const [batchSize, setBatchSize] = useState(1)
  const [aspectRatio, setAspectRatio] = useState<AspectRatio>("1:1")
  const [aspectOpen, setAspectOpen] = useState(false)
  const aspectRef = useRef<HTMLDivElement>(null)

  const [sessionImages, setSessionImages] = useState<GeneratedImage[]>([])
  const [history, setHistory] = useState<GeneratedImage[]>([])
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [lightbox, setLightbox] = useState<{ images: GeneratedImage[]; index: number } | null>(null)

  const [galleryOpen, setGalleryOpen] = useState(false)
  const [tempMaxItems, setTempMaxItems] = useState(100)
  const [maxItems, setMaxItems] = useState(100)
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
        const saved = localStorage.getItem("qwenpi_image_model") || ""
        if (saved && list.includes(saved)) setModel(saved)
        else if (list.length > 0) setModel(list[0])
      })
      .catch(err => toast.error(`Load models: ${err.message}`))
  }, [])
  useEffect(() => { if (model) localStorage.setItem("qwenpi_image_model", model) }, [model])

  useEffect(() => {
    if (!modelOpen) return
    const h = (e: MouseEvent) => { if (modelRef.current && !modelRef.current.contains(e.target as Node)) setModelOpen(false) }
    document.addEventListener("mousedown", h); return () => document.removeEventListener("mousedown", h)
  }, [modelOpen])

  useEffect(() => {
    fetch(HISTORY_API).then(r => r.ok && r.json()).then(d => { if (d?.images) setHistory(d.images) }).catch(() => {})
    const savedRatio = localStorage.getItem("qwenpi_aspect_ratio") as AspectRatio
    if (savedRatio) setAspectRatio(savedRatio)
    const savedMax = parseInt(localStorage.getItem("gallery_max_items") || "100", 10)
    setTempMaxItems(savedMax)
    setMaxItems(savedMax)
  }, [])
  useEffect(() => { localStorage.setItem("qwenpi_aspect_ratio", aspectRatio) }, [aspectRatio])

  useEffect(() => {
    if (!galleryOpen) return
    const h = (e: MouseEvent) => { if (gearRef.current && !gearRef.current.contains(e.target as Node)) setGalleryOpen(false) }
    document.addEventListener("mousedown", h); return () => document.removeEventListener("mousedown", h)
  }, [galleryOpen])
  useEffect(() => {
    if (!aspectOpen) return
    const h = (e: MouseEvent) => { if (aspectRef.current && !aspectRef.current.contains(e.target as Node)) setAspectOpen(false) }
    document.addEventListener("mousedown", h); return () => document.removeEventListener("mousedown", h)
  }, [aspectOpen])

  const SIDEBAR_W = 320
  const [cols, setCols] = useState(3)
  useEffect(() => {
    const calc = () => { const w = window.innerWidth - 288 - (sidebarOpen ? SIDEBAR_W : 0) - 48; setCols(Math.max(2, Math.floor(w / 220))) }
    calc(); window.addEventListener("resize", calc); return () => window.removeEventListener("resize", calc)
  }, [sidebarOpen])

  const handleRemoveHistory = useCallback((id: string) => {
    setHistory(p => { const next = p.filter(i => i.id !== id); return next })
    setSessionImages(p => p.filter(i => i.id !== id))
    fetch(`${HISTORY_API}/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(() => {})
  }, [])

  const generateRef = useRef<() => void>(() => {})
  const handleGenerate = useCallback(async () => {
    if (!prompt.trim()) { toast.error("Enter a prompt"); return }
    setGenerating(true)
    const toastId = toast.loading(`Generating ${batchSize} image(s) at ${aspectRatio}...`)
    try {
      const res = await fetch("/api/v1/images/generations", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt, n: batchSize, aspect_ratio: aspectRatio, ...(model ? { model } : {}) })
      })
      if (!res.ok) { const e = await res.json(); throw new Error(e.detail || "Failed") }
      const data = await res.json()
      const imgs: GeneratedImage[] = data.data.map((d: { url: string }) => ({
        id: Math.random().toString(36).slice(2, 11), url: d.url, prompt, timestamp: Date.now(), aspectRatio
      }))
      setSessionImages(p => [...imgs, ...p])
      setPrompt("")
      fetch(HISTORY_API, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ images: imgs, max_items: maxItems })
      }).then(r => { if (r.ok) r.json().then(d => { fetch(HISTORY_API).then(r2 => r2.ok && r2.json()).then(d2 => { if (d2?.images) setHistory(d2.images) }) }) })
      toast.success(`Generated ${imgs.length} image(s)`, { id: toastId })
    } catch (e: any) {
      toast.error(e.message || "Generation failed", { id: toastId })
    } finally { setGenerating(false) }
  }, [prompt, batchSize, aspectRatio, maxItems, model])
  useEffect(() => { generateRef.current = handleGenerate }, [handleGenerate])

  return (
    <div className="flex h-[calc(100vh-80px)] overflow-hidden animate-fade-in-up">
      <div className="flex flex-col flex-1 min-w-0">
        <div className="flex items-center justify-between px-6 pt-4 pb-3 shrink-0">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-br from-indigo-500/20 to-fuchsia-500/10 border border-indigo-500/20 flex items-center justify-center">
              <ImageIcon className="w-4 h-4 text-indigo-400" />
            </div>
            <div>
              <h1 className="text-xl font-black tracking-tight text-foreground">Image Studio</h1>
              <p className="text-[10px] text-muted-foreground/60">Powered by Qwen · {RATIO_TO_RES[aspectRatio]}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => setSidebarOpen(v => !v)}
              className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-[10px] font-bold transition-all ${sidebarOpen ? "bg-indigo-500/15 border-indigo-500/30 text-indigo-400" : "bg-muted/20 border-border/30 text-muted-foreground hover:text-foreground"}`}>
              <History className="w-3 h-3" />
              {history.length > 0 && <span className="px-1 rounded bg-indigo-500/20 text-indigo-400 text-[9px]">{history.length}</span>}
            </button>
            <div ref={gearRef} className="relative">
              <button onClick={() => setGalleryOpen(v => !v)} className="p-1.5 rounded-lg border border-border/30 bg-muted/20 text-muted-foreground hover:text-foreground transition-all">
                <Settings className="w-3.5 h-3.5" />
              </button>
              {galleryOpen && (
                <div className="absolute right-0 top-9 z-50 w-56 p-4 rounded-xl bg-popover border border-border/40 shadow-2xl space-y-3">
                  <p className="text-[10px] font-bold">Gallery settings</p>
                  <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                    <span>Max images</span><span className="font-bold text-indigo-400">{tempMaxItems}</span>
                  </div>
                  <input type="range" min={10} max={500} step={10} value={tempMaxItems} onChange={e => setTempMaxItems(+e.target.value)}
                    className="w-full h-1 bg-muted/40 rounded-full appearance-none cursor-pointer accent-indigo-500" />
                  <button onClick={() => { localStorage.setItem("gallery_max_items", String(tempMaxItems)); setGalleryOpen(false); toast.success(`Cap: ${tempMaxItems}`) }}
                    className="w-full h-7 bg-indigo-500 text-white font-bold rounded-lg text-[10px] hover:opacity-90 transition-all">Save</button>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-6 pb-2 custom-scrollbar">
          {generating && sessionImages.length === 0 ? (
            <div style={{ display: "grid", gridTemplateColumns: `repeat(${cols}, 1fr)`, gap: 12 }}>
              {Array.from({ length: batchSize }).map((_, i) => <SkeletonCard key={i} ratio={aspectRatio} />)}
            </div>
          ) : sessionImages.length > 0 ? (
            <div style={{ display: "grid", gridTemplateColumns: `repeat(${cols}, 1fr)`, gap: 12, alignItems: "start" }}>
              {generating && Array.from({ length: batchSize }).map((_, i) => <SkeletonCard key={`sk-${i}`} ratio={aspectRatio} />)}
              {sessionImages.map((img, idx) => (
                <ImageCard key={img.id} img={img} onClick={() => setLightbox({ images: sessionImages, index: idx })} />
              ))}
            </div>
          ) : (
            <div className="flex items-center justify-center h-full">
              <div className="text-center space-y-4">
                <div className="w-16 h-16 rounded-2xl bg-muted/10 border border-border/20 flex items-center justify-center mx-auto">
                  <ImageIcon className="w-8 h-8 text-muted-foreground/15" />
                </div>
                <div>
                  <p className="text-sm font-bold text-muted-foreground/40">Create something</p>
                  <p className="text-[10px] text-muted-foreground/25 mt-1">Type a prompt below and hit generate</p>
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="px-6 pt-2 pb-4 shrink-0">
          <div className="glass-card p-3 rounded-2xl border-indigo-500/10">
            <textarea
              value={prompt} onChange={e => setPrompt(e.target.value)}
              onKeyDown={e => { if ((e.ctrlKey || e.metaKey) && e.key === "Enter") { e.preventDefault(); generateRef.current() } }}
              placeholder="Describe the image you want to create..."
              className="w-full h-12 bg-transparent border-none outline-none text-foreground placeholder:text-muted-foreground/30 resize-none text-sm leading-relaxed"
            />
            <div className="flex items-center gap-2 border-t border-border/20 pt-2">
              <div ref={aspectRef} className="relative">
                <button onClick={() => setAspectOpen(v => !v)}
                  className={`h-7 px-2.5 rounded-lg text-[10px] font-bold border flex items-center gap-1.5 transition-all ${aspectOpen ? "bg-indigo-500/15 border-indigo-500/30 text-indigo-400" : "bg-muted/20 border-border/30 text-muted-foreground hover:text-foreground"}`}>
                  {aspectRatio === "1:1" && <SquareIcon className="w-3 h-3" />}
                  {(aspectRatio === "16:9" || aspectRatio === "4:3") && <RectangleHorizontal className="w-3 h-3" />}
                  {(aspectRatio === "9:16" || aspectRatio === "3:4") && <RectangleVertical className="w-3 h-3" />}
                  {aspectRatio}
                </button>
                {aspectOpen && (
                  <div className="absolute bottom-full left-0 mb-2 z-50 bg-popover border border-border/40 rounded-xl shadow-2xl p-1 min-w-[130px]">
                    {ASPECT_RATIOS.map(r => (
                      <button key={r.value} onClick={() => { setAspectRatio(r.value); setAspectOpen(false) }}
                        className={`w-full px-3 py-2 rounded-lg text-[11px] font-bold flex items-center justify-between transition-all ${aspectRatio === r.value ? "bg-indigo-500 text-white" : "text-foreground/70 hover:bg-muted/40"}`}>
                        <span className="flex items-center gap-2">
                          <span className={`block border-2 rounded-sm ${aspectRatio === r.value ? "border-white" : "border-current/40"}`} style={{ width: r.w, height: r.h }} />
                          {r.value}
                        </span>
                        {aspectRatio === r.value && <Check className="w-3 h-3" />}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <div className="flex items-center gap-1">
                {[1, 2, 4].map(n => (
                  <button key={n} onClick={() => setBatchSize(n)}
                    className={`w-6 h-6 rounded-md text-[10px] font-bold transition-all ${batchSize === n ? "bg-indigo-500 text-white" : "bg-muted/20 text-muted-foreground border border-border/30 hover:bg-muted/40"}`}>
                    {n}
                  </button>
                ))}
              </div>
              <div ref={modelRef} className="relative">
                <button onClick={() => setModelOpen(v => !v)}
                  className={`h-7 px-2.5 rounded-lg text-[10px] font-bold border flex items-center gap-1.5 transition-all max-w-[180px] ${modelOpen ? "bg-indigo-500/15 border-indigo-500/30 text-indigo-400" : "bg-muted/20 border-border/30 text-muted-foreground hover:text-foreground"}`}>
                  <span className="truncate">{model || "Loading…"}</span>
                  <ChevronDown className={`w-3 h-3 shrink-0 transition-transform ${modelOpen ? "rotate-180" : ""}`} />
                </button>
                {modelOpen && (
                  <div className="absolute bottom-full left-0 mb-2 z-50 bg-popover border border-border/40 rounded-xl shadow-2xl p-1 min-w-[220px] max-h-72 overflow-y-auto custom-scrollbar">
                    {models.length === 0 && <div className="px-3 py-2 text-[10px] text-muted-foreground">No models</div>}
                    {models.map(m => (
                      <button key={m} onClick={() => { setModel(m); setModelOpen(false) }}
                        className={`w-full px-3 py-2 rounded-lg text-[11px] font-mono font-semibold flex items-center justify-between transition-all ${model === m ? "bg-indigo-500 text-white" : "text-foreground/70 hover:bg-muted/40"}`}>
                        <span className="truncate">{m}</span>
                        {model === m && <Check className="w-3 h-3 shrink-0" />}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <div className="flex-1" />
              <button onClick={() => generateRef.current()} disabled={generating}
                className="h-7 px-4 rounded-lg bg-gradient-to-r from-indigo-500 to-fuchsia-500 text-white font-bold text-[11px] flex items-center gap-1.5 hover:opacity-90 active:scale-[0.97] transition-all disabled:opacity-50 shadow-sm shadow-indigo-500/20">
                {generating ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Sparkles className="w-3 h-3" />}
                {generating ? "Creating..." : "Generate"}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className="shrink-0 overflow-hidden border-l border-border/20 bg-background transition-all duration-300"
        style={{ width: sidebarOpen ? SIDEBAR_W : 0 }}>
        <div style={{ width: SIDEBAR_W }} className="h-full flex flex-col">
          <div className="flex items-center justify-between px-3 pt-3 pb-2 border-b border-border/20 shrink-0">
            <div className="flex items-center gap-1.5">
              <Clock className="w-3 h-3 text-muted-foreground/50" />
              <span className="text-xs font-bold">History</span>
              <span className="text-[9px] text-muted-foreground/40">{history.length}</span>
            </div>
            <button onClick={() => setSidebarOpen(false)} className="p-1 rounded-md hover:bg-muted/40 text-muted-foreground"><X className="w-3.5 h-3.5" /></button>
          </div>
          <HistorySidebar images={history} onClear={() => { setHistory([]); fetch(HISTORY_API, { method: "DELETE" }).catch(() => {}) }}
            onLightbox={(imgs, idx) => setLightbox({ images: imgs, index: idx })} onRemove={handleRemoveHistory} />
        </div>
      </div>

      {lightbox && <Lightbox images={lightbox.images} currentIndex={lightbox.index} onClose={() => setLightbox(null)} onNav={i => setLightbox(p => p ? { ...p, index: i } : null)} />}
    </div>
  )
}
