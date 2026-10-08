import { useEffect, useRef, useState } from 'react'
import { ApiError, dismissPayout, listHistory, recordPayout } from '../lib/api'
import { diramToInput, formatSomoni, parseAmountInput } from '../lib/money'
import { formatDate, todayDushanbe } from '../lib/months'
import { formatPayday, payoutTitle } from '../lib/payouts'
import { haptic, toast } from '../lib/telegram'
import type { Payout, Transaction } from '../lib/types'
import { AmountInput } from './AmountInput'
import { ErrorBanner } from './ErrorBanner'

interface Props {
  payout: Payout
  /** Open the confirmation immediately (reminder deep link). */
  autoOpen?: boolean
  /** Called after the payment was recorded or dismissed, so Home reloads summary and payouts. */
  onChanged(): void
}

/** Home card for an expected salary payment: «💵 Аванс 8 000 с. — 15 октября» + one-tap confirmation. */
export function PayoutOffer({ payout, autoOpen = false, onChanged }: Props) {
  const today = todayDushanbe()
  // An upcoming payday can't be used as the record date (no future dates) — default to today then.
  const defaultDate = payout.payday <= today ? payout.payday : today
  const [open, setOpen] = useState(autoOpen)
  const [amount, setAmount] = useState(diramToInput(payout.amount))
  const [date, setDate] = useState(defaultDate)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const cardRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (autoOpen) cardRef.current?.scrollIntoView({ block: 'center' })
  }, [autoOpen])

  const parsed = parseAmountInput(amount)
  const dateError = date > today ? 'Дата не может быть в будущем' : null
  const apiErr = error instanceof ApiError ? error : null
  const serverField = apiErr?.code === 'validation' ? apiErr.field : undefined
  const canSave = parsed.ok && !dateError && !busy

  async function record() {
    if (!parsed.ok || dateError) return
    setBusy(true)
    setError(null)
    try {
      await recordPayout(payout.month, payout.kind, {
        amount: parsed.diram,
        date: date === payout.payday ? undefined : date,
      })
      haptic('success')
      toast('Записано')
      onChanged()
    } catch (e) {
      haptic('error')
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  async function dismiss() {
    setBusy(true)
    setError(null)
    try {
      await dismissPayout(payout.month, payout.kind)
      onChanged()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card payout" ref={cardRef}>
      <div className="row">
        <span className="payout-title">
          💵 {payoutTitle(payout.kind)} {formatSomoni(payout.amount)}
        </span>
        <span className="hint">— {formatPayday(payout.payday)}</span>
      </div>

      {!open && (
        <div className="payout-actions">
          <button className="btn" disabled={busy} onClick={() => setOpen(true)}>
            Записать
          </button>
          <button className="btn btn-secondary" disabled={busy} onClick={dismiss}>
            Уже записал
          </button>
        </div>
      )}

      {open && (
        <>
          <AmountInput
            id={`payout-amount-${payout.month}-${payout.kind}`}
            value={amount}
            onChange={(v) => {
              setAmount(v)
              setError(null)
            }}
            error={!parsed.ok ? parsed.error : serverField === 'amount' ? apiErr!.message : null}
          />
          <div className="field">
            <label htmlFor={`payday-${payout.month}-${payout.kind}`}>Дата</label>
            <input
              id={`payday-${payout.month}-${payout.kind}`}
              type="date"
              className={'input' + (dateError || serverField === 'date' ? ' invalid' : '')}
              max={today}
              value={date}
              onChange={(e) => setDate(e.target.value || defaultDate)}
            />
            {(dateError || serverField === 'date') && (
              <div className="error-text">{dateError ?? apiErr!.message}</div>
            )}
          </div>
          <div className="payout-actions">
            <button className="btn" disabled={!canSave} onClick={record}>
              {busy ? 'Записываю…' : 'Подтвердить'}
            </button>
            <button className="btn btn-secondary" disabled={busy} onClick={() => setOpen(false)}>
              Отмена
            </button>
          </div>
        </>
      )}

      {error != null && !serverField && <ErrorBanner error={error} />}
    </div>
  )
}

interface RecordedProps {
  payout: Payout
  onOpenHistory(): void
}

/** Shown when the reminder link points to a payment that is already recorded (e.g. on another device). */
export function PayoutRecorded({ payout, onOpenHistory }: RecordedProps) {
  const [tx, setTx] = useState<Transaction | null>(null)

  useEffect(() => {
    let alive = true
    if (!payout.transactionId) return
    listHistory({ month: payout.month, group: 'income', limit: 100 })
      .then((page) => {
        const found = page.items.find((t) => t.id === payout.transactionId) ?? null
        if (alive) setTx(found)
      })
      .catch(() => {
        /* the expected amount is shown instead */
      })
    return () => {
      alive = false
    }
  }, [payout.month, payout.transactionId])

  return (
    <div className="card payout">
      <div className="row">
        <span className="payout-title">✅ Уже записано</span>
      </div>
      <div className="row">
        <span>{payoutTitle(payout.kind)}</span>
        <span>{formatSomoni(tx ? tx.amount : payout.amount)}</span>
      </div>
      <div className="row">
        <span className="hint">Дата</span>
        <span className="hint">{tx ? formatDate(tx.occurredAt) : formatPayday(payout.payday)}</span>
      </div>
      <button className="btn-link" onClick={onOpenHistory}>
        Открыть в истории ›
      </button>
    </div>
  )
}
