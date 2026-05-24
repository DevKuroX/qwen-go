'use client';

import Link from "next/link"
import {
  Zap, Shield, Layers, Globe, ArrowRight, Terminal,
  Image, Film, MessageSquare, Code2, Server, Lock,
  GitBranch, BookOpen, ChevronRight, Copy, Check,
  Cpu, BarChart3, RefreshCw, Sparkles, Play
} from "lucide-react"
import QCatIcon from "@/components/QCatIcon"
import { useState, useEffect, useRef } from "react"

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 2000) }}
      className="p-1.5 rounded-md hover:bg-white/10 transition-colors text-white/40 hover:text-white/80"
      title="Copy"
    >
      {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

/* Animated counter */
function Counter({ end, suffix = "" }: { end: number; suffix?: string }) {
  const [val, setVal] = useState(0)
  const ref = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    const obs = new IntersectionObserver(([e]) => {
      if (e.isIntersecting) {
        let start = 0
        const step = Math.max(1, Math.floor(end / 40))
        const t = setInterval(() => {
          start += step
          if (start >= end) { start = end; clearInterval(t) }
          setVal(start)
        }, 30)
        obs.disconnect()
      }
    }, { threshold: 0.5 })
    if (ref.current) obs.observe(ref.current)
    return () => obs.disconnect()
  }, [end])
  return <span ref={ref}>{val}{suffix}</span>
}

const PROTOCOLS = [
  { name: "OpenAI", color: "from-emerald-500 to-teal-500", icon: MessageSquare, endpoints: ["/v1/chat/completions", "/v1/responses", "/v1/images/generations", "/v1/videos/generations", "/v1/models"] },
  { name: "Anthropic", color: "from-orange-500 to-amber-500", icon: Globe, endpoints: ["/v1/messages (streaming + tool_use)"] },
  { name: "Gemini", color: "from-blue-500 to-cyan-500", icon: Sparkles, endpoints: ["/v1beta/models/{m}:generateContent", "/v1beta/models/{m}:streamGenerateContent"] },
]

const FEATURES = [
  { icon: Layers, title: "Multi-Protocol Gateway", desc: "Serve OpenAI, Anthropic, and Gemini formats from a single deployment. Any client, one endpoint." },
  { icon: Shield, title: "Account Pool & Circuit Breaker", desc: "Min-Heap scheduling, 6-state lifecycle, exponential backoff, adaptive RPM learning per account." },
  { icon: Zap, title: "Auto-Registration & Replenishment", desc: "Automatically register new Qwen accounts when the pool runs low. GuerrillaMail, MoeMail, TempMail supported." },
  { icon: Image, title: "Image Generation", desc: "DALL-E compatible endpoint with 5 aspect ratios. Batch up to 4 images per request." },
  { icon: Film, title: "Video Generation (Beta)", desc: "Text-to-video via Qwen's native T2V pipeline. Experimental but functional." },
  { icon: Cpu, title: "Multi-Engine", desc: "httpx direct, Camoufox browser fingerprint, or hybrid mode. Choose your anti-detection strategy." },
  { icon: Lock, title: "API Key Management", desc: "Generate downstream keys with independent quotas. Distribute access without exposing admin credentials." },
  { icon: BarChart3, title: "Real-time Monitoring", desc: "Live dashboard with RPM/TPM charts, health timeline, account status, and SSE push notifications." },
  { icon: RefreshCw, title: "Self-Healing", desc: "Automatic token refresh, expired JWT cleanup, circuit recovery, and emergency replenishment on exhaustion." },
]

const STATS = [
  { label: "API Protocols", value: 4 },
  { label: "Built-in Aliases", value: 39, suffix: "+" },
  { label: "Account States", value: 6 },
  { label: "Aspect Ratios", value: 5 },
]

const CLIENTS = ["Cherry Studio", "Cursor", "Claude Code", "LobeChat", "NextChat", "New-API", "One-API", "OpenAI SDK"]

