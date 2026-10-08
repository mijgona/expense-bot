import { useEffect, useState } from 'react'

/** Spinner; after 3 s adds a hint that the free-tier server may be waking up. */
export function Loader({ text }: { text?: string }) {
  const [slow, setSlow] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setSlow(true), 3000)
    return () => clearTimeout(t)
  }, [])

  return (
    <div className="loader" role="status" aria-live="polite">
      <div className="spinner" />
      {text && <div>{text}</div>}
      {slow && <div className="hint">Сервер просыпается…</div>}
    </div>
  )
}
