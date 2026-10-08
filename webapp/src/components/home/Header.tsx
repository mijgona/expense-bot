import { useSession } from '../../context'

const dateFmt = new Intl.DateTimeFormat('ru-RU', {
  weekday: 'long',
  day: 'numeric',
  month: 'long',
  timeZone: 'Asia/Dushanbe',
})

/** «Четверг, 8 октября» + «Привет, {имя}» (008 FR-001). */
export function Header() {
  const session = useSession()
  const date = dateFmt.format(new Date())
  const name = session.user.displayName || session.user.firstName
  return (
    <header className="b-header">
      <span className="b-header-date">{date.charAt(0).toUpperCase() + date.slice(1)}</span>
      <h1>Привет, {name}</h1>
    </header>
  )
}
