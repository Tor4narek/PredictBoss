# Domain (.NET 10)

Локальная реализация в ветке `egor`. Проекты: `Core` (сущности), `Infrastructure` (EF Core и импорт CSV), `Api` (HTTP), `Worker` (сбор и доставка). БД Domain отделена от PostgreSQL ESB. API применяет миграцию EF Core при запуске; Worker запускается после API.

## Запуск

1. Скопировать `.env.example` в `.env` в корне репозитория. Заполнить `LLM_API_KEY`, `HERMES_API_KEY`, `ESB_API_KEY`, `S3_BUCKET` и `DOMAIN_JWT_KEY` (минимум 32 байта). Путь `DOMAIN_CSV_PATH` по умолчанию указывает на предоставленный `predictor/domain/data/df_Uid_data_hack.csv`.
2. Выполнить `docker compose up --build domain-api domain-worker`. Compose также запустит зависимости ESB. API доступен на `http://localhost:8080`, проверка готовности — `GET /health`.
   Swagger UI доступен на `http://localhost:8080/swagger`, OpenAPI JSON — на `/swagger/v1/swagger.json`. В Compose он включён через `SWAGGER_ENABLED=true`; вне локального окружения установите `false`. Для защищённых маршрутов нажмите **Authorize** и введите JWT.
3. Зарегистрироваться через `POST /auth/register` либо войти через `POST /auth/login`; обе операции возвращают JWT. Передавать его в `Authorization: Bearer ...`. Логин — 3–128 символов без пробелов, регистр не учитывается; пароль — 12–256 символов. JWT действует один час.

Локально: `dotnet test predictor/domain/PredictBoss.Domain.slnx`. Новую миграцию: `dotnet ef migrations add <Name> --project predictor/domain/src/PredictBoss.Domain.Infrastructure --startup-project predictor/domain/src/PredictBoss.Domain.Infrastructure --output-dir Migrations`.

## Реализовано

- EF Core-модель, миграции, регистрация и вход с хешированием пароля, одноразовая связь пользователя–компании, ревизия профиля и версии источников.
- Создание компании вручную или из полного CSV, изменение профиля; CRUD источников и разовых ссылок.
- Worker с арендами PostgreSQL и параллельной обработкой ссылок. RSS, Atom, sitemap и HTML-индекс; извлечение HTML с Playwright при необходимости, PDF с текстовым слоем; отправка профиля и новости по актуальному ESB-контракту.
- Повторы по статусам в таблицах; текст новости в Domain PostgreSQL не хранится.

## Контракты Domain → ESB

- Профиль отправляется в `POST /call?agent=valera` с `companyId`, `profileRevision` и полным объектом `company`.
- Каждая новость отправляется в `POST /call?agent=serezha` с `news_id`, `source_id`, `company_id`, URL, типом материала, типом содержимого, заголовком, датой публикации и очищенным `news_text`. В текущей модели `news_id` и `source_id` равны `news_links.id`.
- Все вызовы ESB подписываются `Authorization: Bearer <ESB_API_KEY>`. Domain не читает и не пишет PostgreSQL ESB.
- Domain не реализует API чата. Контракт анализа и WebSocket находится в [общей спецификации](../docs/esb-contracts.md).

Парсер выбирает ссылки на HTML-индексах эвристически; правила следует проверить на целевых источниках.
