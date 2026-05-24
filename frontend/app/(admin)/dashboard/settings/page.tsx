'use client'

import { useState, useEffect } from "react"
import { useRouter } from "next/navigation"
import { KeyRound, Code, Mail, Server, Cpu, TriangleAlert, Globe, Users, Zap } from "lucide-react"
import { toast } from "sonner"

function Card({ title, icon: Icon, color = "indigo", children }: {
  title: string; icon: any; color?: string; children: React.ReactNode
}) {
  const palette: Record<string, { bg: string; border: string; text: string }> = {
    indigo: { bg: "bg-indigo-500/10", border: "border-indigo-500/20", text: "text-indigo-500" },
    amber: { bg: "bg-amber-500/10", border: "border-amber-500/20", text: "text-amber-500" },
    cyan: { bg: "bg-cyan-500/10", border: "border-cyan-500/20", text: "text-cyan-500" },
    rose: { bg: "bg-rose-500/10", border: "border-rose-500/20", text: "text-rose-500" },
    violet: { bg: "bg-violet-500/10", border: "border-violet-500/20", text: "text-violet-500" },
    emerald: { bg: "bg-emerald-500/10", border: "border-emerald-500/20", text: "text-emerald-500" },
  }
  const c = palette[color] ?? palette.indigo
  return (
    <div className={`glass-card rounded-3xl p-8 space-y-6 border ${c.border}`}>
      <div className="flex items-center gap-3">
        <div className={`w-10 h-10 rounded-xl ${c.bg} flex items-center justify-center border ${c.border} shrink-0`}>
          <Icon className={`w-5 h-5 ${c.text}`} />
        </div>
        <h3 className="text-base font-black tracking-tight text-foreground">{title}</h3>
      </div>
      {children}
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <label className="text-[11px] font-medium text-muted-foreground ml-1">{label}</label>
      {children}
    </div>
  )
}

const inputCls = "w-full h-12 bg-muted/20 border border-border/40 rounded-xl px-4 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/30 transition-all"
const btnIndigo = "w-full h-11 bg-indigo-500 text-white font-semibold rounded-xl text-sm shadow-lg shadow-indigo-500/20 hover:opacity-90 transition-all"

const API_ENDPOINTS = [
  { badge: "OpenAI", color: "bg-indigo-500", path: "/v1/chat/completions", desc: "Chat completion, tool calls, streaming" },
  { badge: "OpenAI", color: "bg-indigo-500", path: "/v1/responses", desc: "New Responses API format" },
  { badge: "OpenAI", color: "bg-indigo-500", path: "/v1/images/generations", desc: "Image generation" },
  { badge: "OpenAI", color: "bg-indigo-500", path: "/v1/embeddings", desc: "Embedding vectors" },
  { badge: "OpenAI", color: "bg-indigo-500", path: "/v1/models", desc: "Model list" },
  { badge: "Claude", color: "bg-orange-500", path: "/v1/messages", desc: "Anthropic compatibility layer" },
  { badge: "Gemini", color: "bg-emerald-500", path: "/v1beta/models/{model}:generateContent", desc: "Google Gemini passthrough" },
]

