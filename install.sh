#!/usr/bin/env bash
# Cloudrix — установщик и менеджер панели.
#
# Установка одной командой:
#   bash <(curl -fsSL https://raw.githubusercontent.com/k9032431-cmd/Cloudrix/main/install.sh)
#
# После установки доступна команда `cloudrix-manager`. Без аргументов она
# открывает меню; обновление панели — пункт меню или `cloudrix-manager update`.
#
# Конкретная версия: CLOUDRIX_VERSION=v0.1.0. Своё зеркало архива:
#   CLOUDRIX_DOWNLOAD_URL=https://mirror.example.com/cloudrix-linux-{arch}.tar.gz
#
# Без вопросов (для автоматизации), все значения через переменные:
#   CLOUDRIX_ADMIN_USERNAME=admin CLOUDRIX_ADMIN_PASSWORD=secret123 \
#   CLOUDRIX_DOMAIN=panel.example.com CLOUDRIX_EMAIL=me@example.com \
#   bash install.sh install --yes

set -Eeuo pipefail

# Откуда ставить и обновлять. При установке запоминается в $INSTALL_CONF,
# поэтому `update` берёт новые версии из того же репозитория и ветки.
REPO="${CLOUDRIX_REPO-}"
BRANCH="${CLOUDRIX_BRANCH-}"
RELEASE="${CLOUDRIX_VERSION:-latest}"
DEFAULT_REPO="k9032431-cmd/Cloudrix"

APP_DIR="/opt/cloudrix"
CONF_DIR="/etc/cloudrix"
DATA_DIR="/var/lib/cloudrix"
CERT_DIR="$CONF_DIR/certs"
ENV_FILE="$CONF_DIR/cloudrix.env"
INSTALL_CONF="$CONF_DIR/install.conf"
BACKUP_DIR="$DATA_DIR/backups"
BIN="/usr/local/bin/cloudrix"
MANAGER="/usr/local/bin/cloudrix-manager"
SERVICE="cloudrix"
SERVICE_USER="cloudrix"

GO_VERSION="1.24.7"
NODE_VERSION="22.22.0"

ASSUME_YES=0
FORCE=0
FOLLOW=0

# ---------- вывод ----------

if [[ -t 1 ]]; then
  C_RESET=$'\e[0m' C_BOLD=$'\e[1m' C_DIM=$'\e[2m' C_RED=$'\e[31m' C_GREEN=$'\e[32m' C_YELLOW=$'\e[33m' C_BLUE=$'\e[34m' C_CYAN=$'\e[36m'
else
  C_RESET='' C_BOLD='' C_DIM='' C_RED='' C_GREEN='' C_YELLOW='' C_BLUE='' C_CYAN=''
fi

