import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { advisorReport, ApiError } from '../lib/api'
import type { AdvisorReport } from '../lib/types'
import { formatDateTime } from '../lib/months'
import { Loader } from '../components/Loader'
import { ErrorBanner } from '../components/ErrorBanner'

export function Advisor() {
  const [report, setReport] = useState<AdvisorReport | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown>(null)

  async function generate() {
    setLoading(true)
    setError(null)
    try {
      setReport(await advisorReport())
    } catch (e) {
      setError(e)
    } finally {
      setLoading(false)
    }
  }

  const busy = error instanceof ApiError && error.code === 'busy'
  const unavailable = error instanceof ApiError && error.code === 'advisor_unavailable'

  return (
    <div className="screen">
      <div className="screen-title">🤖 ИИ-отчёт</div>
      {!report && !loading && (
        <div className="card hint">Персональный анализ ваших расходов, накоплений и целей за текущий месяц.</div>
      )}

      {loading && <Loader text="Анализирую расходы… это может занять до минуты" />}

      {busy && <div className="banner">⏳ Отчёт уже готовится, подождите</div>}
      {unavailable && (
        <div className="banner">
          <div>⚠️ {(error as ApiError).message}</div>
          <div className="hint">Остальные функции работают как обычно.</div>
        </div>
      )}
      {error != null && !busy && !unavailable && <ErrorBanner error={error} onRetry={generate} />}

      {report && !loading && (
        <div className="card">
          <div className="hint">Сформирован {formatDateTime(report.generatedAt)}</div>
          {/* react-markdown renders no raw HTML by default (no rehype-raw) — safe for LLM output. */}
          <div className="markdown">
            <ReactMarkdown>{report.markdown}</ReactMarkdown>
          </div>
        </div>
      )}

      <button className="btn" disabled={loading} onClick={generate}>
        {report ? 'Обновить' : '🤖 Сформировать отчёт'}
      </button>
    </div>
  )
}
