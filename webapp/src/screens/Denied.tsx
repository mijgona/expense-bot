export function Denied() {
  return (
    <div className="center">
      <div className="big-emoji">⛔</div>
      <div className="card-title">Доступ запрещён</div>
      <div className="hint">Ваш аккаунт не добавлен в список пользователей. Попросите владельца бота дать доступ.</div>
    </div>
  )
}

export function Reopen() {
  return (
    <div className="center">
      <div className="big-emoji">🔄</div>
      <div className="card-title">Откройте приложение заново из Telegram</div>
      <div className="hint">Сессия устарела или недействительна. Закройте окно и снова нажмите «Открыть» в боте.</div>
    </div>
  )
}
