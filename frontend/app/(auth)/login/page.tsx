'use client';

import { useState, useEffect } from "react"
import { useRouter } from "next/navigation"
import { KeyRound, Sparkles, ShieldCheck, Zap, ArrowRight } from "lucide-react"
import QCatIcon from "@/components/QCatIcon"
import { toast } from "sonner"

export default function LoginPage() {
    const [key, setKey] = useState("")
    const [loading, setLoading] = useState(false)
    const router = useRouter()

    useEffect(() => {
        if (!localStorage.getItem('qwenpi_key')) return
        fetch('/api/auth/login/check').then(r => {
            if (r.ok) router.push("/dashboard")
        }).catch(() => {})
    }, [router])

    const handleLogin = (e: React.FormEvent) => {
        e.preventDefault()
        if (!key.trim()) return toast.error("Please enter the admin key")

        setLoading(true)
        fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ key: key.trim() })
        }).then(res => {
            if (res.ok) {
                localStorage.setItem('qwenpi_key', key.trim())
                toast.success("Welcome back, admin")
                router.push("/dashboard")
            } else {
                toast.error("Key verification failed")
            }
        }).catch(() => {
            toast.error("Network connection error")
        }).finally(() => {
            setLoading(false)
        })
    }

    return (
        <div className="min-h-screen w-full flex items-center justify-center bg-background relative overflow-hidden p-6">
            {/* Background effects */}
            <div className="absolute inset-0 pointer-events-none overflow-hidden">
                <div className="absolute top-[-20%] left-[-10%] w-[600px] h-[600px] bg-indigo-500/8 rounded-full blur-[120px]" />
                <div className="absolute bottom-[-20%] right-[-10%] w-[500px] h-[500px] bg-fuchsia-500/6 rounded-full blur-[100px]" />
                <div className="absolute top-[40%] left-[60%] w-[300px] h-[300px] bg-violet-500/5 rounded-full blur-[80px]" />
            </div>

            <div className="w-full max-w-md relative z-10">
                {/* Card */}
                <div className="glass-card rounded-3xl p-8 md:p-10 space-y-8">
                    {/* Logo + Title */}
                    <div className="text-center space-y-4">
                        <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-indigo-500 to-fuchsia-500 shadow-2xl shadow-indigo-500/30 mx-auto">
                            <QCatIcon className="w-10 h-10" />
                        </div>
                        <div>
                            <h1 className="text-3xl font-black tracking-tight text-foreground">
                                Qwenpi
                            </h1>
                            <p className="text-sm text-muted-foreground mt-1.5">
                                Enterprise Gateway Console
                            </p>
                        </div>
                    </div>

                    {/* Status badge */}
                    <div className="flex justify-center">
                        <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20">
                            <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                            <span className="text-[11px] font-medium text-emerald-500">System online</span>
                        </div>
                    </div>

                    {/* Form */}
                    <form onSubmit={handleLogin} className="space-y-5">
                        <div className="space-y-2">
                            <label className="text-xs font-medium text-muted-foreground">Admin Key</label>
                            <div className="relative">
                                <div className="absolute inset-y-0 left-4 flex items-center pointer-events-none">
                                    <KeyRound className="w-4 h-4 text-muted-foreground/50" />
                                </div>
                                <input
                                    type="password"
                                    value={key}
                                    onChange={(e) => setKey(e.target.value)}
                                    placeholder="Enter your admin key"
                                    autoFocus
                                    className="w-full h-12 bg-muted/30 border border-border/50 rounded-xl pl-11 pr-4 text-sm text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:ring-2 focus:ring-indigo-500/30 focus:border-indigo-500/50 transition-all font-mono"
                                />
                            </div>
                            <p className="text-[10px] text-muted-foreground/60 ml-1">Default: 123456</p>
                        </div>

                        <button
                            disabled={loading}
                            className="w-full h-12 bg-gradient-to-r from-indigo-500 to-fuchsia-500 text-white font-bold rounded-xl flex items-center justify-center gap-2 hover:opacity-90 active:scale-[0.98] transition-all text-sm shadow-lg shadow-indigo-500/25 disabled:opacity-50"
                        >
                            {loading ? (
                                <Zap className="w-4 h-4 animate-spin" />
                            ) : (
                                <>
                                    Sign in
                                    <ArrowRight className="w-4 h-4" />
                                </>
                            )}
                        </button>
                    </form>

                    {/* Footer */}
                    <div className="pt-4 border-t border-border/30 text-center">
                        <p className="text-[10px] text-muted-foreground/50">
                            Qwenpi v2.0.0 · Enterprise Gateway
                        </p>
                    </div>
                </div>
            </div>
        </div>
    )
}