export default function LandingPage() {
  return (
    <div className="min-h-screen bg-background text-foreground overflow-x-hidden">
      {/* ─── Navbar ─── */}
      <nav className="fixed top-0 inset-x-0 z-50 h-14 flex items-center justify-between px-6 md:px-10 border-b border-border/10 bg-background/60 backdrop-blur-2xl">
        <div className="flex items-center gap-2.5">
          <div className="h-8 w-8 rounded-lg flex items-center justify-center bg-gradient-to-br from-indigo-500 to-fuchsia-500 shadow-lg shadow-indigo-500/20">
            <QCatIcon className="h-5 w-5" />
          </div>
          <span className="font-black text-base tracking-tight">Qwenpi</span>
          <span className="text-[9px] font-bold text-muted-foreground/50 bg-muted/30 px-1.5 py-0.5 rounded ml-1">v2.0</span>
        </div>
        <div className="flex items-center gap-5">
          <Link href="/docs" className="text-sm text-muted-foreground hover:text-foreground transition-colors font-medium hidden sm:block">Docs</Link>
          <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer" className="text-muted-foreground hover:text-foreground transition-colors hidden sm:block">
            <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
          </a>
          <Link href="/login" className="h-8 px-4 rounded-lg bg-foreground text-background text-xs font-bold flex items-center gap-1.5 hover:opacity-90 transition-opacity">
            Dashboard <ArrowRight className="w-3 h-3" />
          </Link>
        </div>
      </nav>

      {/* ─── Hero ─── */}
      <section className="relative pt-28 pb-24 px-6 md:px-10 max-w-6xl mx-auto">
        {/* Ambient */}
        <div className="absolute inset-0 pointer-events-none overflow-hidden">
          <div className="absolute top-[-10%] left-[20%] w-[600px] h-[600px] bg-indigo-500/[0.07] rounded-full blur-[150px] animate-pulse" style={{animationDuration:"8s"}} />
          <div className="absolute top-[20%] right-[10%] w-[400px] h-[400px] bg-fuchsia-500/[0.05] rounded-full blur-[120px] animate-pulse" style={{animationDuration:"12s"}} />
          <div className="absolute bottom-0 left-[50%] w-[500px] h-[300px] bg-violet-500/[0.04] rounded-full blur-[100px]" />
        </div>

        <div className="relative z-10 text-center space-y-7 max-w-3xl mx-auto">
          {/* Badge */}
          <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gradient-to-r from-indigo-500/10 to-fuchsia-500/10 border border-indigo-500/20 text-xs font-semibold text-indigo-400">
            <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
            Open Source · Self-Hosted · MIT License
          </div>

          {/* Headline */}
          <h1 className="text-5xl md:text-7xl font-black tracking-tighter leading-[1.05]">
            Qwen AI,{" "}
            <span className="bg-gradient-to-r from-indigo-400 via-violet-400 to-fuchsia-400 bg-clip-text text-transparent">
              any format
            </span>
          </h1>

          <p className="text-muted-foreground text-lg md:text-xl leading-relaxed max-w-2xl mx-auto">
            A self-hosted API gateway that exposes Qwen through OpenAI, Anthropic, and Gemini endpoints.
            Account pooling, auto-registration, streaming, tool calls, and creative generation — one container.
          </p>

          {/* CTA buttons */}
          <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
            <Link href="/docs" className="h-11 px-6 rounded-xl bg-gradient-to-r from-indigo-500 to-fuchsia-500 text-white font-bold text-sm flex items-center gap-2 hover:shadow-lg hover:shadow-indigo-500/25 transition-all hover:-translate-y-0.5">
              <BookOpen className="w-4 h-4" /> Documentation
            </Link>
            <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer"
              className="h-11 px-6 rounded-xl border border-border/50 text-foreground font-bold text-sm flex items-center gap-2 hover:bg-muted/20 transition-all hover:-translate-y-0.5">
              <GitBranch className="w-4 h-4" /> View on GitHub
            </a>
            <Link href="/login" className="h-11 px-6 rounded-xl border border-border/50 text-foreground font-bold text-sm flex items-center gap-2 hover:bg-muted/20 transition-all hover:-translate-y-0.5">
              <Play className="w-4 h-4" /> Live Demo
            </Link>
          </div>
        </div>

        {/* Terminal block */}
        <div className="relative z-10 mt-16 max-w-2xl mx-auto">
          <div className="rounded-2xl bg-[#0a0b10] border border-white/[0.06] overflow-hidden shadow-[0_20px_80px_-20px_rgba(99,102,241,0.15)]">
            <div className="flex items-center justify-between px-5 py-3 border-b border-white/[0.04]">
              <div className="flex items-center gap-2">
                <div className="flex gap-1.5">
                  <div className="w-2.5 h-2.5 rounded-full bg-white/10" />
                  <div className="w-2.5 h-2.5 rounded-full bg-white/10" />
                  <div className="w-2.5 h-2.5 rounded-full bg-white/10" />
                </div>
                <div className="ml-3 flex items-center gap-1.5">
                  <Terminal className="w-3 h-3 text-white/20" />
                  <span className="text-[11px] text-white/30 font-medium">terminal</span>
                </div>
              </div>
              <CopyButton text="docker run -d -p 7860:7860 -e ADMIN_KEY=changeme --shm-size=256m ghcr.io/hirotomasato/qwenpi:latest" />
            </div>
            <div className="p-5 font-mono text-[13px] leading-[1.8]">
              <div className="text-white/50">
                <span className="text-emerald-400">❯</span> docker run -d \
              </div>
              <div className="text-white/70 pl-6">
                -p <span className="text-amber-300">7860:7860</span> \
              </div>
              <div className="text-white/70 pl-6">
                -e ADMIN_KEY=<span className="text-amber-300">changeme</span> \
              </div>
              <div className="text-white/70 pl-6">
                --shm-size=<span className="text-amber-300">256m</span> \
              </div>
              <div className="text-white/70 pl-6">
                ghcr.io/hirotomasato/<span className="text-indigo-300">qwenpi</span>:latest
              </div>
              <div className="mt-3 text-emerald-400/70 text-[11px]">
                ✓ Container started · Gateway ready at :7860
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ─── Stats bar ─── */}
      <section className="py-10 border-y border-border/10">
        <div className="max-w-5xl mx-auto px-6 md:px-10 grid grid-cols-2 md:grid-cols-4 gap-6">
          {STATS.map(s => (
            <div key={s.label} className="text-center">
              <div className="text-3xl md:text-4xl font-black text-foreground">
                <Counter end={s.value} suffix={s.suffix || ""} />
              </div>
              <div className="text-xs text-muted-foreground mt-1 font-medium">{s.label}</div>
            </div>
          ))}
        </div>
      </section>

      {/* ─── Protocols ─── */}
      <section className="py-24 px-6 md:px-10 max-w-6xl mx-auto">
        <div className="text-center mb-14">
          <p className="text-xs font-bold text-indigo-400 uppercase tracking-widest mb-3">Compatibility</p>
          <h2 className="text-3xl md:text-4xl font-black tracking-tight">One gateway, every format</h2>
          <p className="text-muted-foreground text-sm mt-3 max-w-lg mx-auto">Use whichever API format your client expects. Qwenpi translates on the fly.</p>
        </div>

        <div className="grid md:grid-cols-3 gap-5">
          {PROTOCOLS.map(p => (
            <div key={p.name} className="glass-card rounded-2xl p-6 space-y-4 hover:border-indigo-500/20 transition-all group">
              <div className="flex items-center gap-3">
                <div className={`w-9 h-9 rounded-xl bg-gradient-to-br ${p.color} flex items-center justify-center shadow-lg`}>
                  <p.icon className="w-4 h-4 text-white" />
                </div>
                <span className="font-black text-sm">{p.name}</span>
              </div>
              <div className="space-y-2">
                {p.endpoints.map(ep => (
                  <div key={ep} className="flex items-center gap-2 text-[11px] font-mono text-muted-foreground group-hover:text-foreground/70 transition-colors">
                    <ChevronRight className="w-3 h-3 text-indigo-500/50 shrink-0" />
                    <span className="truncate">{ep}</span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* ─── Features ─── */}
      <section className="py-24 px-6 md:px-10 max-w-6xl mx-auto">
        <div className="text-center mb-14">
          <p className="text-xs font-bold text-indigo-400 uppercase tracking-widest mb-3">Capabilities</p>
          <h2 className="text-3xl md:text-4xl font-black tracking-tight">Everything you need at scale</h2>
          <p className="text-muted-foreground text-sm mt-3 max-w-lg mx-auto">From account management to creative generation, built for production workloads.</p>
        </div>

        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-4">
          {FEATURES.map(f => (
            <div key={f.title} className="glass-card rounded-2xl p-5 space-y-3 group hover:border-indigo-500/15 transition-all hover:-translate-y-0.5">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500/10 to-fuchsia-500/10 border border-indigo-500/15 flex items-center justify-center group-hover:from-indigo-500/20 group-hover:to-fuchsia-500/15 transition-colors">
                <f.icon className="w-[18px] h-[18px] text-indigo-400" />
              </div>
              <h3 className="text-[13px] font-bold text-foreground">{f.title}</h3>
              <p className="text-[12px] text-muted-foreground leading-relaxed">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ─── Code Example ─── */}
      <section className="py-24 px-6 md:px-10 max-w-6xl mx-auto">
        <div className="grid lg:grid-cols-2 gap-10 items-center">
          <div className="space-y-5">
            <p className="text-xs font-bold text-indigo-400 uppercase tracking-widest">Integration</p>
            <h2 className="text-3xl md:text-4xl font-black tracking-tight leading-tight">Drop-in replacement<br/>for any OpenAI client</h2>
            <p className="text-muted-foreground text-sm leading-relaxed">
              Change the base URL and you're done. Works with the official OpenAI SDK, LangChain, LlamaIndex, or any HTTP client.
              All 39+ model aliases are mapped automatically.
            </p>
            <div className="flex flex-wrap gap-2 pt-2">
              {CLIENTS.map(c => (
                <span key={c} className="px-2.5 py-1 rounded-lg bg-muted/30 border border-border/30 text-[10px] font-bold text-muted-foreground">{c}</span>
              ))}
            </div>
          </div>

          <div className="rounded-2xl bg-[#0a0b10] border border-white/[0.06] overflow-hidden shadow-[0_20px_60px_-20px_rgba(99,102,241,0.12)]">
            <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.04]">
              <div className="flex items-center gap-2">
                <Code2 className="w-3.5 h-3.5 text-white/20" />
                <span className="text-[10px] text-white/30 font-medium">python</span>
              </div>
              <CopyButton text={`from openai import OpenAI\n\nclient = OpenAI(\n    base_url="http://localhost:7860/v1",\n    api_key="your-key"\n)\n\nfor chunk in client.chat.completions.create(\n    model="gpt-4o",\n    messages=[{"role": "user", "content": "Hello!"}],\n    stream=True\n):\n    print(chunk.choices[0].delta.content or "", end="")`} />
            </div>
            <pre className="p-5 font-mono text-[12px] leading-[1.9] overflow-x-auto">
              <code>
                <span className="text-violet-400">from</span> <span className="text-white/90">openai</span> <span className="text-violet-400">import</span> <span className="text-white/90">OpenAI</span>{"\n\n"}
                <span className="text-white/90">client</span> <span className="text-white/30">=</span> <span className="text-white/90">OpenAI</span><span className="text-white/30">(</span>{"\n"}
                <span className="text-white/30">    base_url=</span><span className="text-emerald-400">"http://localhost:7860/v1"</span><span className="text-white/30">,</span>{"\n"}
                <span className="text-white/30">    api_key=</span><span className="text-emerald-400">"your-key"</span>{"\n"}
                <span className="text-white/30">)</span>{"\n\n"}
                <span className="text-violet-400">for</span> <span className="text-white/90">chunk</span> <span className="text-violet-400">in</span> <span className="text-white/90">client.chat.completions.create</span><span className="text-white/30">(</span>{"\n"}
                <span className="text-white/30">    model=</span><span className="text-emerald-400">"gpt-4o"</span><span className="text-white/30">,</span>{"\n"}
                <span className="text-white/30">    messages=[</span><span className="text-white/30">{`{`}</span><span className="text-amber-300">"role"</span><span className="text-white/30">: </span><span className="text-emerald-400">"user"</span><span className="text-white/30">, </span><span className="text-amber-300">"content"</span><span className="text-white/30">: </span><span className="text-emerald-400">"Hello!"</span><span className="text-white/30">{`}`}],</span>{"\n"}
                <span className="text-white/30">    stream=</span><span className="text-amber-300">True</span>{"\n"}
                <span className="text-white/30">):</span>{"\n"}
                <span className="text-white/90">    print</span><span className="text-white/30">(chunk.choices[0].delta.content </span><span className="text-violet-400">or</span><span className="text-white/30"> </span><span className="text-emerald-400">""</span><span className="text-white/30">, end=</span><span className="text-emerald-400">""</span><span className="text-white/30">)</span>
              </code>
            </pre>
          </div>
        </div>
      </section>

      {/* ─── CTA ─── */}
      <section className="py-24 px-6 md:px-10 max-w-4xl mx-auto text-center">
        <div className="glass-card rounded-3xl p-12 md:p-16 space-y-6 relative overflow-hidden">
          {/* Glow */}
          <div className="absolute inset-0 pointer-events-none">
            <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[400px] h-[200px] bg-indigo-500/[0.06] rounded-full blur-[80px]" />
          </div>
          <div className="relative z-10 space-y-6">
            <h2 className="text-3xl md:text-4xl font-black tracking-tight">Start in under a minute</h2>
            <p className="text-muted-foreground text-sm max-w-md mx-auto">Pull the Docker image, set your admin key, and you have a fully functional Qwen API gateway.</p>
            <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
              <Link href="/docs" className="h-11 px-6 rounded-xl bg-gradient-to-r from-indigo-500 to-fuchsia-500 text-white font-bold text-sm flex items-center gap-2 hover:shadow-lg hover:shadow-indigo-500/25 transition-all hover:-translate-y-0.5">
                <BookOpen className="w-4 h-4" /> Read the Docs
              </Link>
              <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer"
                className="h-11 px-6 rounded-xl border border-border/50 text-foreground font-bold text-sm flex items-center gap-2 hover:bg-muted/20 transition-all hover:-translate-y-0.5">
                <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
                Star on GitHub
              </a>
            </div>
          </div>
        </div>
      </section>

      {/* ─── Footer ─── */}
      <footer className="border-t border-border/10 py-10 px-6 md:px-10">
        <div className="max-w-6xl mx-auto flex flex-col md:flex-row items-center justify-between gap-6">
          <div className="flex items-center gap-2.5 text-sm text-muted-foreground">
            <div className="h-7 w-7 rounded-lg flex items-center justify-center bg-gradient-to-br from-indigo-500 to-fuchsia-500">
              <QCatIcon className="h-4 w-4" />
            </div>
            <span className="font-bold text-foreground">Qwenpi</span>
            <span className="text-muted-foreground/40 text-xs">v2.0.0</span>
          </div>
          <div className="flex items-center gap-6 text-xs text-muted-foreground">
            <Link href="/docs" className="hover:text-foreground transition-colors">Documentation</Link>
            <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer" className="hover:text-foreground transition-colors flex items-center gap-1.5">
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
              GitHub
            </a>
            <Link href="/login" className="hover:text-foreground transition-colors">Dashboard</Link>
            <span className="text-muted-foreground/30">MIT License</span>
          </div>
        </div>
      </footer>
    </div>
  )
}
