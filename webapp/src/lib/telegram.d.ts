// Minimal hand-written types for the subset of Telegram.WebApp used by this app.
// Full reference: https://core.telegram.org/bots/webapps#initializing-mini-apps

export interface TelegramWebAppUser {
  id: number
  first_name: string
  last_name?: string
  username?: string
  language_code?: string
}

export interface TelegramThemeParams {
  bg_color?: string
  text_color?: string
  hint_color?: string
  link_color?: string
  button_color?: string
  button_text_color?: string
  secondary_bg_color?: string
  section_bg_color?: string
  destructive_text_color?: string
}

export interface TelegramBackButton {
  isVisible: boolean
  show(): void
  hide(): void
  onClick(cb: () => void): void
  offClick(cb: () => void): void
}

export interface TelegramMainButton {
  text: string
  isVisible: boolean
  show(): void
  hide(): void
}

export interface TelegramHapticFeedback {
  notificationOccurred(type: 'error' | 'success' | 'warning'): void
}

export interface TelegramWebApp {
  initData: string
  initDataUnsafe: { user?: TelegramWebAppUser; auth_date?: number; hash?: string }
  version: string
  platform: string
  colorScheme: 'light' | 'dark'
  themeParams: TelegramThemeParams
  BackButton: TelegramBackButton
  MainButton: TelegramMainButton
  HapticFeedback: TelegramHapticFeedback
  ready(): void
  expand(): void
  showAlert(message: string, callback?: () => void): void
  onEvent(event: 'themeChanged', cb: () => void): void
  offEvent(event: 'themeChanged', cb: () => void): void
}

declare global {
  interface Window {
    Telegram?: { WebApp?: TelegramWebApp }
  }
}
