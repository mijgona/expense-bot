import { useNav } from '../../context'
import { IconCard, IconSave } from '../icons'
import { formatAmount } from './format'

/** «Долг по карте» with «Погасить», or neutral «Долга нет» (008 FR-008). Opens the card screen. */
export function DebtBlock({ debt }: { debt: number }) {
  const nav = useNav()
  const open = () => nav.push({ name: 'credit' })
  if (debt <= 0) {
    return (
      <section className="b-block">
        <button className="b-block-main" onClick={open}>
          <span className="b-block-icon">
            <IconCard size={20} />
          </span>
          <span className="b-block-text">
            <span className="b-block-label">Долг по карте</span>
            <span className="b-block-value neutral">Долга нет</span>
          </span>
        </button>
      </section>
    )
  }
  return (
    <section className="b-block debt">
      <button className="b-block-main" onClick={open}>
        <span className="b-block-icon">
          <IconCard size={20} />
        </span>
        <span className="b-block-text">
          <span className="b-block-label">Долг по карте</span>
          <span className="b-block-value num display">{formatAmount(debt)} с.</span>
        </span>
      </button>
      <button className="b-outline" onClick={open}>
        Погасить
      </button>
    </section>
  )
}

/** «Накопления» with «Пополнить» (008 FR-009). Opens Savings on the deposit form. */
export function SavingsBlock({ balance }: { balance: number }) {
  const nav = useNav()
  const open = () => nav.push({ name: 'savings', mode: 'deposit' })
  return (
    <section className="b-block savings">
      <button className="b-block-main" onClick={open}>
        <span className="b-block-icon">
          <IconSave size={20} />
        </span>
        <span className="b-block-text">
          <span className="b-block-label">Накопления</span>
          <span className="b-block-value num display">{formatAmount(balance)} с.</span>
        </span>
      </button>
      <button className="b-outline" onClick={open}>
        Пополнить
      </button>
    </section>
  )
}
