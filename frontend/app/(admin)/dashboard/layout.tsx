'use client'

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { Activity, Key, Settings, LayoutDashboard, MessageSquare, Menu, X, Image, Film, LogOut, Sun, Moon, Layers, ChevronLeft, ChevronRight, Network, Terminal, ChevronDown } from "lucide-react"
import QCatIcon from "@/components/QCatIcon"
import { useState, useEffect, useRef } from "react"
import { toast } from "sonner"

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [isDark, setIsDark] = useState(true)
  const [collapsed, setCollapsed] = useState(false)
  useEffect(() => {
    setCollapsed(localStorage.getItem("sidebar_collapsed") === "true")
  }, [])

  useEffect(() => {
    const key = localStorage.getItem('qwenpi_key')
    if (!key) router.push("/login")
    else fetch('/api/auth/login/check').catch(() => {})
  }, [router, pathname])

  const sseRef = useRef<EventSource | null>(null)
  useEffect(() => {
    const key = localStorage.getItem('qwenpi_key')
    if (!key) return
    const connect = () => {
      const es = new EventSource(`/api/admin/events?key=${key}`)
      sseRef.current = es
      es.onmessage = (e) => {
        try {
          const data = JSON.parse(e.data)
          const type = data.type || ''
          const msg = data.message || ''
          if (type === 'account_banned') toast.error(msg, { duration: 10000 })
          else if (type === 'replenish_success') toast.success(msg, { duration: 8000 })
          else if (type === 'replenish_error') toast.warning(msg, { duration: 8000 })
          else if (type === 'replenish_stopped') toast.error(msg, { duration: 15000 })
          else if (type === 'account_removed') toast.info(msg, { duration: 5000 })
        } catch { }
      }
      es.onerror = () => { es.close(); setTimeout(connect, 5000) }
    }
    connect()
    return () => { sseRef.current?.close() }
  }, [])

  const lightColorOverrides: Record<string, string> = {
    '--color-background': 'hsl(210 40% 98%)',
    '--color-foreground': 'hsl(222.2 84% 4.9%)',
    '--color-card': 'hsl(0 0% 100%)',
    '--color-card-foreground': 'hsl(222.2 84% 4.9%)',
    '--color-popover': 'hsl(0 0% 100%)',
    '--color-popover-foreground': 'hsl(222.2 84% 4.9%)',
    '--color-primary': 'hsl(221.2 83.2% 53.3%)',
    '--color-primary-foreground': 'hsl(210 40% 98%)',
    '--color-secondary': 'hsl(210 40% 96.1%)',
    '--color-secondary-foreground': 'hsl(222.2 47.4% 11.2%)',
    '--color-muted': 'hsl(210 40% 96.1%)',
    '--color-muted-foreground': 'hsl(215.4 16.3% 46.9%)',
    '--color-accent': 'hsl(210 40% 96.1%)',
    '--color-accent-foreground': 'hsl(222.2 47.4% 11.2%)',
    '--color-destructive': 'hsl(0 84.2% 60.2%)',
    '--color-destructive-foreground': 'hsl(210 40% 98%)',
    '--color-border': 'hsl(214.3 31.8% 91.4%)',
    '--color-input': 'hsl(214.3 31.8% 91.4%)',
    '--color-ring': 'hsl(221.2 83.2% 53.3%)',
  }

  const applyTheme = (theme: string) => {
    const el = document.documentElement
    el.setAttribute('data-theme', theme)
    if (theme === 'light') {
      el.classList.remove('dark')
      Object.entries(lightColorOverrides).forEach(([k, v]) => el.style.setProperty(k, v))
    } else {
      el.classList.add('dark')
      Object.entries(lightColorOverrides).forEach(([k]) => el.style.removeProperty(k))
    }
  }

  useEffect(() => {
    const saved = localStorage.getItem('theme') || 'dark'
    setIsDark(saved === 'dark')
    applyTheme(saved)
  }, [])

  const themeBtnRef = useRef<HTMLButtonElement>(null)
  const isAnimating = useRef(false)

  const toggleTheme = () => {
    if (isAnimating.current) return
    isAnimating.current = true
    const next = isDark ? 'light' : 'dark'
    const btn = themeBtnRef.current
    const rect = btn?.getBoundingClientRect()
    const x = rect ? rect.left + rect.width / 2 : window.innerWidth / 2
    const y = rect ? rect.top + rect.height / 2 : window.innerHeight / 2
    const overlay = document.createElement('div')
    overlay.style.cssText = `position:fixed;inset:0;z-index:99999;pointer-events:none;background:${next === 'light' ? '#f0f4f8' : '#0d0e16'};clip-path:circle(0px at ${x}px ${y}px);transition:clip-path 0.35s cubic-bezier(0.4,0,0.2,1);`
    document.body.appendChild(overlay)
    requestAnimationFrame(() => { requestAnimationFrame(() => { overlay.style.clipPath = `circle(200vmax at ${x}px ${y}px)` }) })
    setTimeout(() => { setIsDark(!isDark); localStorage.setItem('theme', next); applyTheme(next) }, 200)
    overlay.addEventListener('transitionend', () => {
      setTimeout(() => { overlay.style.transition = 'opacity 0.15s ease'; overlay.style.opacity = '0'; setTimeout(() => { overlay.remove(); isAnimating.current = false }, 150) }, 50)
    }, { once: true })
  }

  const toggleCollapse = () => {
    const next = !collapsed
    setCollapsed(next)
    localStorage.setItem("sidebar_collapsed", String(next))
  }

  const handleLogout = async () => {
    try { await fetch('/api/auth/logout', { method: 'POST' }) } catch {}
    localStorage.removeItem('qwenpi_key')
    router.push("/")
  }

  type Nav = { name: string; path?: string; icon: any; children?: { name: string; path: string }[] }
  const navs: Nav[] = [
    { name: "Overview", path: "/dashboard", icon: LayoutDashboard },
    { name: "Providers", path: "/dashboard/providers", icon: Layers },
    { name: "Proxy Pool", path: "/dashboard/proxy-pool", icon: Network },
    { name: "Proxy Scraper", path: "/dashboard/proxy-scraper", icon: Network },
    { name: "Accounts", path: "/dashboard/accounts", icon: Activity },
    { name: "API Keys", path: "/dashboard/tokens", icon: Key },
    { name: "Playground", path: "/dashboard/playground", icon: MessageSquare },
    { name: "Image Studio", path: "/dashboard/images", icon: Image },
    { name: "Video Studio", path: "/dashboard/videos", icon: Film },
    {
      name: "Console Log",
      path: "/dashboard/console",
      icon: Terminal,
      children: [
        { name: "Request Log", path: "/dashboard/console/request-log" },
        { name: "Auto Script", path: "/dashboard/console/auto-script" },
      ],
    },
    { name: "Settings", path: "/dashboard/settings", icon: Settings },
  ]

  const [expandedGroup, setExpandedGroup] = useState<string | null>(
    pathname.startsWith("/dashboard/console") ? "Console Log" : null
  )
  useEffect(() => {
    if (pathname.startsWith("/dashboard/console")) setExpandedGroup("Console Log")
  }, [pathname])

  const sidebarWidth = collapsed ? "w-[72px]" : "w-64"

  return (
    <div className="flex min-h-screen w-full bg-background text-foreground transition-colors duration-500 overflow-hidden font-sans">
      {mobileOpen && (
        <div className="fixed inset-0 bg-black/80 z-40 md:hidden backdrop-blur-xl" onClick={() => setMobileOpen(false)} />
      )}

      <aside className={`sidebar-themed fixed md:static inset-y-0 left-0 ${sidebarWidth} flex-col flex z-50 border-r border-border/30 transition-all duration-300 ease-in-out ${mobileOpen ? "translate-x-0 w-64" : "-translate-x-full md:translate-x-0"}`}>
        <div className={`h-16 flex items-center ${collapsed ? "justify-center px-2" : "justify-between px-5"} border-b border-border/20`}>
          <div className="flex items-center gap-2.5">
            <div className="h-9 w-9 rounded-xl flex items-center justify-center bg-gradient-to-br from-indigo-500 to-fuchsia-500 shadow-lg shadow-indigo-500/25">
              <QCatIcon className="h-6 w-6" />
            </div>
            {!collapsed && <span className="font-black text-lg tracking-tight text-foreground">Qwenpi</span>}
          </div>
          {!collapsed && (
            <button className="md:hidden text-muted-foreground p-1.5" onClick={() => setMobileOpen(false)}>
              <X className="h-5 w-5" />
            </button>
          )}
        </div>

        <nav className="flex-1 py-4 px-2 space-y-1 overflow-y-auto">
          {navs.map(n => {
            const hasChildren = n.children && n.children.length > 0
            const parentActive = !!(n.path && (pathname === n.path || (n.path !== "/dashboard" && pathname.startsWith(n.path))))

            if (hasChildren) {
              const expanded = !collapsed && expandedGroup === n.name
              return (
                <div key={n.name}>
                  <button
                    onClick={() => {
                      if (collapsed) {
                        if (n.children && n.children[0]) router.push(n.children[0].path)
                      } else {
                        setExpandedGroup(expanded ? null : n.name)
                      }
                    }}
                    title={collapsed ? n.name : undefined}
                    className={`group relative w-full flex items-center ${collapsed ? "justify-center h-10 w-10 mx-auto" : "gap-3 px-3 h-10"} rounded-xl text-[13px] font-semibold transition-all duration-200 ${parentActive
                      ? "bg-gradient-to-r from-indigo-500/15 to-fuchsia-500/10 text-indigo-400 shadow-sm shadow-indigo-500/10"
                      : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                    }`}
                  >
                    <n.icon className={`h-[18px] w-[18px] shrink-0 ${parentActive ? "text-indigo-400" : "opacity-60 group-hover:opacity-100"}`} />
                    {!collapsed && <span className="truncate flex-1 text-left">{n.name}</span>}
                    {!collapsed && <ChevronDown className={`h-3.5 w-3.5 opacity-60 transition-transform ${expanded ? "rotate-180" : ""}`} />}
                    {parentActive && <div className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 rounded-r-full bg-indigo-500" />}
                    {collapsed && (
                      <div className="absolute left-full ml-2 px-2.5 py-1 rounded-lg bg-popover text-popover-foreground text-[11px] font-semibold shadow-xl border border-border/40 opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity whitespace-nowrap z-50">
                        {n.name}
                      </div>
                    )}
                  </button>
                  {expanded && (
                    <div className="ml-7 mt-1 space-y-0.5 border-l border-border/30 pl-2">
                      {n.children!.map(c => {
                        const childActive = pathname === c.path
                        return (
                          <Link
                            key={c.path}
                            href={c.path}
                            onClick={() => setMobileOpen(false)}
                            className={`block px-3 py-1.5 rounded-lg text-[12px] font-medium transition-all ${childActive
                              ? "bg-indigo-500/10 text-indigo-400"
                              : "text-muted-foreground hover:bg-muted/40 hover:text-foreground"
                            }`}
                          >
                            {c.name}
                          </Link>
                        )
                      })}
                    </div>
                  )}
                </div>
              )
            }

            return (
              <Link
                key={n.path}
                href={n.path!}
                onClick={() => setMobileOpen(false)}
                title={collapsed ? n.name : undefined}
                className={`group relative flex items-center ${collapsed ? "justify-center h-10 w-10 mx-auto" : "gap-3 px-3 h-10"} rounded-xl text-[13px] font-semibold transition-all duration-200 ${parentActive
                  ? "bg-gradient-to-r from-indigo-500/15 to-fuchsia-500/10 text-indigo-400 shadow-sm shadow-indigo-500/10"
                  : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                }`}
              >
                <n.icon className={`h-[18px] w-[18px] shrink-0 ${parentActive ? "text-indigo-400" : "opacity-60 group-hover:opacity-100"}`} />
                {!collapsed && <span className="truncate">{n.name}</span>}
                {parentActive && <div className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 rounded-r-full bg-indigo-500" />}
                {collapsed && (
                  <div className="absolute left-full ml-2 px-2.5 py-1 rounded-lg bg-popover text-popover-foreground text-[11px] font-semibold shadow-xl border border-border/40 opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity whitespace-nowrap z-50">
                    {n.name}
                  </div>
                )}
              </Link>
            )
          })}
        </nav>

        <div className={`${collapsed ? "px-2 pb-3" : "px-3 pb-4"} space-y-2`}>
          <button
            onClick={toggleCollapse}
            className="hidden md:flex w-full items-center justify-center h-8 rounded-lg bg-muted/30 hover:bg-muted/60 text-muted-foreground transition-all"
          >
            {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
          </button>

          <button
            ref={themeBtnRef}
            onClick={toggleTheme}
            className={`w-full ${collapsed ? "h-9 justify-center" : "h-9 justify-center gap-2"} flex items-center rounded-lg bg-muted/20 hover:bg-muted/40 border border-border/30 transition-all text-xs font-semibold`}
          >
            {isDark ? <Sun className="w-3.5 h-3.5 text-amber-400" /> : <Moon className="w-3.5 h-3.5 text-indigo-400" />}
            {!collapsed && <span>{isDark ? "Light" : "Dark"}</span>}
          </button>

          <button
            onClick={handleLogout}
            className={`w-full ${collapsed ? "h-9 justify-center" : "h-9 justify-center gap-2"} flex items-center rounded-lg hover:bg-rose-500/10 hover:text-rose-500 border border-border/30 transition-all text-xs font-semibold text-muted-foreground`}
          >
            <LogOut className="w-3.5 h-3.5" />
            {!collapsed && <span>Sign out</span>}
          </button>

          {!collapsed && (
            <div className="flex items-center justify-center gap-2 h-7 rounded-lg bg-indigo-500/10 text-indigo-400 text-[10px] font-medium">
              <span>v2.0.0</span>
              <a href="https://github.com/hirotomasato/Qwenpi" target="_blank" rel="noopener noreferrer" className="hover:text-indigo-300 transition-colors" title="GitHub">
                <svg className="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" /></svg>
              </a>
            </div>
          )}
        </div>
      </aside>

      <main className="flex-1 flex flex-col relative overflow-hidden h-screen bg-background">
        <header className="h-14 flex items-center justify-between px-5 border-b border-border/30 bg-background/80 backdrop-blur-xl md:hidden z-10 shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="h-7 w-7 rounded-lg flex items-center justify-center bg-gradient-to-br from-indigo-500 to-fuchsia-500">
              <QCatIcon className="h-5 w-5" />
            </div>
            <span className="font-black text-base tracking-tight">Qwenpi</span>
          </div>
          <button className="text-muted-foreground p-1.5" onClick={() => setMobileOpen(true)}>
            <Menu className="h-6 w-6" />
          </button>
        </header>

        <div className="flex-1 overflow-y-auto relative px-4 py-5 md:px-8 md:py-8 custom-scrollbar">
          {children}
        </div>
      </main>
    </div>
  )
}
