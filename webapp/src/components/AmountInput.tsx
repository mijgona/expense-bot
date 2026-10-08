interface Props {
  value: string
  onChange(value: string): void
  error?: string | null
  label?: string
  autoFocus?: boolean
  /** DOM id; needed when several amount fields are on one screen. */
  id?: string
}

/** Decimal amount field. Validation is done by the caller via parseAmountInput. */
export function AmountInput({ value, onChange, error, label = 'Сумма, с.', autoFocus = true, id = 'amount' }: Props) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        className={'input input-amount' + (error ? ' invalid' : '')}
        inputMode="decimal"
        autoComplete="off"
        placeholder="0"
        autoFocus={autoFocus}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {error && <div className="error-text">{error}</div>}
    </div>
  )
}
