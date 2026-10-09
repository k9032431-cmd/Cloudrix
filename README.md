# Cloudrix

Панель управления прокси-серверами для Xray-core и sing-box. Идейно похожа на Marzban и PasarGuard: делает то же самое, но быстрее, со всеми протоколами в одном месте и с более удобным интерфейсом.

- **Бэкенд:** Go, один бинарник, база SQLite (без CGO). Веб-интерфейс встроен в бинарник.
- **Фронтенд:** React + TypeScript + Tailwind. Светлая и тёмная тема, русский и английский язык, адаптирован под телефон.

## Возможности

| | Cloudrix |
|---|---|
| Протоколы | VLESS (Reality, Vision, XHTTP), VMess, Trojan, Shadowsocks (вкл. 2022), Hysteria2, TUIC, WireGuard |
| Транспорты | TCP, WebSocket, gRPC, XHTTP, HTTPUpgrade, TCP+HTTP |
| Ядра | Xray (VLESS/VMess/Trojan/SS) и sing-box (Hysteria2/TUIC/WireGuard): конфиги генерируются автоматически |
| Подписки | обычные ссылки, base64, Clash/mihomo, sing-box. Формат выбирается по User-Agent клиента. Для WireGuard можно скачать `.conf` |
| Страница подписки | публичная страница с QR-кодом, остатком трафика и кнопками импорта в Happ, v2RayTun, Hiddify, Streisand, sing-box, Clash |
| Лимит устройств | учёт HWID (заголовок `x-hwid`), отдельный лимит для каждого пользователя, список устройств в панели |
| Пользователи | лимит трафика, срок действия, режим «ожидает» (срок пойдёт с первого подключения), сброс трафика раз в день, неделю или месяц, перевыпуск ключей одной кнопкой |
| Массовые действия | включить, отключить, сбросить трафик, продлить, удалить |
| Названия в клиенте | шаблоны `{USERNAME}`, `{DATA_LEFT}`, `{DAYS_LEFT}`, `{EXPIRE_DATE}`, `{PROTOCOL}` |
| Роли | суперадмин, админ, реселлер с лимитом пользователей. Каждый видит только своих пользователей |
| Безопасность | bcrypt, JWT, ограничение попыток входа, журнал всех действий админов, токен сессии сбрасывается при смене роли |
| Инструменты | генерация ключей Reality и WireGuard, ключей SS-2022 и паролей obfs прямо в форме |

## Быстрый старт

### Установка одной командой

На чистом сервере с Ubuntu, Debian, CentOS, Rocky или Alma (amd64 или arm64), от root:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/k9032431-cmd/Cloudrix/main/install.sh)
```

Установщик по очереди спросит:

1. **Логин и пароль главного администратора.** Если нажать Enter вместо пароля, он сгенерирует надёжный пароль.
2. **Есть ли у вас домен.** Если есть, проверит, что домен указывает на этот сервер, и предложит бесплатный SSL-сертификат Let's Encrypt (продлевается автоматически).
3. **Порт панели** (по умолчанию 2053 с доменом и 8000 без него) и название подписки, которое увидят клиенты в своих приложениях.

После этого он сам скачает панель (или соберёт её из исходников, если готового релиза нет), создаст systemd-сервис, откроет порт в firewall и покажет адрес, логин и пароль.

Управление после установки:

```bash
cloudrix-manager status      # состояние
cloudrix-manager logs        # логи в реальном времени
cloudrix-manager restart     # перезапуск
cloudrix-manager update      # обновление (перед ним делается бэкап базы)
cloudrix-manager admin       # создать админа или сбросить пароль
cloudrix-manager cert        # добавить домен и включить HTTPS позже
cloudrix-manager uninstall   # удалить
```

Установка без вопросов, например для Ansible:

```bash
CLOUDRIX_ADMIN_USERNAME=admin CLOUDRIX_ADMIN_PASSWORD='S3cure-pass' \
CLOUDRIX_DOMAIN=panel.example.com CLOUDRIX_EMAIL=me@example.com CLOUDRIX_PORT=2053 \
bash install.sh install --yes
```

Если GitHub с сервера недоступен, можно указать своё зеркало архива: `CLOUDRIX_DOWNLOAD_URL=https://mirror/cloudrix-linux-{arch}.tar.gz`.

