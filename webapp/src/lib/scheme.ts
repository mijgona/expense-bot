import { useEffect, useState } from 'react'
import { tg } from './telegram'

export type Scheme = 'light' | 'dark'

function current(): Scheme {
  const app = tg()
  if (app?.colorScheme === 'dark' || app?.colorScheme === 'light') return app.colorScheme
  // Outside Telegram (dev): follow the OS setting.
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

/** Telegram's light/dark mode, updated live on themeChanged (constitution IV, 008 FR-016). */
export function useTelegramScheme(): Scheme {
  const [scheme, setScheme] = useState<Scheme>(current)

  useEffect(() => {
    const update = () => setScheme(current())
    const app = tg()
    if (app?.onEvent) {
      app.onEvent('themeChanged', update)
      return () => app.offEvent?.('themeChanged', update)
    }
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
    mq?.addEventListener('change', update)
    return () => mq?.removeEventListener('change', update)
  }, [])

  return scheme
}
