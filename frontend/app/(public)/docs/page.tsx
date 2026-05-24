'use client';

import Link from "next/link"
import { ArrowLeft, Copy, Check, Terminal, Key, Server, Zap, Image, Film, MessageSquare, Globe } from "lucide-react"
import QCatIcon from "@/components/QCatIcon"
import { useState } from "react"

function CopyBtn({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 2000) }}
      className="p-1 rounded hover:bg-white/10 text-white/30 hover:text-white/70 transition-colors" title="Copy">
      {copied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
    </button>
  )
}

function CodeBlock({ title, code, lang = "bash" }: { title?: string; code: string; lang?: string }) {
  return (
    <div className="rounded-xl bg-[#0d0e14] border border-white/8 overflow-hidden my-4">
      {title && (
        <div className="flex items-center justify-between px-4 py-2 border-b border-white/5">
          <span className="text-[10px] text-white/40 font-medium">{title}</span>
          <CopyBtn text={code} />
        </div>
      )}
      <pre className="p-4 font-mono text-[12px] text-white/80 leading-relaxed overflow-x-auto whitespace-pre-wrap">{code}</pre>
    </div>
  )
}

function Section({ id, icon: Icon, title, children }: { id: string; icon: any; title: string; children: React.ReactNode }) {
  return (
    <section id={id} className="scroll-mt-20 py-10 border-b border-border/20 last:border-0">
      <div className="flex items-center gap-2.5 mb-5">
        <div className="w-7 h-7 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center">
          <Icon className="w-3.5 h-3.5 text-indigo-400" />
        </div>
        <h2 className="text-xl font-black tracking-tight">{title}</h2>
      </div>
      <div className="text-sm text-muted-foreground leading-relaxed space-y-4">{children}</div>
    </section>
  )
}

const NAV = [
  { id: "auth", label: "Authentication", icon: Key },
  { id: "chat", label: "Chat Completions", icon: MessageSquare },
  { id: "images", label: "Image Generation", icon: Image },
  { id: "videos", label: "Video Generation", icon: Film },
  { id: "anthropic", label: "Anthropic", icon: Globe },
  { id: "gemini", label: "Gemini", icon: Globe },
  { id: "models", label: "Models", icon: Server },
  { id: "errors", label: "Errors", icon: Zap },
]

