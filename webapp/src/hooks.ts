import { useCallback, useEffect, useRef, useState } from 'react'

export interface Async<T> {
  data: T | null
  error: unknown
  loading: boolean
  reload: () => void
}

/** Runs fn on mount and whenever deps change; ignores results of superseded calls. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): Async<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)
  const seq = useRef(0)

  useEffect(() => {
    const id = ++seq.current
    setLoading(true)
    setError(null)
    fn().then(
      (v) => {
        if (id !== seq.current) return
        setData(v)
        setLoading(false)
      },
      (e) => {
        if (id !== seq.current) return
        setError(e)
        setLoading(false)
      },
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick])

  const reload = useCallback(() => setTick((t) => t + 1), [])
  return { data, error, loading, reload }
}
