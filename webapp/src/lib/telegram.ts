import { useEffect } from 'react'
import type { TelegramWebApp } from './telegram.d'

/** Returns the Telegram WebApp object, or null outside Telegram / before the script loaded. */
export function tg(): TelegramWebApp | null {
  return window.Telegram?.WebApp ?? null
}

/** True only when launched from Telegram (signed initData is present). */
export function isInsideTelegram(): boolean {
  return !!tg()?.initData
}

export function initTelegram(): void {
  const app = tg()
  if (!app) return
  app.ready()
  app.expand()
}

export function haptic(type: 'error' | 'success' | 'warning'): void {
  try {
    tg()?.HapticFeedback?.notificationOccurred(type)
  } catch {
    // Older clients lack haptics — ignore.
  }
}

/** Shows Telegram's BackButton while onBack is set; hides it otherwise. */
export function useBackButton(onBack: (() => void) | null): void {
  useEffect(() => {
    const btn = tg()?.BackButton
    if (!btn) return
    if (!onBack) {
      btn.hide()
      return
    }
    btn.onClick(onBack)
    btn.show()
    return () => {
      btn.offClick(onBack)
    }
  }, [onBack])
}
