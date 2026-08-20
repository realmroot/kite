import { useEffect, useState } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { IconBrandOauth, IconLoader2 } from '@tabler/icons-react'
import { useSearchParams } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function LoginPage() {
  const { user, isLoading, login, loginPrompt, providerName } = useAuth()
  const [searchParams] = useSearchParams()
  const [submitting, setSubmitting] = useState(false)
  const callbackError = searchParams.get('error')

  useEffect(() => {
    if (user) window.location.replace('./')
  }, [user])

  const handleLogin = async () => {
    setSubmitting(true)
    try {
      await login()
    } finally {
      setSubmitting(false)
    }
  }

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <IconLoader2 className="h-6 w-6 animate-spin" />
      </div>
    )
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-muted/30 p-6">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <img src="./icon.svg" alt="Kite" className="mx-auto h-16 w-16" />
          <CardTitle className="text-2xl">Sign in to Kite</CardTitle>
          <p className="text-sm text-muted-foreground">
            Your Kubernetes permissions come directly from your identity.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {loginPrompt && (
            <p className="rounded-md bg-muted p-3 text-sm">{loginPrompt}</p>
          )}
          {callbackError && (
            <p role="alert" className="text-sm text-destructive">
              Sign-in failed. Please try again or contact your administrator.
            </p>
          )}
          <Button
            className="w-full"
            disabled={submitting}
            onClick={handleLogin}
          >
            {submitting ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconBrandOauth className="h-4 w-4" />
            )}
            Continue with {providerName}
          </Button>
        </CardContent>
      </Card>
    </main>
  )
}
