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

/** Native Telegram confirm dialog; falls back to window.confirm outside Telegram (dev). */
export function confirmAction(message: string): Promise<boolean> {
  const app = tg()
  if (app?.initData && typeof app.showConfirm === 'function') {
    return new Promise((resolve) => {
      try {
        app.showConfirm(message, (ok) => resolve(ok))
      } catch {
        resolve(window.confirm(message))
      }
    })
  }
  return Promise.resolve(window.confirm(message))
}

/** Short non-blocking notice at the bottom of the screen. */
export function toast(text: string): void {
  const el = document.createElement('div')
  el.className = 'toast'
  el.textContent = text
  document.body.appendChild(el)
  setTimeout(() => el.remove(), 1800)
}
