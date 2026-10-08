import { ApiError } from '../lib/api'

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return 'Что-то пошло не так'
}

export function ErrorBanner({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="banner" role="alert">
      <div>❌ {errorMessage(error)}</div>
      {onRetry && (
        <button className="btn-link" onClick={onRetry}>
          Повторить
        </button>
      )}
    </div>
  )
}