export default function SettingsPage() {
  const router = useRouter()
  const [sessionKey, setSessionKey] = useState("")
  const [maxInflight, setMaxInflight] = useState(4)
  const [modelAliases, setModelAliases] = useState("")
  const [moemailDomain, setMoemailDomain] = useState("")
  const [moemailKey, setMoemailKey] = useState("")
  const [tempmailDomain, setTempmailDomain] = useState("")
  const [tempmailKey, setTempmailKey] = useState("")
  const [engineMode, setEngineMode] = useState("hybrid")
  const [mailTab, setMailTab] = useState<'moemail' | 'tempmail'>('moemail')
  const [proxyEnabled, setProxyEnabled] = useState(false)
  const [proxyUrl, setProxyUrl] = useState("")
  const [proxyUsername, setProxyUsername] = useState("")
  const [proxyPassword, setProxyPassword] = useState("")
  const [proxyTesting, setProxyTesting] = useState(false)
  const [proxyTestResult, setProxyTestResult] = useState<{
    ok: boolean; direct_ip: string; proxy_ip: string; error?: string;
  } | null>(null)
  const [poolProviders, setPoolProviders] = useState<Array<{ name: string; total: number }>>([])
  const [rtkEnabled, setRtkEnabled] = useState(true)
  const [cavemanEnabled, setCavemanEnabled] = useState(false)
  const [cavemanLevel, setCavemanLevel] = useState<'lite' | 'full' | 'ultra'>('full')

  const fetchSettings = () => {
    fetch("/api/admin/settings")
      .then(res => { if (!res.ok) throw new Error(); return res.json() })
      .then(d => {
        setMaxInflight(d.max_inflight_per_account || 4)
        setModelAliases(JSON.stringify(d.model_aliases || {}, null, 2))
        setMoemailDomain(d.moemail_domain || "")
        setMoemailKey(d.moemail_key || "")
        setTempmailDomain(d.tempmail_domain || "")
        setTempmailKey(d.tempmail_key || "")
        setEngineMode(d.engine_mode || "hybrid")
        setProxyEnabled(!!d.proxy_enabled)
        setProxyUrl(d.proxy_url || "")
        setProxyUsername(d.proxy_username || "")
        setProxyPassword(d.proxy_password || "")
        setRtkEnabled(d.rtk_enabled !== false)
        setCavemanEnabled(!!d.caveman_enabled)
        const lvl = (d.caveman_level || 'full') as 'lite' | 'full' | 'ultra'
        setCavemanLevel(['lite', 'full', 'ultra'].includes(lvl) ? lvl : 'full')
      })
      .catch(() => toast.error("Failed to fetch config — please check the key"))
  }

  useEffect(() => {
    setSessionKey(localStorage.getItem("qwenpi_key") || "")
    fetchSettings()
    fetch("/api/admin/providers")
      .then(r => r.json())
      .then(d => setPoolProviders(d.providers || []))
      .catch(() => {})
  }, [])

  const saveSetting = (key: string, value: any, label = key) => {
    const id = toast.loading("Saving...")
    fetch("/api/admin/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ [key]: value }),
    })
      .then(res => res.ok ? toast.success(`${label} updated`, { id }) : toast.error("Update failed", { id }))
      .catch(() => toast.error("Request error", { id }))
  }

  const handleSaveAliases = () => {
    try { saveSetting("model_aliases", JSON.parse(modelAliases), "Model routing") }
    catch { toast.error("Invalid JSON format") }
  }

  const isMoe = mailTab === 'moemail'
  const [backendUrl, setBackendUrl] = useState("")
  useEffect(() => { setBackendUrl(window.location.origin) }, [])

  return (
    <div className="animate-fade-in-up max-w-[1400px] mx-auto pb-20 space-y-10">

      <div className="flex items-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-indigo-500/10 flex items-center justify-center border border-indigo-500/20">
          <KeyRound className="w-5 h-5 text-indigo-500" />
        </div>
        <h2 className="text-3xl font-black tracking-tighter text-foreground">Gateway Settings</h2>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-12 gap-8 items-start">

        <div className="xl:col-span-7 space-y-6">

          <Card title="Admin key" icon={KeyRound}>
            <div className="flex gap-2">
              <input type="password" value={sessionKey} onChange={e => setSessionKey(e.target.value)}
                placeholder="Admin Key (login password)" className={`${inputCls} font-mono flex-1`} />
              <button
                onClick={() => {
                  if (!sessionKey.trim()) { toast.error("Key cannot be empty"); return; }
                  fetch("/api/admin/settings", {
                    method: "PUT",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ admin_key: sessionKey.trim() })
                  }).then(res => {
                    if (res.ok) {
                      toast.success("Admin key updated. Please log in again with the new key.");
                      localStorage.removeItem("qwenpi_key");
                      setTimeout(() => router.push("/login"), 1500);
                    } else {
                      toast.error("Failed to update key. Please check that your current key is correct.");
                    }
                  }).catch(() => {
                    toast.error("Network connection error");
                  });
                }}
                className="h-12 px-5 bg-foreground text-background font-semibold rounded-xl text-sm hover:opacity-90 transition-all whitespace-nowrap">
                Save
              </button>
              <button
                onClick={() => { localStorage.removeItem("qwenpi_key"); setSessionKey(""); toast.info("Credentials cleared") }}
                className="h-12 px-4 bg-muted/30 text-muted-foreground font-black rounded-xl text-[11px] hover:bg-rose-500/10 hover:text-rose-500 transition-all whitespace-nowrap">
                Clear
              </button>
            </div>
            <p className="text-[10px] text-muted-foreground">
              Takes effect immediately and the new key persists across restarts.
            </p>
          </Card>

          <Card title="Bulk Account Pool" icon={Users} color="cyan">
            <p className="text-xs text-muted-foreground leading-relaxed">
              Quickly open the provider page with the relevant batch / import modal already open.
            </p>
            {poolProviders.length === 0 ? (
              <p className="text-xs text-muted-foreground">No providers detected.</p>
            ) : (
              <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
                {poolProviders.map(p => (
                  <button
                    key={p.name}
                    onClick={() => {
                      router.push(`/dashboard/providers?bulkAdd=${encodeURIComponent(p.name)}`)
                    }}
                    className="h-11 px-3 rounded-xl bg-muted/20 border border-border/40 hover:bg-cyan-500/10 hover:border-cyan-500/30 hover:text-cyan-500 transition-all flex items-center gap-2 text-xs font-black"
                  >
                    <div className="h-7 w-7 rounded-lg bg-gradient-to-br from-cyan-500 to-indigo-500 flex items-center justify-center text-white text-[11px] font-black">
                      {p.name[0]?.toUpperCase()}
                    </div>
                    <span className="capitalize truncate">{p.name}</span>
                    <span className="ml-auto text-[10px] text-muted-foreground tabular-nums">{p.total}</span>
                  </button>
                ))}
              </div>
            )}
          </Card>

          <Card title="Self-hosted Mail Service" icon={Mail} color="indigo">
            <div className="flex gap-1 p-1 bg-muted/30 rounded-xl border border-border/40">
              {(['moemail', 'tempmail'] as const).map(tab => (
                <button key={tab} onClick={() => setMailTab(tab)}
                  className={`flex-1 h-9 rounded-lg text-xs font-black tracking-wide transition-all ${mailTab === tab ? 'bg-background shadow text-foreground' : 'text-muted-foreground hover:text-foreground'
                    }`}>
                  {tab === 'moemail' ? 'MoeMail' : 'TempMail (CF Workers)'}
                </button>
              ))}
            </div>

            <p className="text-xs text-muted-foreground leading-relaxed">
              {isMoe ? (
                <>For self-hosted <a href="https://github.com/beilunyang/moemail" target="_blank" rel="noreferrer" className="text-indigo-500 underline">MoeMail</a> mail. Provide the service domain (including http/https) and the API key.</>
              ) : (
                <>For Cloudflare Workers deployments of <a href="https://temp-mail-docs.awsl.uk" target="_blank" rel="noreferrer" className="text-indigo-500 underline">TempMail</a>. Provide the Workers domain and the admin password (x-admin-auth).</>
              )}
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Field label={isMoe ? 'Service domain' : 'Workers domain'}>
                <input type="text"
                  value={isMoe ? moemailDomain : tempmailDomain}
                  onChange={e => isMoe ? setMoemailDomain(e.target.value) : setTempmailDomain(e.target.value)}
                  className={inputCls}
                  placeholder={isMoe ? 'https://api.moemail.app' : 'https://xxxx.xxxx.workers.dev'} />
              </Field>
              <Field label={isMoe ? 'API key' : 'Admin password'}>
                <input type="password"
                  value={isMoe ? moemailKey : tempmailKey}
                  onChange={e => isMoe ? setMoemailKey(e.target.value) : setTempmailKey(e.target.value)}
                  className={`${inputCls} font-mono`}
                  placeholder={isMoe ? 'x-api-key' : 'x-admin-auth'} />
              </Field>
            </div>

            <button
              onClick={() => {
                if (isMoe) { saveSetting('moemail_domain', moemailDomain, 'MoeMail domain'); saveSetting('moemail_key', moemailKey, 'MoeMail key') }
                else { saveSetting('tempmail_domain', tempmailDomain, 'TempMail domain'); saveSetting('tempmail_key', tempmailKey, 'TempMail key') }
              }}
              className={btnIndigo}>
              Save {isMoe ? 'MoeMail' : 'TempMail'} config
            </button>
          </Card>

          <Card title="Registration Proxy Pool" icon={Globe} color="violet">
            <p className="text-xs text-muted-foreground leading-relaxed">
              When enabled, browser registrations are sent through the proxy IP, bypassing Alibaba WAF rate limits.
              Supports the <code className="text-violet-400 font-mono">http://</code>, <code className="text-violet-400 font-mono">https://</code>, and <code className="text-violet-400 font-mono">socks5://</code> protocols.
            </p>

            <div className="flex items-center justify-between p-3 rounded-xl bg-muted/30 border border-border/40">
              <div>
                <p className="text-sm font-black">Enable proxy</p>
                <p className="text-[10px] text-muted-foreground">Hot-reload — applies on the next registration</p>
              </div>
              <button
                onClick={() => {
                  const next = !proxyEnabled
                  setProxyEnabled(next)
                  saveSetting('proxy_enabled', next, 'Proxy toggle')
                }}
                className={`relative w-11 h-6 rounded-full transition-colors ${proxyEnabled ? 'bg-violet-500' : 'bg-muted/50 border border-border/60'
                  }`}>
                <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${proxyEnabled ? 'translate-x-5' : ''
                  }`} />
              </button>
            </div>

            <div>
              <p className="text-[10px] font-black text-muted-foreground mb-2">Quick-fill provider templates</p>
              <div className="grid grid-cols-2 gap-2">
                {[
                  { name: 'BrightData', tpl: 'http://brd.superproxy.io:22225' },
                  { name: 'Oxylabs', tpl: 'http://pr.oxylabs.io:7777' },
                  { name: 'Smartproxy', tpl: 'http://gate.smartproxy.com:7000' },
                  { name: 'ProxyScrape', tpl: 'http://proxy.proxyscrape.com:7777' },
                ].map(p => (
                  <button key={p.name}
                    onClick={() => setProxyUrl(p.tpl)}
                    className="h-8 text-[11px] font-black bg-muted/20 border border-border/40 rounded-lg hover:bg-violet-500/10 hover:border-violet-500/30 hover:text-violet-500 transition-all">
                    {p.name}
                  </button>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4">
              <Field label="Proxy URL">
                <input type="text" value={proxyUrl} onChange={e => setProxyUrl(e.target.value)}
                  placeholder="http://host:port or socks5://host:port"
                  className={`${inputCls} font-mono`} />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Username (optional)">
                  <input type="text" value={proxyUsername} onChange={e => setProxyUsername(e.target.value)}
                    placeholder="username"
                    className={`${inputCls} font-mono`} />
                </Field>
                <Field label="Password (optional)">
                  <input type="password" value={proxyPassword} onChange={e => setProxyPassword(e.target.value)}
                    placeholder="password"
                    className={`${inputCls} font-mono`} />
                </Field>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <button
                onClick={() => {
                  const id = toast.loading("Saving...")
                  fetch("/api/admin/settings", {
                    method: "PUT",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                      proxy_enabled: proxyEnabled,
                      proxy_url: proxyUrl,
                      proxy_username: proxyUsername,
                      proxy_password: proxyPassword,
                    }),
                  })
                    .then(res => res.ok ? toast.success("Proxy configuration saved", { id }) : toast.error("SaveFailed", { id }))
                    .catch(() => toast.error("Request error", { id }))
                  setProxyTestResult(null)
                }}
                className="h-11 bg-violet-500 text-white font-semibold rounded-xl text-sm hover:opacity-90 transition-all">
                Save proxy configuration
              </button>
              <button
                disabled={proxyTesting}
                onClick={async () => {
                  setProxyTesting(true)
                  setProxyTestResult(null)
                  try {
                    const res = await fetch("/api/admin/proxy-test", {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({
                        proxy_url: proxyUrl,
                        proxy_username: proxyUsername,
                        proxy_password: proxyPassword,
                      }),
                    })
                    const d = await res.json()
                    setProxyTestResult(d)
                    if (d.ok) toast.success('✅ Proxy active, IP has been replaced')
                    else toast.error(d.error || 'Proxy test failed')
                  } catch (e: any) {
                    toast.error('Request error: ' + e.message)
                  } finally {
                    setProxyTesting(false)
                  }
                }}
                className="h-11 bg-muted/30 border border-violet-500/40 text-violet-500 font-semibold rounded-xl text-sm hover:bg-violet-500/10 transition-all disabled:opacity-50">
                {proxyTesting ? 'Testing...' : 'Test connectivity'}
              </button>
            </div>

            {proxyTestResult && (
              <div className={`p-4 rounded-xl border text-xs space-y-2.5 ${proxyTestResult.ok
                ? 'bg-green-500/10 border-green-500/30'
                : 'bg-rose-500/10 border-rose-500/30'
                }`}>
                <div className="flex items-center gap-2 font-bold text-sm">
                  <span>{proxyTestResult.ok ? '✅' : '⚠️'}</span>
                  <span className={proxyTestResult.ok ? 'text-green-500' : 'text-rose-500'}>
                    {proxyTestResult.ok ? 'Proxy active, IP has been replaced' : 'Proxy not active'}
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <p className="text-muted-foreground text-[11px] mb-1">Direct IP (server)</p>
                    <p className="text-foreground font-mono text-[13px]">{proxyTestResult.direct_ip || 'Failed to fetch'}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground text-[11px] mb-1">Browser IP (via proxy)</p>
                    <p className={`font-mono text-[13px] ${proxyTestResult.ok ? 'text-green-400' : 'text-rose-400'}`}>
                      {proxyTestResult.proxy_ip || 'Failed to fetch'}
                    </p>
                  </div>
                </div>
                {proxyTestResult.error && (
                  <p className="text-rose-400 text-[11px] break-all">Error: {proxyTestResult.error}</p>
                )}
              </div>
            )}
          </Card>

          <Card title="Model Name Routing" icon={Code} color="amber">
            <p className="text-xs text-muted-foreground leading-relaxed">
              Map downstream client model names to physical nodes — wildcards supported. Edit the JSON and click "Apply" for immediate effect.
            </p>
            <textarea rows={7} value={modelAliases} onChange={e => setModelAliases(e.target.value)}
              className="w-full bg-muted/20 border border-border/40 rounded-2xl p-5 text-[13px] font-mono text-foreground focus:outline-none focus:ring-2 focus:ring-amber-500/20 transition-all leading-relaxed resize-none" />
            <button onClick={handleSaveAliases}
              className="w-full h-11 bg-amber-500 text-black font-semibold rounded-xl text-sm hover:opacity-90 transition-all">
              Apply mapping rules
            </button>
          </Card>
        </div>

        <div className="xl:col-span-5 space-y-6 xl:sticky xl:top-10">

          <Card title="Endpoint Integration" icon={Server}>
            <div className="p-3 rounded-xl bg-muted/30 border border-border/40 flex items-center gap-2">
              <code className="text-indigo-500 font-mono text-sm font-bold flex-1 break-all">{backendUrl}</code>
              <button onClick={() => { navigator.clipboard.writeText(backendUrl); toast.success("Copied") }}
                className="shrink-0 text-xs text-muted-foreground/50 hover:text-indigo-500 transition-colors">📋</button>
            </div>

            <div className="space-y-1">
              <p className="text-[10px] font-black text-muted-foreground mb-2">Supported protocols and paths</p>
              {API_ENDPOINTS.map(e => (
                <div
                  key={e.path}
                  title={`Click to copy: ${backendUrl}${e.path}`}
                  onClick={() => { navigator.clipboard.writeText(`${backendUrl}${e.path}`); toast.success("Full address copied") }}
                  className="flex items-center gap-2.5 px-3 py-2 rounded-xl bg-muted/20 border border-border/30 hover:bg-indigo-500/5 hover:border-indigo-500/20 transition-all group cursor-pointer">
                  <span className={`shrink-0 text-[9px] font-black text-white px-1.5 py-0.5 rounded-md ${e.color}`}>{e.badge}</span>
                  <code className="text-[11px] font-mono text-foreground/80 flex-1 truncate">{e.path}</code>
                  <span className="text-[10px] text-muted-foreground shrink-0 opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap">{e.desc}</span>
                </div>
              ))}
            </div>
          </Card>

          <Card title="Concurrency & Engine Strategy" icon={Cpu} color="cyan">
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-black text-foreground">Max concurrent requests per account</span>
                <span className="text-sm font-black text-cyan-500 tabular-nums">{maxInflight}</span>
              </div>
              <p className="text-[10px] text-muted-foreground leading-relaxed">
                Controls how many concurrent API requests each Qwen account handles. 1 = strict queue (safest); 3–5 = moderate throughput; over 5 may trigger account throttling or bans.
              </p>
              <input type="range" min="1" max="10" value={maxInflight}
                onChange={e => setMaxInflight(parseInt(e.target.value))}
                className="w-full h-2 bg-muted/30 rounded-full appearance-none cursor-pointer accent-cyan-500 mt-1" />
              <div className="flex justify-between text-[9px] text-muted-foreground/60 font-black px-0.5">
                <span>1 Safest</span><span>5 Recommended</span><span>10 Aggressive</span>
              </div>
            </div>
            <button onClick={() => saveSetting("max_inflight_per_account", maxInflight, "Concurrency")}
              className="w-full h-10 bg-muted/30 border border-border/40 font-black text-[11px] rounded-xl hover:bg-muted/50 transition-all">
              Apply concurrency setting
            </button>
            <Field label="Engine mode">
              <select value={engineMode} onChange={e => { setEngineMode(e.target.value); saveSetting("engine_mode", e.target.value, "Engine mode") }}
                className="w-full h-12 bg-muted/20 border border-border/40 rounded-xl px-4 text-sm font-black">
                <option value="browser">Browser engine (Camoufox)</option>
                <option value="httpx">Direct engine (Httpx fingerprint)</option>
                <option value="hybrid">Hybrid engine (adaptive)</option>
              </select>
            </Field>
            <p className="text-[10px] text-muted-foreground">Engine mode takes full effect after a gateway restart; concurrency changes apply immediately.</p>
          </Card>

          <Card title="Compression" icon={Zap} color="emerald">
            <p className="text-xs text-muted-foreground leading-relaxed">
              Mirrors 9router. <code className="text-emerald-500 font-mono">RTK</code> compresses tool output via filter chain (git-diff, grep, ls, tree…). <code className="text-emerald-500 font-mono">Caveman</code> injects a terse-response hint into the system prompt.
            </p>

            <div className="flex items-center justify-between p-3 rounded-xl bg-muted/30 border border-border/40">
              <div>
                <p className="text-sm font-black">RTK</p>
                <p className="text-[10px] text-muted-foreground">Per-tool filters on tool_result content. Auto-detected.</p>
              </div>
              <button
                onClick={() => {
                  const next = !rtkEnabled
                  setRtkEnabled(next)
                  saveSetting('rtk_enabled', next, 'RTK')
                }}
                className={`relative w-11 h-6 rounded-full transition-colors ${rtkEnabled ? 'bg-emerald-500' : 'bg-muted/50 border border-border/60'}`}>
                <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${rtkEnabled ? 'translate-x-5' : ''}`} />
              </button>
            </div>

            <div className="flex items-center justify-between p-3 rounded-xl bg-muted/30 border border-border/40">
              <div>
                <p className="text-sm font-black">Caveman</p>
                <p className="text-[10px] text-muted-foreground">Adds a terse-response instruction to the system prompt.</p>
              </div>
              <button
                onClick={() => {
                  const next = !cavemanEnabled
                  setCavemanEnabled(next)
                  saveSetting('caveman_enabled', next, 'Caveman')
                }}
                className={`relative w-11 h-6 rounded-full transition-colors ${cavemanEnabled ? 'bg-emerald-500' : 'bg-muted/50 border border-border/60'}`}>
                <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${cavemanEnabled ? 'translate-x-5' : ''}`} />
              </button>
            </div>

            {cavemanEnabled && (
              <div className="grid grid-cols-3 gap-1.5 p-1 rounded-xl bg-muted/30 border border-border/40">
                {([
                  { v: 'lite', label: 'Lite', desc: 'Drop filler, keep grammar' },
                  { v: 'full', label: 'Full', desc: 'Drop articles, fragments OK' },
                  { v: 'ultra', label: 'Ultra', desc: 'Telegraphic, max compression' },
                ] as const).map(opt => {
                  const active = cavemanLevel === opt.v
                  return (
                    <button
                      key={opt.v}
                      onClick={() => {
                        setCavemanLevel(opt.v)
                        saveSetting('caveman_level', opt.v, 'Caveman level')
                      }}
                      className={`flex flex-col items-center gap-0.5 px-2 py-2 rounded-lg text-xs font-black transition-all ${
                        active
                          ? 'bg-emerald-500 text-white shadow-md shadow-emerald-500/30'
                          : 'hover:bg-muted/50 text-muted-foreground'
                      }`}
                    >
                      <span>{opt.label}</span>
                      <span className={`text-[9px] font-medium ${active ? 'text-emerald-50' : 'text-muted-foreground/70'}`}>{opt.desc}</span>
                    </button>
                  )
                })}
              </div>
            )}

            <p className="text-[10px] text-muted-foreground">Applies on the next request. Defaults match 9router: <span className="text-emerald-500 font-semibold">RTK on, Caveman off</span>.</p>
          </Card>

          <div className="flex gap-3 p-5 rounded-2xl bg-amber-500/10 border border-amber-500/20">
            <TriangleAlert className="w-5 h-5 text-amber-500 shrink-0 mt-0.5" />
            <p className="text-[11px] text-amber-600 dark:text-amber-400 font-bold leading-relaxed">
              After changing the engine mode, restart the backend service to ensure all engines initialize correctly.
            </p>
          </div>

        </div>
      </div>
    </div>
  )
}