info() { printf '%s➜%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok() { printf '%s✔%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die() {
  printf '%s✘ %s%s\n' "$C_RED" "$*" "$C_RESET" >&2
  exit 1
}
step() { printf '\n%s%s== %s ==%s\n' "$C_BOLD" "$C_CYAN" "$*" "$C_RESET"; }

# Сообщение об ошибке печатаем только в основном процессе, не в $(...) подоболочках.
on_error() {
  local line=$1
  ((BASH_SUBSHELL == 0)) || return 0
  die "Ошибка в строке $line. Установка прервана."
}
trap 'on_error $LINENO' ERR

banner() {
  printf '%s' "$C_BLUE"
  cat <<'EOF'
   ____ _                 _      _
  / ___| | ___  _   _  __| |_ __(_)_  __
 | |   | |/ _ \| | | |/ _` | '__| \ \/ /
 | |___| | (_) | |_| | (_| | |  | |>  <
  \____|_|\___/ \__,_|\__,_|_|  |_/_/\_\
EOF
  printf '%s%s  Панель управления прокси: Xray + sing-box%s\n\n' "$C_RESET" "$C_DIM" "$C_RESET"
}

# ---------- ввод ----------
# Вопросы читаются с /dev/tty, чтобы работал запуск через `curl ... | bash`.

TTY=/dev/tty

have_tty() { [[ -r $TTY ]] && { : <"$TTY"; } 2>/dev/null; }

# ask VAR "Вопрос" "значение по умолчанию"
ask() {
  local __var=$1 prompt=$2 def=${3-} reply
  if [[ -n ${!__var-} ]]; then return; fi
  if ((ASSUME_YES)) || ! have_tty; then
    printf -v "$__var" '%s' "$def"
    return
  fi
  if [[ -n $def ]]; then
    read -r -p "$(printf '%s?%s %s %s[%s]%s: ' "$C_CYAN" "$C_RESET" "$prompt" "$C_DIM" "$def" "$C_RESET")" reply <"$TTY"
  else
    read -r -p "$(printf '%s?%s %s: ' "$C_CYAN" "$C_RESET" "$prompt")" reply <"$TTY"
  fi
  printf -v "$__var" '%s' "${reply:-$def}"
}

# ask_yes_no "Вопрос" y|n  -> код возврата 0 = да
ask_yes_no() {
  local prompt=$1 def=${2:-y} reply hint
  if ((ASSUME_YES)) || ! have_tty; then [[ $def == y ]]; return; fi
  [[ $def == y ]] && hint="Д/н" || hint="д/Н"
  while true; do
    read -r -p "$(printf '%s?%s %s %s[%s]%s: ' "$C_CYAN" "$C_RESET" "$prompt" "$C_DIM" "$hint" "$C_RESET")" reply <"$TTY"
    reply=${reply:-$def}
    case ${reply,,} in
      y | yes | д | да | Д | Да | ДА) return 0 ;;
      n | no | н | нет | Н | Нет | НЕТ) return 1 ;;
    esac
  done
}

ask_password() {
  local __var=$1 p1 p2
  if [[ -n ${!__var-} ]]; then return; fi
  if ((ASSUME_YES)) || ! have_tty; then
    printf -v "$__var" '%s' "$(random_string 16)"
    GENERATED_PASSWORD=1
    return
  fi
  while true; do
    read -r -s -p "$(printf '%s?%s Пароль администратора %s(Enter — сгенерировать)%s: ' "$C_CYAN" "$C_RESET" "$C_DIM" "$C_RESET")" p1 <"$TTY"
    echo
    if [[ -z $p1 ]]; then
      printf -v "$__var" '%s' "$(random_string 16)"
      GENERATED_PASSWORD=1
      return
    fi
    if ((${#p1} < 8)); then
      warn "Пароль должен быть не короче 8 символов."
      continue
    fi
    read -r -s -p "$(printf '%s?%s Повторите пароль: ' "$C_CYAN" "$C_RESET")" p2 <"$TTY"
    echo
    if [[ $p1 != "$p2" ]]; then
      warn "Пароли не совпадают, попробуйте ещё раз."
      continue
    fi
    printf -v "$__var" '%s' "$p1"
    return
  done
}

random_string() {
  local n=${1:-16}
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$n" || true
}

# ---------- проверки системы ----------

require_root() {
  [[ $EUID -eq 0 ]] || die "Запустите от root: sudo bash install.sh"
}

detect_os() {
  [[ -f /etc/os-release ]] || die "Не удалось определить ОС."
  # В подоболочке: os-release определяет VERSION, ID и т.п., не засоряем ими скрипт.
  OS_NAME=$(
    # shellcheck disable=SC1091
    . /etc/os-release
    echo "${PRETTY_NAME:-${ID:-unknown}}"
  )
  if command -v apt-get >/dev/null; then
    PKG=apt
  elif command -v dnf >/dev/null; then
    PKG=dnf
  elif command -v yum >/dev/null; then
    PKG=yum
  else
    die "Поддерживаются Debian/Ubuntu и RHEL-подобные системы (apt, dnf, yum)."
  fi
  command -v systemctl >/dev/null || die "Нужен systemd."
}

detect_arch() {
  case $(uname -m) in
    x86_64 | amd64) ARCH=amd64 NODE_ARCH=x64 ;;
    aarch64 | arm64) ARCH=arm64 NODE_ARCH=arm64 ;;
    *) die "Архитектура $(uname -m) не поддерживается (нужна amd64 или arm64)." ;;
  esac
}

install_packages() {
  local pkgs=("$@")
  case $PKG in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq
      apt-get install -y -qq "${pkgs[@]}" >/dev/null
      ;;
    dnf) dnf install -y -q "${pkgs[@]}" >/dev/null ;;
    yum) yum install -y -q "${pkgs[@]}" >/dev/null ;;
  esac
}

ensure_deps() {
  local need=()
  for c in curl tar openssl; do command -v "$c" >/dev/null || need+=("$c"); done
  command -v xz >/dev/null || need+=(xz-utils)
  command -v socat >/dev/null || need+=(socat)
  # cron нужен acme.sh для автопродления сертификата
  if ! command -v crontab >/dev/null; then
    [[ $PKG == apt ]] && need+=(cron) || need+=(cronie)
  fi
  [[ -f /etc/ssl/certs/ca-certificates.crt || -f /etc/pki/tls/certs/ca-bundle.crt ]] || need+=(ca-certificates)
  if [[ $PKG != apt ]]; then
    need=("${need[@]/xz-utils/xz}")
  fi
  if ((${#need[@]})); then
    info "Устанавливаю пакеты: ${need[*]}"
    install_packages "${need[@]}"
  fi
}

public_ip() {
  local ip
  for url in https://api.ipify.org https://ifconfig.me https://icanhazip.com; do
    ip=$(curl -4 -fsS --max-time 5 "$url" 2>/dev/null | tr -d '[:space:]' || true)
    [[ $ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] && {
      echo "$ip"
      return
    }
  done
  hostname -I 2>/dev/null | awk '{print $1}'
}

port_in_use() {
  if command -v ss >/dev/null; then
    ss -ltnH "sport = :$1" 2>/dev/null | grep -q .
  else
    (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
  fi
}

valid_domain() {
  [[ $1 =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$ ]]
}

valid_username() {
  [[ $1 =~ ^[a-zA-Z0-9_.@-]{3,64}$ ]]
}

# ---------- получение бинарника ----------

# ---------- источник и версии ----------

# load_install_conf восстанавливает репозиторий и ветку, с которых ставили панель.
# Явно заданные CLOUDRIX_REPO / CLOUDRIX_BRANCH важнее сохранённых.
load_install_conf() {
  INSTALLED_REF=""
  if [[ -f $INSTALL_CONF ]]; then
    local repo branch ref
    repo=$(sed -n 's/^REPO=//p' "$INSTALL_CONF")
    branch=$(sed -n 's/^BRANCH=//p' "$INSTALL_CONF")
    ref=$(sed -n 's/^REF=//p' "$INSTALL_CONF")
    [[ -z $REPO ]] && REPO=$repo
    [[ -z $BRANCH ]] && BRANCH=$branch
    INSTALLED_REF=$ref
  fi
  [[ -n $REPO ]] || REPO=$DEFAULT_REPO
}

save_install_conf() {
  mkdir -p "$CONF_DIR"
  printf '# Откуда ставилась панель; используется командой update\nREPO=%s\nBRANCH=%s\nCHANNEL=%s\nREF=%s\nUPDATED=%s\n' \
    "$REPO" "$BRANCH" "$TARGET_CHANNEL" "$TARGET_REF" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$INSTALL_CONF"
}

github_api() {
  curl -fsSL --max-time 15 -H "Accept: application/vnd.github+json" "https://api.github.com/repos/$REPO$1" 2>/dev/null
}

# resolve_branch выбирает ветку по умолчанию репозитория, если она не задана.
resolve_branch() {
  [[ -n $BRANCH ]] && return
  BRANCH=$(github_api "" | sed -n 's/.*"default_branch": *"\([^"]*\)".*/\1/p' | head -n1 || true)
  [[ -n $BRANCH ]] || BRANCH=main
}

# resolve_target определяет, что ставить: последний релиз (если есть) или
# последний коммит ветки. Заполняет TARGET_CHANNEL и TARGET_REF.
resolve_target() {
  if [[ -n ${CLOUDRIX_LOCAL_BINARY-} ]]; then
    TARGET_CHANNEL=local TARGET_REF=local
    return
  fi
  if [[ -n ${CLOUDRIX_DOWNLOAD_URL-} ]]; then
    TARGET_CHANNEL=mirror TARGET_REF=$RELEASE
    return
  fi
  local tag=""
  if [[ $RELEASE == latest ]]; then
    tag=$(github_api "/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1 || true)
  else
    tag=$RELEASE
  fi
  if [[ -n $tag ]]; then
    TARGET_CHANNEL=release TARGET_REF=$tag
    return
  fi
  resolve_branch
  TARGET_CHANNEL=source
  TARGET_REF=$(curl -fsSL --max-time 15 -H "Accept: application/vnd.github.sha" \
    "https://api.github.com/repos/$REPO/commits/$BRANCH" 2>/dev/null | head -c 40 || true)
  [[ $TARGET_REF =~ ^[0-9a-f]{40}$ ]] || TARGET_REF=$BRANCH
}

short_ref() {
  local r=${1-}
  [[ $r =~ ^[0-9a-f]{40}$ ]] && r=${r:0:7}
  printf '%s' "${r:-?}"
}

download_release() {
  local url
  if [[ $TARGET_CHANNEL == mirror ]]; then
    # Своё зеркало, если GitHub недоступен с сервера.
    url=${CLOUDRIX_DOWNLOAD_URL//\{arch\}/$ARCH}
  else
    url="https://github.com/$REPO/releases/download/$TARGET_REF/cloudrix-linux-$ARCH.tar.gz"
  fi
  info "Скачиваю готовую сборку: $url"
  curl -fsSL --retry 3 -o "$WORK/cloudrix.tar.gz" "$url" 2>/dev/null || return 1
  tar -xzf "$WORK/cloudrix.tar.gz" -C "$WORK" || return 1
  [[ -x $WORK/cloudrix ]] || return 1
}

build_from_source() {
  warn "Собираю из исходников ($REPO, ветка $BRANCH, коммит $(short_ref "$TARGET_REF")). Это займёт несколько минут."
  local src="$WORK/src" tools="$WORK/tools"
  mkdir -p "$src" "$tools"

  info "Скачиваю исходники"
  curl -fsSL --retry 3 "https://github.com/$REPO/archive/$TARGET_REF.tar.gz" | tar -xz -C "$src" --strip-components=1 ||
    die "Не удалось скачать исходники $REPO (репозиторий приватный или нет сети?)."

  info "Скачиваю Go $GO_VERSION и Node.js $NODE_VERSION (только для сборки)"
  curl -fsSL --retry 3 "${CLOUDRIX_GO_URL:-https://dl.google.com/go/go$GO_VERSION.linux-$ARCH.tar.gz}" | tar -xz -C "$tools"
  curl -fsSL --retry 3 "https://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION-linux-$NODE_ARCH.tar.xz" | tar -xJ -C "$tools"
  local path="$tools/go/bin:$tools/node-v$NODE_VERSION-linux-$NODE_ARCH/bin:$PATH"
  local version
  version="git-$(short_ref "$TARGET_REF")"

  # env, а не "PATH=... cmd": так новый PATH используется и для поиска самой команды.
  info "Собираю веб-интерфейс"
  (cd "$src/web" &&
    env PATH="$path" npm ci --no-audit --no-fund --no-update-notifier --loglevel=error >/dev/null &&
    env PATH="$path" npm run build --silent >/dev/null)

  info "Собираю панель"
  (cd "$src" &&
    { env PATH="$path" GOPATH="$WORK/gopath" GOCACHE="$WORK/gocache" go mod download >/dev/null 2>&1 || true; } &&
    env PATH="$path" GOPATH="$WORK/gopath" GOCACHE="$WORK/gocache" CGO_ENABLED=0 \
      go build -trimpath -ldflags "-s -w -X github.com/k9032431-cmd/cloudrix/internal/api.Version=$version" \
      -o "$WORK/cloudrix" ./cmd/cloudrix)
  cp "$src/install.sh" "$WORK/install.sh"
}

fetch_binary() {
  WORK=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$WORK'" EXIT
  [[ -n ${TARGET_CHANNEL-} ]] || resolve_target
  case $TARGET_CHANNEL in
    local) cp "$CLOUDRIX_LOCAL_BINARY" "$WORK/cloudrix" ;;
    mirror) download_release || die "Не удалось скачать архив с зеркала." ;;
    release)
      if ! download_release; then
        warn "Релиз $TARGET_REF скачать не удалось — соберу из исходников."
        resolve_branch
        TARGET_CHANNEL=source TARGET_REF=$BRANCH
        build_from_source
      fi
      ;;
    source) build_from_source ;;
  esac
  "$WORK/cloudrix" version >/dev/null || die "Собранный бинарник не запускается."
}

install_binary() {
  install -m 0755 "$WORK/cloudrix" "$BIN"
  # Сам скрипт становится командой управления.
  if [[ -f $WORK/install.sh ]]; then
    install -m 0755 "$WORK/install.sh" "$MANAGER"
  elif [[ -f ${BASH_SOURCE[0]-} && ${BASH_SOURCE[0]} != /dev/* && ${BASH_SOURCE[0]} != /proc/* ]]; then
    # При `cloudrix-manager update` скрипт и есть $MANAGER — копировать не нужно.
    [[ ${BASH_SOURCE[0]} -ef $MANAGER ]] || install -m 0755 "${BASH_SOURCE[0]}" "$MANAGER"
  else
    # Скрипт запущен через pipe — своего файла нет, скачиваем копию.
    if curl -fsSL "https://raw.githubusercontent.com/$REPO/${TARGET_REF:-$BRANCH}/install.sh" -o "$MANAGER.tmp" 2>/dev/null; then
      install -m 0755 "$MANAGER.tmp" "$MANAGER"
      rm -f "$MANAGER.tmp"
    else
      rm -f "$MANAGER.tmp"
      warn "Не удалось скачать cloudrix-manager; управлять можно, запустив установщик снова."
    fi
  fi
  ok "Установлена версия $("$BIN" version)"
}

# ---------- сертификат ----------

issue_certificate() {
  local domain=$1 email=$2 acme="$HOME/.acme.sh/acme.sh"
  if [[ ! -x $acme ]]; then
    info "Устанавливаю acme.sh"
    curl -fsSL https://get.acme.sh | sh -s email="$email" >/dev/null || return 1
  fi
  "$acme" --set-default-ca --server letsencrypt >/dev/null

  if port_in_use 80; then
    warn "Порт 80 занят — Let's Encrypt не сможет проверить домен. Освободите порт 80 и запустите: cloudrix-manager cert"
    return 1
  fi
  info "Получаю сертификат Let's Encrypt для $domain (порт 80 должен быть открыт)"
  open_firewall 80
  local rc=0
  "$acme" --issue -d "$domain" --standalone --keylength ec-256 >"$WORK/acme.log" 2>&1 || rc=$?
  # 2 = сертификат уже есть и ещё не пора продлевать
  if ((rc != 0 && rc != 2)); then
    tail -n 15 "$WORK/acme.log" >&2
    return 1
  fi

  mkdir -p "$CERT_DIR"
  "$acme" --install-cert -d "$domain" --ecc \
    --fullchain-file "$CERT_DIR/fullchain.pem" \
    --key-file "$CERT_DIR/privkey.pem" \
    --reloadcmd "chown -R $SERVICE_USER:$SERVICE_USER $CERT_DIR; chmod 600 $CERT_DIR/privkey.pem; systemctl restart $SERVICE 2>/dev/null || true" >/dev/null
  ok "Сертификат сохранён в $CERT_DIR (продлевается автоматически)"
}

# ---------- systemd и окружение ----------

write_env() {
  umask 077
  cat >"$ENV_FILE" <<EOF
# Настройки Cloudrix. После изменения: cloudrix-manager restart
CLOUDRIX_LISTEN=:$PANEL_PORT
CLOUDRIX_DB=$DATA_DIR/cloudrix.db
CLOUDRIX_HOST=$PUBLIC_HOST
CLOUDRIX_SUB_URL=$PANEL_URL
CLOUDRIX_SUB_PATH=sub
CLOUDRIX_SUB_TITLE="$SUB_TITLE"
CLOUDRIX_SUB_UPDATE_HOURS=12
CLOUDRIX_TOKEN_TTL_HOURS=24
CLOUDRIX_JOBS_INTERVAL_SECONDS=30
EOF
  if [[ $TLS == 1 ]]; then
    cat >>"$ENV_FILE" <<EOF
CLOUDRIX_TLS_CERT=$CERT_DIR/fullchain.pem
CLOUDRIX_TLS_KEY=$CERT_DIR/privkey.pem
EOF
  fi
  chown root:"$SERVICE_USER" "$ENV_FILE"
  chmod 0640 "$ENV_FILE"
  umask 022
}

write_service() {
  cat >"/etc/systemd/system/$SERVICE.service" <<EOF
[Unit]
Description=Cloudrix panel
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
User=$SERVICE_USER
Group=$SERVICE_USER
EnvironmentFile=$ENV_FILE
ExecStart=$BIN
Restart=on-failure
RestartSec=3
LimitNOFILE=65535
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$DATA_DIR
ReadOnlyPaths=$CONF_DIR

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

ensure_user() {
  id "$SERVICE_USER" >/dev/null 2>&1 || useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
  mkdir -p "$DATA_DIR" "$CONF_DIR" "$APP_DIR"
  chown "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"
  chmod 0750 "$DATA_DIR"
  if [[ -d $CERT_DIR ]]; then
    chown -R "$SERVICE_USER:$SERVICE_USER" "$CERT_DIR"
    chmod 600 "$CERT_DIR"/privkey.pem 2>/dev/null || true
  fi
}

# run_as_service команда… — запуск CLI с теми же правами и базой, что у сервиса.
run_as_service() {
  if command -v runuser >/dev/null; then
    runuser -u "$SERVICE_USER" -- env CLOUDRIX_DB="$DATA_DIR/cloudrix.db" "$@"
  else
    su -s /bin/sh "$SERVICE_USER" -c "CLOUDRIX_DB='$DATA_DIR/cloudrix.db' $*"
  fi
}

setup_admin() {
  local out
  if out=$(CLOUDRIX_ADMIN_PASSWORD="$ADMIN_PASS" run_as_service "$BIN" admin create -u "$ADMIN_USER" 2>&1); then
    ok "Создан администратор $ADMIN_USER"
  elif [[ $out == *"already exists"* ]]; then
    CLOUDRIX_ADMIN_PASSWORD="$ADMIN_PASS" run_as_service "$BIN" admin passwd -u "$ADMIN_USER" >/dev/null
    ok "Администратор $ADMIN_USER уже был — пароль обновлён"
  else
    die "Не удалось создать администратора: $out"
  fi
}

open_firewall() {
  local port=$1
  if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
    ufw allow "$port"/tcp >/dev/null && ok "Открыт порт $port в ufw"
  elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port"/tcp >/dev/null && firewall-cmd --reload >/dev/null && ok "Открыт порт $port в firewalld"
  fi
}

wait_healthy() {
  local scheme=http
  [[ $TLS == 1 ]] && scheme=https
  for _ in $(seq 1 20); do
    if curl -fsk --noproxy '*' --max-time 2 "$scheme://127.0.0.1:$PANEL_PORT/health" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  return 1
}

# ---------- команды ----------

cmd_install() {
  require_root
  banner
  detect_os
  detect_arch
  info "Система: $OS_NAME ($ARCH)"

  load_install_conf
  if [[ -x $BIN ]] && systemctl is-enabled "$SERVICE" >/dev/null 2>&1; then
    warn "Cloudrix уже установлен ($("$BIN" version 2>/dev/null || echo '?'))."
    # С --yes переустанавливаем без вопроса: флаг передан явно.
    if ! ((ASSUME_YES)) && ! ask_yes_no "Переустановить? Пользователи и настройки сохранятся" n; then
      info "Для обновления используйте: cloudrix-manager update"
      exit 0
    fi
  fi

  step "1/4  Администратор"
  ADMIN_USER=${CLOUDRIX_ADMIN_USERNAME-}
  ADMIN_PASS=${CLOUDRIX_ADMIN_PASSWORD-}
  GENERATED_PASSWORD=0
  while true; do
    ask ADMIN_USER "Логин администратора" "admin"
    valid_username "$ADMIN_USER" && break
    warn "Логин: 3–64 символа, латиница, цифры и _ . @ -"
    ((ASSUME_YES)) && die "Неверный CLOUDRIX_ADMIN_USERNAME"
    ADMIN_USER=
  done
  ask_password ADMIN_PASS
  ((${#ADMIN_PASS} >= 8)) || die "Пароль администратора должен быть не короче 8 символов."

  step "2/4  Домен и HTTPS"
  SERVER_IP=$(public_ip)
  info "IP сервера: ${SERVER_IP:-не определён}"
  DOMAIN=${CLOUDRIX_DOMAIN-}
  EMAIL=${CLOUDRIX_EMAIL-}
  TLS=0
  if [[ -z $DOMAIN ]] && ! ((ASSUME_YES)) && have_tty; then
    if ask_yes_no "У вас есть домен для панели (например panel.example.com)?" y; then
      while true; do
        ask DOMAIN "Домен"
        valid_domain "$DOMAIN" && break
        warn "Похоже, это не домен. Пример: panel.example.com"
        DOMAIN=
      done
    fi
  fi
  if [[ -n $DOMAIN ]]; then
    valid_domain "$DOMAIN" || die "Неверный домен: $DOMAIN"
    local resolved
    resolved=$(getent ahostsv4 "$DOMAIN" 2>/dev/null | awk 'NR==1{print $1}' || true)
    if [[ -z $resolved ]]; then
      warn "Домен $DOMAIN пока не указывает ни на какой IP. Создайте A-запись → $SERVER_IP"
    elif [[ -n $SERVER_IP && $resolved != "$SERVER_IP" ]]; then
      warn "Домен $DOMAIN указывает на $resolved, а у сервера IP $SERVER_IP."
      warn "Если домен за Cloudflare — временно выключите проксирование (серое облако) для выпуска сертификата."
    else
      ok "DNS в порядке: $DOMAIN → $resolved"
    fi
    if ask_yes_no "Выпустить бесплатный SSL-сертификат Let's Encrypt?" y; then
      ask EMAIL "Email для Let's Encrypt (уведомления о продлении)" "admin@$DOMAIN"
      WANT_CERT=1
    else
      WANT_CERT=0
      if [[ -f $CERT_DIR/fullchain.pem && -f $CERT_DIR/privkey.pem ]]; then
        TLS=1
        ok "Использую существующий сертификат из $CERT_DIR"
      fi
    fi
    PUBLIC_HOST=$DOMAIN
  else
    WANT_CERT=0
    PUBLIC_HOST=${SERVER_IP:-127.0.0.1}
    warn "Без домена панель будет работать по HTTP на IP-адресе. Домен можно добавить позже: cloudrix-manager cert"
  fi

  step "3/4  Порт и подписки"
  local def_port=8000
  [[ -n $DOMAIN ]] && def_port=2053
  PANEL_PORT=${CLOUDRIX_PORT-}
  while true; do
    ask PANEL_PORT "Порт панели (443 лучше оставить для VLESS Reality)" "$def_port"
    if [[ $PANEL_PORT =~ ^[0-9]+$ ]] && ((PANEL_PORT >= 1 && PANEL_PORT <= 65535)); then
      if port_in_use "$PANEL_PORT" && ! systemctl is-active --quiet "$SERVICE"; then
        warn "Порт $PANEL_PORT уже занят другой программой."
        ((ASSUME_YES)) && die "Порт $PANEL_PORT занят"
        PANEL_PORT=
        continue
      fi
      break
    fi
    warn "Порт должен быть числом от 1 до 65535."
    ((ASSUME_YES)) && die "Неверный CLOUDRIX_PORT"
    PANEL_PORT=
  done
  SUB_TITLE=${CLOUDRIX_SUB_TITLE-}
  ask SUB_TITLE "Название подписки в приложениях клиентов" "Cloudrix"
  SUB_TITLE=${SUB_TITLE//[\"\\\$\`]/}

  step "4/4  Установка"
  ensure_deps
  fetch_binary
  ensure_user
  install_binary

  if [[ ${WANT_CERT:-0} == 1 ]]; then
    systemctl stop "$SERVICE" 2>/dev/null || true
    if issue_certificate "$DOMAIN" "$EMAIL"; then
      TLS=1
    elif [[ -f $CERT_DIR/fullchain.pem && -f $CERT_DIR/privkey.pem ]]; then
      TLS=1
      warn "Новый сертификат получить не удалось — использую уже имеющийся из $CERT_DIR."
    else
      warn "Сертификат получить не удалось — панель запустится по HTTP. Повторить: cloudrix-manager cert"
    fi
  fi
  ensure_user

  if [[ $TLS == 1 ]]; then
    PANEL_URL="https://$DOMAIN"
  else
    PANEL_URL="http://$PUBLIC_HOST"
  fi
  if ! { [[ $TLS == 1 && $PANEL_PORT == 443 ]] || [[ $TLS == 0 && $PANEL_PORT == 80 ]]; }; then
    PANEL_URL="$PANEL_URL:$PANEL_PORT"
  fi

  write_env
  write_service
  save_install_conf
  setup_admin
  open_firewall "$PANEL_PORT"
  systemctl enable --now "$SERVICE" >/dev/null 2>&1
  systemctl restart "$SERVICE"

  if wait_healthy; then
    ok "Панель запущена"
  else
    warn "Панель не ответила. Посмотрите логи: cloudrix-manager logs"
  fi
  print_summary
}

print_summary() {
  local creds="$CONF_DIR/admin-credentials.txt"
  if ((GENERATED_PASSWORD)); then
    umask 077
    printf 'url=%s\nusername=%s\npassword=%s\n' "$PANEL_URL" "$ADMIN_USER" "$ADMIN_PASS" >"$creds"
    umask 022
  fi
  printf '\n%s%s╔══════════════════════════════════════════════╗%s\n' "$C_BOLD" "$C_GREEN" "$C_RESET"
  printf '%s%s║          Cloudrix успешно установлен         ║%s\n' "$C_BOLD" "$C_GREEN" "$C_RESET"
  printf '%s%s╚══════════════════════════════════════════════╝%s\n\n' "$C_BOLD" "$C_GREEN" "$C_RESET"
  printf '  %sПанель:%s  %s\n' "$C_BOLD" "$C_RESET" "$PANEL_URL"
  printf '  %sЛогин:%s   %s\n' "$C_BOLD" "$C_RESET" "$ADMIN_USER"
  if ((GENERATED_PASSWORD)); then
    printf '  %sПароль:%s  %s%s%s  %s(сохранён в %s)%s\n' "$C_BOLD" "$C_RESET" "$C_YELLOW" "$ADMIN_PASS" "$C_RESET" "$C_DIM" "$creds" "$C_RESET"
  else
    printf '  %sПароль:%s  тот, что вы ввели\n' "$C_BOLD" "$C_RESET"
  fi
  [[ $TLS == 1 ]] && printf '  %sСертификат:%s %s (подходит и для инбаундов Hysteria2/TUIC/TLS)\n' "$C_BOLD" "$C_RESET" "$CERT_DIR/fullchain.pem"
  printf '\n  Меню управления (обновление, логи, бэкапы): %scloudrix-manager%s\n\n' "$C_CYAN" "$C_RESET"
}

load_env() {
  [[ -f $ENV_FILE ]] || die "Cloudrix не установлен ($ENV_FILE не найден)."
  # shellcheck disable=SC1090
  set -a && . "$ENV_FILE" && set +a
}

panel_port() { printf '%s' "${CLOUDRIX_LISTEN##*:}"; }

panel_tls() { [[ -n ${CLOUDRIX_TLS_CERT-} ]] && echo 1 || echo 0; }

# make_backup [метка] [бинарник] — согласованная копия базы без остановки
# панели. Если бинарник не умеет `backup` (старые версии), база копируется
# при остановленной панели.
make_backup() {
  local label=${1:-manual} bin=${2:-$BIN} file
  mkdir -p "$BACKUP_DIR"
  chown "$SERVICE_USER:$SERVICE_USER" "$BACKUP_DIR"
  file="$BACKUP_DIR/cloudrix-$(date +%Y%m%d-%H%M%S)-$label.db"
  if ! run_as_service "$bin" backup "$file" >/dev/null 2>&1; then
    local was_active=0
    systemctl is-active --quiet "$SERVICE" && was_active=1
    systemctl stop "$SERVICE" 2>/dev/null || true
    local rc=0
    cp -p "$DATA_DIR/cloudrix.db" "$file" || rc=1
    [[ -f $DATA_DIR/cloudrix.db-wal ]] && { cp -p "$DATA_DIR/cloudrix.db-wal" "$file-wal" || rc=1; }
    ((was_active)) && systemctl start "$SERVICE"
    ((rc == 0)) || return 1
  fi
  # Храним 10 последних копий.
  local old
  find "$BACKUP_DIR" -maxdepth 1 -name 'cloudrix-*.db' -printf '%T@ %p\n' 2>/dev/null |
    sort -rn | tail -n +11 | cut -d' ' -f2- | while read -r old; do rm -f "$old" "$old-wal"; done
  BACKUP_FILE=$file
}

# self_update загружает свежую версию этого скрипта и перезапускает команду ею,
# чтобы новые пункты меню и исправления установщика приходили вместе с панелью.
self_update() {
  [[ -z ${CLOUDRIX_SELF_UPDATED-} && $TARGET_CHANNEL != local && $TARGET_CHANNEL != mirror ]] || return 0
  local tmp self=${BASH_SOURCE[0]-}
  tmp=$(mktemp)
  if curl -fsSL --max-time 20 "https://raw.githubusercontent.com/$REPO/$TARGET_REF/install.sh" -o "$tmp" 2>/dev/null &&
    bash -n "$tmp" 2>/dev/null && ! { [[ -f $self ]] && cmp -s "$tmp" "$self"; }; then
    info "Обновляю сам менеджер"
    local args=(update)
    ((ASSUME_YES)) && args+=(--yes)
    ((FORCE)) && args+=(--force)
    exec env CLOUDRIX_SELF_UPDATED=1 CLOUDRIX_REPO="$REPO" CLOUDRIX_BRANCH="$BRANCH" \
      CLOUDRIX_TARGET="$TARGET_CHANNEL:$TARGET_REF" bash "$tmp" "${args[@]}"
  fi
  rm -f "$tmp"
}

cmd_update() {
  require_root
  detect_os
  detect_arch
  load_env
  load_install_conf
  local current
  current=$("$BIN" version 2>/dev/null || echo '?')

  if [[ -n ${CLOUDRIX_TARGET-} ]]; then
    TARGET_CHANNEL=${CLOUDRIX_TARGET%%:*} TARGET_REF=${CLOUDRIX_TARGET#*:}
  else
    info "Проверяю обновления ($REPO${BRANCH:+, ветка $BRANCH})"
    resolve_target
  fi
  info "Установлена: $current"
  info "Доступна:    $(short_ref "$TARGET_REF")${TARGET_CHANNEL:+ ($TARGET_CHANNEL)}"

  if [[ $TARGET_CHANNEL != local && $TARGET_CHANNEL != mirror && -n $INSTALLED_REF && $INSTALLED_REF == "$TARGET_REF" ]] && ! ((FORCE)); then
    ok "У вас уже последняя версия."
    if ((ASSUME_YES)) || ! ask_yes_no "Всё равно переустановить эту версию?" n; then
      return 0
    fi
  fi
  self_update

  ensure_deps
  fetch_binary

  # Бэкап делает уже новая версия: у старых может не быть команды backup.
  chmod 0755 "$WORK" "$WORK/cloudrix"
  if make_backup "before-update" "$WORK/cloudrix"; then
    ok "Резервная копия базы: $BACKUP_FILE"
  else
    ask_yes_no "Не удалось сделать резервную копию базы. Продолжить без неё?" n || die "Обновление отменено."
  fi

  cp -p "$BIN" "$BIN.prev"
  install_binary
  systemctl restart "$SERVICE"
  PANEL_PORT=$(panel_port) TLS=$(panel_tls)
  if ! wait_healthy; then
    warn "Новая версия не запустилась — возвращаю предыдущую."
    mv -f "$BIN.prev" "$BIN"
    systemctl restart "$SERVICE"
    die "Обновление откатено. Посмотрите логи: cloudrix-manager logs"
  fi
  rm -f "$BIN.prev"
  save_install_conf
  ok "Панель обновлена: $current → $("$BIN" version)"
}

cmd_backup() {
  require_root
  load_env
  make_backup manual || die "Не удалось создать резервную копию."
  ok "Резервная копия: $BACKUP_FILE"
}

cmd_restore() {
  require_root
  load_env
  local files=() i choice
  mapfile -t files < <(find "$BACKUP_DIR" -maxdepth 1 -name 'cloudrix-*.db' -printf '%T@ %p\n' 2>/dev/null | sort -rn | cut -d' ' -f2-)
  ((${#files[@]})) || die "Резервных копий нет ($BACKUP_DIR)."
  echo
  for i in "${!files[@]}"; do
    printf '  %s%2d)%s %s  %s(%s)%s\n' "$C_BOLD" $((i + 1)) "$C_RESET" "$(basename "${files[$i]}")" "$C_DIM" "$(du -h "${files[$i]}" | cut -f1)" "$C_RESET"
  done
  echo
  choice=${1-}
  ask choice "Номер копии для восстановления"
  if ! [[ $choice =~ ^[0-9]+$ ]] || ((choice < 1 || choice > ${#files[@]})); then
    die "Нет такого номера."
  fi
  local file=${files[$((choice - 1))]}
  warn "Текущая база будет заменена копией $(basename "$file"). Перед этим сохраню её копию."
  ask_yes_no "Восстановить?" n || return 0
  make_backup "before-restore" && ok "Текущая база сохранена: $BACKUP_FILE"
  systemctl stop "$SERVICE"
  rm -f "$DATA_DIR/cloudrix.db-wal" "$DATA_DIR/cloudrix.db-shm"
  install -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0640 "$file" "$DATA_DIR/cloudrix.db"
  if [[ -f $file-wal ]]; then
    install -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0640 "$file-wal" "$DATA_DIR/cloudrix.db-wal"
  fi
  systemctl start "$SERVICE"
  ok "База восстановлена из $(basename "$file")"
}

cmd_info() {
  load_env
  load_install_conf
  local state
  state=$(systemctl is-active "$SERVICE" 2>/dev/null || true)
  printf '\n  %sПанель:%s     %s\n' "$C_BOLD" "$C_RESET" "${CLOUDRIX_SUB_URL:-?}"
  printf '  %sСостояние:%s  %s\n' "$C_BOLD" "$C_RESET" "$([[ $state == active ]] && printf '%sработает%s' "$C_GREEN" "$C_RESET" || printf '%s%s%s' "$C_RED" "${state:-остановлена}" "$C_RESET")"
  printf '  %sВерсия:%s     %s\n' "$C_BOLD" "$C_RESET" "$("$BIN" version 2>/dev/null || echo '?')"
  printf '  %sИсточник:%s   %s%s\n' "$C_BOLD" "$C_RESET" "$REPO" "${BRANCH:+ (ветка $BRANCH)}"
  printf '  %sHTTPS:%s      %s\n' "$C_BOLD" "$C_RESET" "$([[ $(panel_tls) == 1 ]] && echo "да ($CERT_DIR)" || echo нет)"
  printf '  %sДанные:%s     %s, настройки %s\n' "$C_BOLD" "$C_RESET" "$DATA_DIR" "$ENV_FILE"
  [[ -f $CONF_DIR/admin-credentials.txt ]] && printf '  %sПароль:%s     сгенерированный пароль лежит в %s\n' "$C_BOLD" "$C_RESET" "$CONF_DIR/admin-credentials.txt"
  echo
}

cmd_logs() {
  if ((FOLLOW)); then
    journalctl -u "$SERVICE" -f -n 100
  else
    journalctl -u "$SERVICE" -n 60 --no-pager
  fi
}

# ---------- меню ----------

menu() {
  require_root
  [[ -f $ENV_FILE ]] || {
    warn "Панель ещё не установлена."
    ask_yes_no "Установить сейчас?" y && cmd_install
    return
  }
  have_tty || die "Меню работает только в терминале. Команды: $(basename "$0") --help"
  local choice
  while true; do
    clear 2>/dev/null || true
    banner
    (cmd_info) || true
    printf '  %s1)%s Обновить панель\n' "$C_BOLD" "$C_RESET"
    printf '  %s2)%s Перезапустить\n' "$C_BOLD" "$C_RESET"
    printf '  %s3)%s Логи (последние строки)\n' "$C_BOLD" "$C_RESET"
    printf '  %s4)%s Логи в реальном времени %s(выход — Ctrl+C)%s\n' "$C_BOLD" "$C_RESET" "$C_DIM" "$C_RESET"
    printf '  %s5)%s Сбросить пароль / добавить администратора\n' "$C_BOLD" "$C_RESET"
    printf '  %s6)%s Домен и HTTPS-сертификат\n' "$C_BOLD" "$C_RESET"
    printf '  %s7)%s Сделать резервную копию базы\n' "$C_BOLD" "$C_RESET"
    printf '  %s8)%s Восстановить базу из копии\n' "$C_BOLD" "$C_RESET"
    printf '  %s9)%s Удалить панель\n' "$C_BOLD" "$C_RESET"
    printf '  %s0)%s Выход\n\n' "$C_BOLD" "$C_RESET"
    read -r -p "$(printf '%s?%s Выберите пункт: ' "$C_CYAN" "$C_RESET")" choice <"$TTY" || return 0
    case $choice in
      1) run_menu_action cmd_update ;;
      2) run_menu_action bash -c "systemctl restart $SERVICE && echo '✔ Перезапущено'" ;;
      3) run_menu_action cmd_logs ;;
      4) FOLLOW=1 run_menu_action cmd_logs ;;
      5) run_menu_action cmd_admin ;;
      6) run_menu_action cmd_cert ;;
      7) run_menu_action cmd_backup ;;
      8) run_menu_action cmd_restore ;;
      9)
        run_menu_action cmd_uninstall
        [[ -f $ENV_FILE ]] || return 0
        ;;
      0 | q | exit | "") return 0 ;;
      *) warn "Нет такого пункта" ;;
    esac
  done
}

# run_menu_action запускает пункт в подоболочке: ошибка не закрывает меню.
run_menu_action() {
  echo
  local rc=0
  # Ctrl+C прерывает только сам пункт (например, логи), а не меню.
  trap ':' INT
  ("$@") || rc=$?
  trap - INT
  ((rc == 0 || rc == 130)) || warn "Действие завершилось с ошибкой (код $rc)."
  read -r -p "$(printf '\n%sНажмите Enter, чтобы вернуться в меню…%s' "$C_DIM" "$C_RESET")" _ <"$TTY" || true
}

cmd_admin() {
  require_root
  load_env
  local user=${1-} pass=${CLOUDRIX_ADMIN_PASSWORD-}
  ask user "Логин администратора, которому сбросить пароль (или новый логин)" "admin"
  valid_username "$user" || die "Неверный логин"
  GENERATED_PASSWORD=0
  ask_password pass
  ADMIN_USER=$user ADMIN_PASS=$pass setup_admin
  ((GENERATED_PASSWORD)) && printf '  Новый пароль: %s%s%s\n' "$C_YELLOW" "$pass" "$C_RESET"
  return 0
}

cmd_cert() {
  require_root
  detect_os
  load_env
  local domain=${1-} email=${CLOUDRIX_EMAIL-}
  ask domain "Домен панели"
  valid_domain "$domain" || die "Неверный домен"
  ask email "Email для Let's Encrypt" "admin@$domain"
  ensure_deps
  systemctl stop "$SERVICE" || true
  if ! issue_certificate "$domain" "$email"; then
    systemctl start "$SERVICE"
    die "Не удалось получить сертификат. Проверьте, что домен указывает на сервер и порт 80 открыт."
  fi
  local port=${CLOUDRIX_LISTEN##*:} url="https://$domain"
  [[ $port != 443 ]] && url="$url:$port"
  sed -i '/^CLOUDRIX_TLS_CERT=/d; /^CLOUDRIX_TLS_KEY=/d; /^CLOUDRIX_HOST=/d; /^CLOUDRIX_SUB_URL=/d' "$ENV_FILE"
  printf 'CLOUDRIX_HOST=%s\nCLOUDRIX_SUB_URL=%s\nCLOUDRIX_TLS_CERT=%s\nCLOUDRIX_TLS_KEY=%s\n' \
    "$domain" "$url" "$CERT_DIR/fullchain.pem" "$CERT_DIR/privkey.pem" >>"$ENV_FILE"
  ensure_user
  systemctl start "$SERVICE"
  ok "HTTPS включён: $url"
}

cmd_uninstall() {
  require_root
  warn "Будут удалены панель, сервис и настройки."
  ask_yes_no "Продолжить?" n || exit 0
  local keep=0
  ask_yes_no "Сохранить базу данных ($DATA_DIR)?" y && keep=1
  systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$SERVICE.service" "$BIN"
  systemctl daemon-reload
  if [[ -x $HOME/.acme.sh/acme.sh && -f $ENV_FILE ]]; then
    local d
    d=$(sed -n 's/^CLOUDRIX_HOST=//p' "$ENV_FILE")
    if [[ -n $d ]]; then
      "$HOME/.acme.sh/acme.sh" --remove -d "$d" --ecc >/dev/null 2>&1 || true
    fi
  fi
  rm -rf "$CONF_DIR" "$APP_DIR"
  if ((keep)); then
    ok "База сохранена в $DATA_DIR"
  else
    rm -rf "$DATA_DIR"
    userdel "$SERVICE_USER" 2>/dev/null || true
  fi
  rm -f "$MANAGER"
  ok "Cloudrix удалён"
}

usage() {
  cat <<EOF
Использование: $(basename "$0") [команда] [--yes]

  (без команды)  меню управления — если панель уже установлена
  install        установить или переустановить панель
  update         обновить панель до последней версии (с бэкапом и откатом)
  info           адрес, версия и состояние панели
  status         состояние сервиса systemd
  restart        перезапустить панель
  logs [-f]      логи (с -f — в реальном времени)
  admin          создать администратора или сбросить ему пароль
  cert           выпустить SSL-сертификат и включить HTTPS
  backup         сделать резервную копию базы
  restore        восстановить базу из резервной копии
  uninstall      удалить панель

  --yes          не задавать вопросов (значения из CLOUDRIX_* переменных)
  --force        для update: переустановить, даже если версия последняя
EOF
}

main() {
  local cmd="" args=()
  ASSUME_YES=0 FORCE=0 FOLLOW=0
  for a in "$@"; do
    case $a in
      -y | --yes) ASSUME_YES=1 ;;
      --force) FORCE=1 ;;
      -f | --follow) FOLLOW=1 ;;
      -h | --help | help)
        usage
        exit 0
        ;;
      *) args+=("$a") ;;
    esac
  done
  ((${#args[@]})) && cmd=${args[0]} && args=("${args[@]:1}")
  if [[ -z $cmd ]]; then
    # Установленная панель + терминал → меню; иначе (curl | bash на чистом сервере) → установка.
    if [[ -f $ENV_FILE ]] && ! ((ASSUME_YES)) && have_tty; then
      cmd=menu
    else
      cmd=install
    fi
  fi
  case $cmd in
    menu) menu ;;
    install) cmd_install ;;
    update | upgrade) cmd_update ;;
    info) cmd_info ;;
    status) systemctl status "$SERVICE" --no-pager ;;
    restart)
      require_root
      systemctl restart "$SERVICE" && ok "Перезапущено"
      ;;
    logs) cmd_logs ;;
    admin) cmd_admin "${args[@]}" ;;
    cert) cmd_cert "${args[@]}" ;;
    backup) cmd_backup ;;
    restore) cmd_restore "${args[@]}" ;;
    uninstall | remove) cmd_uninstall ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