Где что лежит: настройки в `/etc/cloudrix/cloudrix.env`, сертификаты в `/etc/cloudrix/certs`, база в `/var/lib/cloudrix`. Панель работает от отдельного системного пользователя `cloudrix`.

### Docker

```bash
cp .env.example .env   # задайте CLOUDRIX_ADMIN_PASSWORD и CLOUDRIX_HOST
docker compose up -d --build
```

### Из исходников

Понадобятся Go 1.24+ и Node 22+.

```bash
make web build
CLOUDRIX_ADMIN_USERNAME=admin CLOUDRIX_ADMIN_PASSWORD=change-me ./bin/cloudrix
```

Создать суперадмина или сбросить ему пароль вручную:

```bash
./bin/cloudrix admin create -u admin
./bin/cloudrix admin passwd -u admin
```

### Разработка

```bash
make dev    # API на :8000, UI с hot-reload на :5173
make test   # go vet + go test + проверка типов TypeScript
```

## Настройки

Все настройки задаются переменными окружения. Полный список с пояснениями лежит в [`.env.example`](.env.example). Чтобы панель сама работала по HTTPS, укажите `CLOUDRIX_TLS_CERT` и `CLOUDRIX_TLS_KEY`.

## Релизы

При пуше тега `v*` GitHub Actions собирает `cloudrix-linux-amd64.tar.gz` и `cloudrix-linux-arm64.tar.gz` и прикладывает их к релизу. Установщик скачивает именно их.

```bash
git tag v0.1.0 && git push origin v0.1.0
```

## API

Все запросы, кроме входа и подписок, требуют заголовок `Authorization: Bearer <token>`.

| Метод | Путь | |
|---|---|---|
| POST | `/api/auth/login` | вход, возвращает токен |
| GET | `/api/system/stats` | статистика и трафик за 30 дней |
| GET/POST | `/api/users` | список пользователей (`search`, `status`, `sort`, `limit`, `offset`) и создание |
| GET/PUT/DELETE | `/api/users/{id}` | |
| POST | `/api/users/bulk` | `{ids, action: enable/disable/reset_traffic/extend/add_data/delete}` |
| POST | `/api/users/{id}/reset-traffic`, `/revoke` | |
| GET | `/api/users/{id}/subscription`, `/devices`, `/traffic` | |
| GET/POST/PUT/DELETE | `/api/inbounds`, `/api/nodes`, `/api/admins` | только суперадмин |
| GET | `/api/core/config?core=xray\|sing-box&node=ID` | готовый серверный конфиг |
| GET | `/sub/{token}` | подписка (`?format=links\|base64\|clash\|singbox`) |
| GET | `/sub/{token}/info`, `/sub/{token}/wireguard/{tag}.conf` | |

## Структура

```
cmd/cloudrix        точка входа и CLI
internal/api        HTTP API, авторизация, подписки, раздача UI
internal/store      SQLite и миграции
internal/sub        ссылки, Clash и sing-box для клиентов
internal/core       серверные конфиги Xray и sing-box
internal/jobs       истечение срока, лимиты, режим «ожидает», сброс трафика
web/                React-интерфейс (собирается в web/dist и встраивается в бинарник)
```

## Дорожная карта

- [x] Каркас: API, роли, пользователи, инбаунды, подписки для всех протоколов, интерфейс
- [ ] **Агент ноды** (`cloudrix-node`): gRPC + mTLS, запуск Xray и sing-box, горячее добавление и удаление пользователей без перезапуска ядра
- [ ] Сбор трафика и онлайна из Stats API ядер, учёт коэффициента ноды
- [ ] Локальное ядро на сервере с панелью
- [ ] Проверка состояния нод, автоматическое переключение между нодами, группы хостов
- [ ] 2FA (TOTP) для админов, API-ключи
- [ ] Telegram-бот, уведомления (истекает срок, кончается трафик, нода недоступна), вебхуки
- [ ] Тарифы и оплата, баланс реселлеров
- [ ] Миграция из Marzban и PasarGuard
- [ ] Prometheus `/metrics`, поддержка PostgreSQL
