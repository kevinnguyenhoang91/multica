'use client'

import { useLayoutEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { isAuthenticated } from '@/lib/auth'

interface AuthGuardProps {
  children: React.ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const router = useRouter()
  // Initialize state synchronously to avoid cascading renders
  const [authState] = useState(() => {
    if (typeof window === 'undefined') return { checking: true, authed: false }
    const authed = isAuthenticated()
    return { checking: false, authed }
  })

  useLayoutEffect(() => {
    if (!authState.checking && !authState.authed) {
      router.replace('/login')
    }
  }, [authState, router])

  if (authState.checking) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <div className="size-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
      </div>
    )
  }

  if (!authState.authed) {
    return null
  }

  return <>{children}</>
}
