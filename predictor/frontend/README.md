# Альфа Пульс — фронтенд PredictBoss

## Запуск

Нужен Node.js 22.13+ (проверка: `node -v`).

```bash
cd predictor/frontend
npm install      # один раз: скачать библиотеки
npm run dev      # запустить, открыть http://localhost:5173/kit
```

Остановить сервер — `Ctrl+C` в терминале.

## Команды

| Команда | Что делает |
|---|---|
| `npm run dev` | Сервер разработки, правки видны сразу |
| `npm run build` | Проверка типов и сборка в папку `dist` |
| `npm run lint` | Проверка кода ESLint |
| `npm run preview` | Посмотреть собранную версию из `dist` |

## Что где лежит

- `src/styles/tokens.css` — токены дизайна из Figma (цвета, шрифты, отступы).
- `src/ui` — UI kit; витрина всех компонентов: `/kit`.
- `src/pages` — страницы.
- `.env.example` — настройки (моки или реальный API).

Правила работы — в [CLAUDE.md](CLAUDE.md).