export default function DocsPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Navbar */}
      <nav className="fixed top-0 inset-x-0 z-50 h-14 flex items-center justify-between px-6 md:px-10 border-b border-border/20 bg-background/80 backdrop-blur-xl">
        <div className="flex items-center gap-4">
          <Link href="/" className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors">
            <ArrowLeft className="w-4 h-4" />
            <span className="text-xs font-medium hidden sm:inline">Back</span>
          </Link>
          <div className="h-5 w-px bg-border/40" />
          <div className="flex items-center gap-2">
            <div className="h-7 w-7 rounded-lg flex items-center justify-center bg-gradient-to-br from-indigo-500 to-fuchsia-500">
              <QCatIcon className="h-4 w-4" />
            </div>
            <span className="font-black text-sm">Qwenpi Docs</span>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer" className="text-muted-foreground hover:text-foreground transition-colors">
            <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" /></svg>
          </a>
          <Link href="/login" className="h-8 px-3 rounded-lg bg-foreground text-background text-xs font-bold flex items-center gap-1.5 hover:opacity-90 transition-opacity">
            Dashboard
          </Link>
        </div>
      </nav>

      <div className="flex pt-14">
        {/* Sidebar nav */}
        <aside className="hidden lg:block w-56 shrink-0 sticky top-14 h-[calc(100vh-56px)] overflow-y-auto border-r border-border/20 py-6 px-4">
          <p className="text-[10px] font-bold text-muted-foreground/50 uppercase tracking-widest mb-3 px-2">API Reference</p>
          <nav className="space-y-0.5">
            {NAV.map(n => (
              <a key={n.id} href={`#${n.id}`} className="flex items-center gap-2 px-2 py-1.5 rounded-lg text-xs font-medium text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors">
                <n.icon className="w-3.5 h-3.5 opacity-50" />
                {n.label}
              </a>
            ))}
          </nav>
        </aside>

        {/* Content */}
        <main className="flex-1 max-w-3xl mx-auto px-6 md:px-10 py-10">
          <div className="mb-10">
            <h1 className="text-3xl font-black tracking-tight">API Reference</h1>
            <p className="text-muted-foreground text-sm mt-2">
              Base URL: <code className="px-2 py-0.5 rounded bg-muted/50 text-foreground font-mono text-xs">http://localhost:7860</code>
            </p>
          </div>

          <Section id="auth" icon={Key} title="Authentication">
            <p>All API requests require a Bearer token in the <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">Authorization</code> header, or an <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">x-api-key</code> header for Anthropic format.</p>
            <CodeBlock title="Header format" code={`Authorization: Bearer your-api-key\n\n# Or for Anthropic:\nx-api-key: your-api-key`} />
            <p>The default admin key is <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">123456</code>. You can generate additional downstream keys from the admin dashboard.</p>
          </Section>

          <Section id="chat" icon={MessageSquare} title="Chat Completions">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1/chat/completions</code></p>
            <p>Fully compatible with the OpenAI Chat Completions API. Supports streaming, tool calling, and thinking modes.</p>
            <CodeBlock title="Request" code={`curl http://localhost:7860/v1/chat/completions \\\n  -H "Authorization: Bearer your-key" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "gpt-4o",\n    "messages": [{"role": "user", "content": "Hello!"}],\n    "stream": true\n  }'`} />
            <p><strong>Thinking modes:</strong> Append <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">-thinking</code> for deep reasoning or <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">-nothinking</code> for fast responses. Example: <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">qwen3.6-plus-thinking</code></p>
          </Section>

          <Section id="images" icon={Image} title="Image Generation">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1/images/generations</code></p>
            <p>Compatible with the OpenAI DALL-E endpoint. Supports aspect ratio selection.</p>
            <CodeBlock title="Request" code={`curl http://localhost:7860/v1/images/generations \\\n  -H "Authorization: Bearer your-key" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "prompt": "A sunset over mountains",\n    "aspect_ratio": "16:9",\n    "n": 1\n  }'`} />
            <p><strong>Supported aspect ratios:</strong> <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">1:1</code>, <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">4:3</code>, <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">3:4</code>, <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">16:9</code>, <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">9:16</code></p>
            <p>Also accepts the OpenAI <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">size</code> parameter (e.g. <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">1024x576</code> for 16:9).</p>
          </Section>

          <Section id="videos" icon={Film} title="Video Generation (Beta)">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1/videos/generations</code></p>
            <p>Experimental text-to-video endpoint. Generation typically takes 30s–3min. Availability depends on upstream account capabilities.</p>
            <CodeBlock title="Request" code={`curl http://localhost:7860/v1/videos/generations \\\n  -H "Authorization: Bearer your-key" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "prompt": "A cat walking in snow",\n    "aspect_ratio": "16:9",\n    "n": 1\n  }'`} />
            <p>Returns a JSON response with <code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">data[].url</code> containing the video URL.</p>
          </Section>

          <Section id="anthropic" icon={Globe} title="Anthropic Messages">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1/messages</code></p>
            <p>Compatible with the Anthropic Messages API, including tool_use blocks.</p>
            <CodeBlock title="Request" code={`curl http://localhost:7860/v1/messages \\\n  -H "x-api-key: your-key" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "claude-3-5-sonnet-latest",\n    "messages": [{"role": "user", "content": "Hello"}],\n    "max_tokens": 1024\n  }'`} />
          </Section>

          <Section id="gemini" icon={Globe} title="Gemini">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1beta/models/{'{model}'}:generateContent</code></p>
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">POST /v1beta/models/{'{model}'}:streamGenerateContent</code></p>
            <p>Compatible with the Google Gemini API format. Pass the API key as a query parameter or Bearer token.</p>
            <CodeBlock title="Request" code={`curl "http://localhost:7860/v1beta/models/gemini-2.5-pro:generateContent?key=your-key" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "contents": [{"parts": [{"text": "Hello"}]}]\n  }'`} />
          </Section>

          <Section id="models" icon={Server} title="Models">
            <p><code className="px-1.5 py-0.5 rounded bg-muted/50 text-xs font-mono">GET /v1/models</code></p>
            <p>Returns the list of all available models (native + aliases). No authentication required.</p>
            <div className="mt-4">
              <p className="font-bold text-foreground text-xs mb-2">Native models:</p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-1">
                {["qwen3.6-plus", "qwen3.6-plus-thinking", "qwen3.6-plus-nothinking", "qwen3.6-max-preview", "qwen3.6-max-preview-thinking", "qwen3.6-max-preview-nothinking", "qwen3.6-27b", "qwen3.6-27b-thinking", "qwen3.6-27b-nothinking"].map(m => (
                  <code key={m} className="px-2 py-1 rounded bg-muted/30 text-[11px] font-mono text-foreground/80">{m}</code>
                ))}
              </div>
            </div>
            <div className="mt-4">
              <p className="font-bold text-foreground text-xs mb-2">Popular aliases (39+ built-in):</p>
              <p className="text-xs text-muted-foreground">gpt-4o, gpt-4o-mini, o1, o3, claude-3-5-sonnet-latest, claude-sonnet-4-20250514, gemini-2.5-pro, deepseek-chat, and more. All automatically routed to the appropriate Qwen model.</p>
            </div>
          </Section>

          <Section id="errors" icon={Zap} title="Error Handling">
            <p>The API returns standard HTTP status codes:</p>
            <div className="mt-3 space-y-2">
              {[
                { code: "200", desc: "Success" },
                { code: "400", desc: "Bad request (missing required fields)" },
                { code: "401", desc: "Unauthorized (invalid or missing API key)" },
                { code: "429", desc: "Rate limited (all accounts exhausted)" },
                { code: "500", desc: "Internal error (generation failed after retries)" },
              ].map(e => (
                <div key={e.code} className="flex items-center gap-3">
                  <code className={`px-2 py-0.5 rounded text-[11px] font-mono font-bold ${e.code === "200" ? "bg-emerald-500/10 text-emerald-500" : e.code.startsWith("4") ? "bg-amber-500/10 text-amber-500" : "bg-rose-500/10 text-rose-500"}`}>{e.code}</code>
                  <span className="text-xs">{e.desc}</span>
                </div>
              ))}
            </div>
            <CodeBlock title="Error response format" code={`{\n  "detail": "Image generation failed: all accounts have hit the daily usage limit"\n}`} />
          </Section>
        </main>
      </div>
    </div>
  )
}
