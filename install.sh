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

# Нода: агент Cloudrix + ядра Xray и sing-box. VPN раздают только ноды.
NODE_CONF_DIR="/etc/cloudrix-node"
NODE_ENV="$NODE_CONF_DIR/node.env"
NODE_DATA_DIR="/var/lib/cloudrix-node"
NODE_LIB="/usr/local/lib/cloudrix-node"
NODE_SERVICE="cloudrix-node"
# если узнать последнюю версию sing-box не получилось
SINGBOX_FALLBACK_VERSION="1.12.12"

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

# wait_apt_lock: на свежем сервере Ubuntu часто сама ставит обновления
# (unattended-upgrades) и держит блокировку apt. Ждём и показываем, кого ждём.
wait_apt_lock() {
  command -v fuser >/dev/null || return 0
  local waited=0
  while fuser /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/lib/apt/lists/lock >/dev/null 2>&1; do
    if ((waited == 0)); then
      info "Ubuntu сейчас сама устанавливает обновления (apt занят) — жду, обычно 1–10 минут"
    fi
    if ((waited >= 900)); then
      die "apt занят уже 15 минут. Проверьте: ps aux | grep -E 'apt|dpkg'"
    fi
    printf '  ⏳ %s' "$(elapsed "$waited")"
    sleep 15
    waited=$((waited + 15))
  done
  ((waited)) && echo
  return 0
}

install_packages() {
  local pkgs=("$@")
  case $PKG in
    apt)
      # needrestart на Ubuntu 22.04+ иначе молча ждёт ответа на невидимый вопрос
      export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a NEEDRESTART_SUSPEND=1
      wait_apt_lock
      apt-get -o DPkg::Lock::Timeout=600 update -qq
      apt-get -o DPkg::Lock::Timeout=600 install -y -qq "${pkgs[@]}" >/dev/null
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

# download_edge берёт готовую сборку нужного коммита, которую GitHub Actions
# публикует в пре-релиз "edge-<ветка>" после каждого пуша. Если сборка этого
# коммита ещё не готова, немного ждёт: обычно GitHub собирает 1–2 минуты.
download_edge() {
  [[ $TARGET_REF =~ ^[0-9a-f]{40}$ ]] || return 1
  local tag="edge-${BRANCH//\//-}" ref waited=0
  local url="https://github.com/$REPO/releases/download/$tag/cloudrix-linux-$ARCH.tar.gz"
  # Пре-релиза нет вовсе (Actions выключены) — сразу собираем сами.
  github_api "/releases/tags/$tag" >/dev/null || return 1
  local deadline=$((SECONDS + ${CLOUDRIX_EDGE_WAIT:-300}))
  while true; do
    rm -rf "$WORK/edge" && mkdir -p "$WORK/edge"
    if curl -fsSL --retry 2 -o "$WORK/edge.tar.gz" "$url" 2>/dev/null &&
      tar -xzf "$WORK/edge.tar.gz" -C "$WORK/edge" 2>/dev/null; then
      ref=$(cat "$WORK/edge/REF" 2>/dev/null || true)
      if [[ $ref == "$TARGET_REF" && -x $WORK/edge/cloudrix ]]; then
        ((waited)) && echo
        mv "$WORK/edge/cloudrix" "$WORK/cloudrix"
        [[ -f $WORK/edge/install.sh ]] && mv "$WORK/edge/install.sh" "$WORK/install.sh"
        ok "Скачана готовая сборка $(short_ref "$TARGET_REF") с GitHub"
        return 0
      fi
    fi
    if ((SECONDS >= deadline)); then
      ((waited)) && echo
      warn "Готовая сборка $(short_ref "$TARGET_REF") так и не появилась на GitHub."
      return 1
    fi
    if ((!waited)); then
      info "GitHub ещё собирает версию $(short_ref "$TARGET_REF") — жду готовую сборку (обычно 1–2 минуты)"
      waited=1
    fi
    printf '\r  ⏳ ждём %s ' "$(elapsed "$((SECONDS - deadline + ${CLOUDRIX_EDGE_WAIT:-300}))")"
    sleep 15
  done
}

elapsed() { printf '%d:%02d' $(($1 / 60)) $(($1 % 60)); }

# run_with_progress "текст" команда… — запускает долгую команду и показывает
# таймер, чтобы было видно, что установка не зависла.
run_with_progress() {
  local label=$1 start=$SECONDS pid rc=0
  shift
  "$@" &
  pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    printf '\r%s➜%s %s %s%s%s ' "$C_BLUE" "$C_RESET" "$label" "$C_DIM" "$(elapsed $((SECONDS - start)))" "$C_RESET"
    sleep 2
  done
  wait "$pid" || rc=$?
  printf '\r%s➜%s %s %s%s%s\n' "$C_BLUE" "$C_RESET" "$label" "$C_DIM" "$(elapsed $((SECONDS - start)))" "$C_RESET"
  return "$rc"
}

# ensure_build_memory добавляет временный swap, если памяти мало: сборка
# встроенной SQLite требует около 2 ГБ, на маленьких VPS без swap её убивает система.
SWAPFILE=""
ensure_build_memory() {
  local mem_mb swap_mb
  mem_mb=$(awk '/^MemTotal:/ {print int($2/1024)}' /proc/meminfo)
  swap_mb=$(awk '/^SwapFree:/ {print int($2/1024)}' /proc/meminfo)
  BUILD_PROCS=$(nproc 2>/dev/null || echo 1)
  if ((mem_mb < 3000)); then
    BUILD_PROCS=1 # меньше параллельных компиляций — меньше пиковая память
  fi
  if ((mem_mb + swap_mb >= 2500)); then
    return 0
  fi
  local free_mb
  free_mb=$(df -Pm /var/tmp | awk 'NR==2 {print $4}')
  if ((free_mb < 3000)); then
    warn "Мало памяти (${mem_mb} МБ) и места на диске для временного swap — сборка может не пройти."
    return 0
  fi
  SWAPFILE=/var/tmp/cloudrix-build.swap
  info "Памяти ${mem_mb} МБ — временно добавляю 2 ГБ swap на время сборки"
  local how
  # fallocate быстрее, но на части файловых систем swap из такого файла не включается — тогда dd.
  for how in fallocate dd; do
    rm -f "$SWAPFILE"
    if [[ $how == fallocate ]]; then
      fallocate -l 2G "$SWAPFILE" 2>/dev/null || continue
    else
      dd if=/dev/zero of="$SWAPFILE" bs=1M count=2048 status=none 2>/dev/null || continue
    fi
    chmod 600 "$SWAPFILE"
    if mkswap "$SWAPFILE" >/dev/null 2>&1 && swapon "$SWAPFILE" 2>/dev/null; then
      return 0
    fi
  done
  warn "Не удалось включить временный swap — продолжаю без него."
  rm -f "$SWAPFILE"
  SWAPFILE=""
}

cleanup_work() {
  if [[ -n $SWAPFILE ]]; then
    swapoff "$SWAPFILE" 2>/dev/null || true
    rm -f "$SWAPFILE"
  fi
  [[ -n ${WORK-} ]] && rm -rf "$WORK"
  return 0
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

  ensure_build_memory

  # env, а не "PATH=... cmd": так новый PATH используется и для поиска самой команды.
  # shellcheck disable=SC2016 # переменные раскрывает вложенный bash
  run_with_progress "Собираю веб-интерфейс" bash -c '
    cd "$1/web" &&
      env PATH="$2" npm ci --no-audit --no-fund --no-update-notifier --loglevel=error >/dev/null &&
      env PATH="$2" npm run build --silent >/dev/null' _ "$src" "$path"

  info "Собираю панель — на слабом сервере это может занять 5–15 минут"
  # shellcheck disable=SC2016
  run_with_progress "Компиляция" bash -c '
    cd "$1" || exit 1
    export PATH="$2" GOPATH="$3/gopath" GOCACHE="$3/gocache" CGO_ENABLED=0
    go mod download >/dev/null 2>&1 || true
    go build -p "$4" -trimpath -ldflags "-s -w -X github.com/k9032431-cmd/cloudrix/internal/api.Version=$5" \
      -o "$3/cloudrix" ./cmd/cloudrix' _ "$src" "$path" "$WORK" "$BUILD_PROCS" "$version" ||
    die "Сборка не удалась. Если на сервере мало памяти, убедитесь, что есть хотя бы 2 ГБ RAM+swap."
  cp "$src/install.sh" "$WORK/install.sh"
}

fetch_binary() {
  WORK=$(mktemp -d)
  trap cleanup_work EXIT
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
    source) download_edge || build_from_source ;;
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
# откуда ставить ноды (команда установки ноды в панели)
CLOUDRIX_REPO=$REPO
CLOUDRIX_BRANCH=${BRANCH:-main}
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

# self_update загружает свежую версию этого скрипта и перезапускает команду ею
# (update или node-update), чтобы новые пункты меню и исправления установщика
# приходили вместе с обновлением панели или ноды.
self_update() {
  local cmd=${1:-update}
  [[ -z ${CLOUDRIX_SELF_UPDATED-} && $TARGET_CHANNEL != local && $TARGET_CHANNEL != mirror ]] || return 0
  local tmp self=${BASH_SOURCE[0]-}
  tmp=$(mktemp)
  if curl -fsSL --max-time 20 "https://raw.githubusercontent.com/$REPO/$TARGET_REF/install.sh" -o "$tmp" 2>/dev/null &&
    bash -n "$tmp" 2>/dev/null && ! { [[ -f $self ]] && cmp -s "$tmp" "$self"; }; then
    info "Обновляю сам менеджер"
    local args=("$cmd")
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
  # Старые установки не знали, откуда ставить ноды: дописываем.
  grep -q '^CLOUDRIX_REPO=' "$ENV_FILE" || printf 'CLOUDRIX_REPO=%s\n' "$REPO" >>"$ENV_FILE"
  grep -q '^CLOUDRIX_BRANCH=' "$ENV_FILE" || printf 'CLOUDRIX_BRANCH=%s\n' "${BRANCH:-main}" >>"$ENV_FILE"
  systemctl restart "$SERVICE"
  PANEL_PORT=$(panel_port) TLS=$(panel_tls)
  if ! wait_healthy; then
    warn "Новая версия не запустилась — возвращаю предыдущую."
    mv -f "$BIN.prev" "$BIN"
    systemctl restart "$SERVICE"
    die "Обновление отменено, работает прежняя версия. Причина — в логах: cloudrix-manager logs"
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

# ---------- нода ----------
# На сервере-ноде работает агент Cloudrix (тот же бинарник, `cloudrix node run`)
# и ядра Xray и sing-box. Агент сам обращается к панели, поэтому на ноде не
# нужно открывать порт управления — только порты инбаундов.

# latest_tag owner/repo печатает последний тег релиза без "v" (через редирект
# GitHub, без API и его лимитов).
latest_tag() {
  curl -fsSI --max-time 15 "https://github.com/$1/releases/latest" 2>/dev/null |
    sed -n 's#^[Ll]ocation: .*/tag/v\{0,1\}\([^[:space:]]*\).*#\1#p' | tr -d '\r' | tail -n1 || true
}

ensure_node_deps() {
  local need=()
  for c in curl tar unzip openssl; do command -v "$c" >/dev/null || need+=("$c"); done
  # xz нужен, если готовой сборки нет и агент собирается из исходников
  command -v xz >/dev/null || { [[ $PKG == apt ]] && need+=(xz-utils) || need+=(xz); }
  [[ -f /etc/ssl/certs/ca-certificates.crt || -f /etc/pki/tls/certs/ca-bundle.crt ]] || need+=(ca-certificates)
  if ((${#need[@]})); then
    info "Устанавливаю пакеты: ${need[*]}"
    install_packages "${need[@]}"
  fi
}

install_cores() {
  local tmp xarch sarch v url
  tmp=$(mktemp -d)
  case $ARCH in
    amd64) xarch=64 sarch=amd64 ;;
    arm64) xarch=arm64-v8a sarch=arm64 ;;
  esac
  mkdir -p "$NODE_LIB/xray"

  info "Скачиваю Xray"
  url=${CLOUDRIX_XRAY_URL:-https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-$xarch.zip}
  curl -fsSL --retry 3 -o "$tmp/xray.zip" "$url" || die "Не удалось скачать Xray ($url)."
  unzip -oq "$tmp/xray.zip" -d "$tmp/xray" || die "Архив Xray повреждён."
  install -m 0755 "$tmp/xray/xray" "$NODE_LIB/xray/xray"
  cp -f "$tmp/xray/"*.dat "$NODE_LIB/xray/" 2>/dev/null || true

  info "Скачиваю sing-box"
  v=${CLOUDRIX_SINGBOX_VERSION:-$(latest_tag SagerNet/sing-box)}
  [[ $v =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || v=$SINGBOX_FALLBACK_VERSION
  url=${CLOUDRIX_SINGBOX_URL:-https://github.com/SagerNet/sing-box/releases/download/v$v/sing-box-$v-linux-$sarch.tar.gz}
  curl -fsSL --retry 3 "$url" | tar -xz -C "$tmp" || die "Не удалось скачать sing-box ($url)."
  install -m 0755 "$tmp"/sing-box-*/sing-box "$NODE_LIB/sing-box"
  rm -rf "$tmp"
  ok "Ядра: $("$NODE_LIB/xray/xray" version | head -n1 | awk '{print $1, $2}'), $("$NODE_LIB/sing-box" version | head -n1)"
}

write_node_env() {
  mkdir -p "$NODE_CONF_DIR" "$NODE_DATA_DIR"
  chmod 0700 "$NODE_CONF_DIR" "$NODE_DATA_DIR"
  umask 077
  cat >"$NODE_ENV" <<EOF
# Нода Cloudrix. После изменения: systemctl restart $NODE_SERVICE
CLOUDRIX_PANEL_URL=$NODE_PANEL
CLOUDRIX_NODE_ID=$NODE_ID
CLOUDRIX_NODE_SECRET=$NODE_SECRET
CLOUDRIX_NODE_DIR=$NODE_DATA_DIR
CLOUDRIX_XRAY_BIN=$NODE_LIB/xray/xray
CLOUDRIX_SINGBOX_BIN=$NODE_LIB/sing-box
XRAY_LOCATION_ASSET=$NODE_LIB/xray
EOF
  umask 022
}

write_node_service() {
  cat >"/etc/systemd/system/$NODE_SERVICE.service" <<EOF
[Unit]
Description=Cloudrix node (Xray + sing-box)
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=$NODE_ENV
ExecStart=$BIN node run
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

# tune_network включает BBR: заметно быстрее на дальних и нестабильных каналах.
tune_network() {
  cat >/etc/sysctl.d/99-cloudrix-node.conf <<EOF
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
net.core.rmem_max=16777216
net.core.wmem_max=16777216
EOF
  sysctl --system >/dev/null 2>&1 || true
}

wait_node_synced() {
  for _ in $(seq 1 40); do
    if "$BIN" node status 2>/dev/null | grep -q '"ok": true'; then return 0; fi
    sleep 1
  done
  return 1
}

cmd_node_install() {
  require_root
  banner
  detect_os
  detect_arch
  info "Система: $OS_NAME ($ARCH)"
  NODE_PANEL=${CLOUDRIX_PANEL_URL-} NODE_ID=${CLOUDRIX_NODE_ID-} NODE_SECRET=${CLOUDRIX_NODE_SECRET-}
  while (($#)); do
    case $1 in
      --panel) NODE_PANEL=${2-} && shift ;;
      --node-id) NODE_ID=${2-} && shift ;;
      --secret) NODE_SECRET=${2-} && shift ;;
      *) die "Неизвестный параметр: $1" ;;
    esac
    shift
  done
  if [[ -f $ENV_FILE ]]; then
    warn "На этом сервере установлена панель. Ноду лучше ставить на отдельный сервер: так трафик клиентов не идёт через сервер панели."
    ((ASSUME_YES)) || ask_yes_no "Всё равно поставить ноду сюда?" n || exit 0
  fi
  ask NODE_PANEL "Адрес панели (например https://panel.example.com:2053)"
  ask NODE_ID "Номер ноды (из панели)"
  ask NODE_SECRET "Ключ ноды (из панели)"
  NODE_PANEL=${NODE_PANEL%/}
  [[ $NODE_PANEL =~ ^https?://[^[:space:]/]+$ ]] || die "Адрес панели должен быть вида https://host:port"
  [[ $NODE_ID =~ ^[0-9]+$ ]] || die "Номер ноды — число"
  [[ $NODE_SECRET =~ ^[A-Za-z0-9_-]{20,}$ ]] || die "Неверный ключ ноды. Скопируйте команду установки из панели целиком."

  step "1/4  Подготовка"
  ensure_node_deps
  info "Проверяю связь с панелью $NODE_PANEL"
  curl -fsS --max-time 10 "$NODE_PANEL/health" >/dev/null 2>&1 ||
    die "Сервер не достучался до панели $NODE_PANEL. Проверьте адрес, что панель работает и её порт открыт для этого сервера."
  ok "Панель отвечает"

  step "2/4  Агент Cloudrix"
  INSTALL_CONF=$NODE_CONF_DIR/install.conf
  mkdir -p "$NODE_CONF_DIR"
  load_install_conf
  resolve_target
  fetch_binary
  install_binary

  step "3/4  Ядра Xray и sing-box"
  install_cores

  step "4/4  Запуск"
  write_node_env
  write_node_service
  tune_network
  systemctl enable "$NODE_SERVICE" >/dev/null 2>&1
  systemctl restart "$NODE_SERVICE"
  save_install_conf
  if wait_node_synced; then
    ok "Нода подключилась к панели"
  else
    warn "Нода пока не подключилась. Причина — в логах: cloudrix-manager node-logs"
  fi
  printf '\n%s%s  Нода установлена%s\n\n' "$C_BOLD" "$C_GREEN" "$C_RESET"
  printf '  В панели статус ноды сменится на «connected».\n'
  printf '  Добавьте инбаунды на эту ноду в разделе «Инбаунды» — ядра получат их сами,\n'
  printf '  порты откроются автоматически, если на сервере включён ufw или firewalld.\n'
  printf '  Если фаервол есть у хостера (в личном кабинете), откройте там порты инбаундов.\n\n'
  printf '  Управление нодой: %scloudrix-manager%s\n\n' "$C_BOLD" "$C_RESET"
}

cmd_node_update() {
  require_root
  detect_os
  detect_arch
  [[ -f $NODE_ENV ]] || die "Нода не установлена."
  INSTALL_CONF=$NODE_CONF_DIR/install.conf
  load_install_conf
  if [[ -n ${CLOUDRIX_TARGET-} ]]; then
    TARGET_CHANNEL=${CLOUDRIX_TARGET%%:*} TARGET_REF=${CLOUDRIX_TARGET#*:}
  else
    info "Проверяю обновления ($REPO${BRANCH:+, ветка $BRANCH})"
    resolve_target
  fi
  self_update node-update
  ensure_node_deps
  fetch_binary
  install_binary
  install_cores
  systemctl restart "$NODE_SERVICE"
  save_install_conf
  if wait_node_synced; then ok "Нода обновлена и подключена к панели"; else warn "Нода обновлена, но пока не подключилась: cloudrix-manager node-logs"; fi
}

cmd_node_logs() {
  if ((FOLLOW)); then
    journalctl -u "$NODE_SERVICE" -f -n 100
  else
    journalctl -u "$NODE_SERVICE" -n 60 --no-pager
  fi
}

cmd_node_info() {
  local state panel id
  state=$(systemctl is-active "$NODE_SERVICE" 2>/dev/null || true)
  panel=$(sed -n 's/^CLOUDRIX_PANEL_URL=//p' "$NODE_ENV" 2>/dev/null || true)
  id=$(sed -n 's/^CLOUDRIX_NODE_ID=//p' "$NODE_ENV" 2>/dev/null || true)
  printf '\n  %sНода:%s       №%s, панель %s\n' "$C_BOLD" "$C_RESET" "${id:-?}" "${panel:-?}"
  printf '  %sСостояние:%s  %s\n' "$C_BOLD" "$C_RESET" "$([[ $state == active ]] && printf '%sработает%s' "$C_GREEN" "$C_RESET" || printf '%s%s%s' "$C_RED" "${state:-остановлена}" "$C_RESET")"
  printf '  %sАгент:%s      %s\n' "$C_BOLD" "$C_RESET" "$("$BIN" version 2>/dev/null || echo '?')"
  if "$BIN" node status >/dev/null 2>&1; then
    printf '  %sСвязь:%s      %s\n' "$C_BOLD" "$C_RESET" "$("$BIN" node status | tr -d '\n' | sed 's/  */ /g')"
  fi
  echo
}

cmd_node_uninstall() {
  require_root
  warn "Будут удалены агент ноды, ядра Xray и sing-box и их настройки."
  ((ASSUME_YES)) || ask_yes_no "Продолжить?" n || exit 0
  systemctl disable --now "$NODE_SERVICE" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$NODE_SERVICE.service" /etc/sysctl.d/99-cloudrix-node.conf
  systemctl daemon-reload
  rm -rf "$NODE_CONF_DIR" "$NODE_DATA_DIR" "$NODE_LIB"
  if [[ ! -f $ENV_FILE ]]; then
    rm -f "$BIN" "$MANAGER"
  fi
  ok "Нода удалена. Удалите её и в панели (раздел «Ноды»)."
}

# ---------- Hysteria2 на ноде ----------

HY_CONF="$NODE_DATA_DIR/hysteria.json"
HY_CERT_DIR="$NODE_DATA_DIR/tls/hysteria"

# ask_secret VAR "Вопрос" — ввод без отображения на экране
ask_secret() {
  local __var=$1 prompt=$2 reply
  if [[ -n ${!__var-} ]]; then return; fi
  have_tty || die "Нужен ввод в терминале или переменная окружения."
  read -r -s -p "$(printf '%s?%s %s: ' "$C_CYAN" "$C_RESET" "$prompt")" reply <"$TTY"
  echo
  printf -v "$__var" '%s' "$reply"
}

# cmd_node_hysteria: домен + токен Cloudflare → A-запись на эту ноду,
# сертификат Let's Encrypt через DNS (порт 80 не нужен) и инбаунд Hysteria2,
# который панель создаёт сама. Повторный запуск заменяет прежнюю установку.
cmd_node_hysteria() {
  require_root
  detect_os
  [[ -f $NODE_ENV ]] || die "Сначала установите ноду (команда из панели, раздел «Ноды»)."
  local old="" domain=${CLOUDRIX_HY_DOMAIN-} token=${CLOUDRIX_CF_TOKEN-} port=${CLOUDRIX_HY_PORT-} ip zone acme="$HOME/.acme.sh/acme.sh"
  if [[ -f $HY_CONF ]]; then
    old=$(sed -n 's/.*"domain": *"\([^"]*\)".*/\1/p' "$HY_CONF")
    warn "Hysteria2 уже установлен${old:+ для $old}. Прежняя установка будет удалена, и будет установлена новая."
    ((ASSUME_YES)) || ask_yes_no "Продолжить?" n || return 0
  fi

  echo
  info "Нужны домен в Cloudflare и API-токен с правом Zone → DNS → Edit"
  info "(dash.cloudflare.com → My Profile → API Tokens → Create Token → «Edit zone DNS»)."
  ask domain "Домен для Hysteria2, например hy.example.com"
  domain=$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]' | sed 's#^https\?://##; s#/.*##; s/\.$//')
  [[ $domain =~ ^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$ ]] || die "Это не похоже на домен: $domain"
  ask_secret token "API-токен Cloudflare (ввод скрыт)"
  [[ -n $token ]] || die "Токен не введён."
  ask port "UDP-порт для Hysteria2" 443
  [[ $port =~ ^[0-9]+$ ]] && ((port >= 1 && port <= 65535)) || die "Неверный порт: $port"

  ip=$(public_ip)
  [[ -n $ip ]] || die "Не удалось узнать внешний IP этого сервера."
  info "Направляю $domain на $ip в Cloudflare (без проксирования — Hysteria2 работает по UDP)"
  zone=$(CLOUDFLARE_API_TOKEN=$token "$BIN" node dns "$domain" "$ip") || die "Cloudflare не принял запрос. Проверьте токен (право Zone → DNS → Edit) и что домен добавлен в Cloudflare."

  if ! command -v crontab >/dev/null; then
    [[ $PKG == apt ]] && install_packages cron || install_packages cronie
  fi
  if [[ ! -x $acme ]]; then
    info "Устанавливаю acme.sh"
    curl -fsSL https://get.acme.sh | sh -s >/dev/null || die "Не удалось установить acme.sh"
  fi
  if [[ -n $old ]]; then
    info "Удаляю прежнюю установку ($old)"
    "$acme" --remove -d "$old" --ecc >/dev/null 2>&1 || true
    rm -rf "$HOME/.acme.sh/${old}_ecc"
  fi
  rm -rf "$HY_CERT_DIR" "$HY_CONF"

  info "Получаю сертификат Let's Encrypt для $domain через DNS (1–2 минуты)"
  local rc=0 log
  log=$(mktemp)
  CF_Token=$token CF_Zone_ID=$zone "$acme" --issue --dns dns_cf -d "$domain" --keylength ec-256 \
    --server letsencrypt --force >"$log" 2>&1 || rc=$?
  if ((rc != 0 && rc != 2)); then
    tail -n 15 "$log" >&2
    rm -f "$log"
    die "Сертификат не выпущен. Лог выше."
  fi
  rm -f "$log"
  mkdir -p "$HY_CERT_DIR"
  "$acme" --install-cert -d "$domain" --ecc \
    --fullchain-file "$HY_CERT_DIR/fullchain.pem" \
    --key-file "$HY_CERT_DIR/key.pem" \
    --reloadcmd "chmod 600 $HY_CERT_DIR/key.pem; systemctl restart $NODE_SERVICE" >/dev/null || die "Не удалось сохранить сертификат."

  umask 077
  printf '{\n  "domain": "%s",\n  "port": %d,\n  "cert": "%s",\n  "key": "%s"\n}\n' \
    "$domain" "$port" "$HY_CERT_DIR/fullchain.pem" "$HY_CERT_DIR/key.pem" >"$HY_CONF"
  umask 022
  systemctl restart "$NODE_SERVICE"
  ok "Hysteria2 установлен: $domain, UDP $port. Сертификат продлевается автоматически."
  info "Панель сама создаст инбаунд «hysteria2-node…» и добавит его в первую группу (Default)."
  info "Если у хостинга есть свой файрвол в личном кабинете, откройте в нём UDP $port."
}

node_menu() {
  have_tty || die "Меню работает только в терминале. Команды: $(basename "$0") --help"
  local choice
  while true; do
    clear 2>/dev/null || true
    banner
    (cmd_node_info) || true
    printf '  %s1)%s Обновить ноду (агент и ядра)\n' "$C_BOLD" "$C_RESET"
    printf '  %s2)%s Перезапустить\n' "$C_BOLD" "$C_RESET"
    printf '  %s3)%s Логи (последние строки)\n' "$C_BOLD" "$C_RESET"
    printf '  %s4)%s Логи в реальном времени %s(выход — Ctrl+C)%s\n' "$C_BOLD" "$C_RESET" "$C_DIM" "$C_RESET"
    printf '  %s5)%s Hysteria2 (домен + Cloudflare)%s\n' "$C_BOLD" "$C_RESET" "$([[ -f $HY_CONF ]] && printf ' %s— установлен%s' "$C_GREEN" "$C_RESET")"
    printf '  %s6)%s Удалить ноду\n' "$C_BOLD" "$C_RESET"
    printf '  %s0)%s Выход\n\n' "$C_BOLD" "$C_RESET"
    read -r -p "$(printf '%s?%s Выберите пункт: ' "$C_CYAN" "$C_RESET")" choice <"$TTY" || return 0
    case $choice in
      1) run_menu_action cmd_node_update ;;
      2) run_menu_action bash -c "systemctl restart $NODE_SERVICE && echo '✔ Перезапущено'" ;;
      3) run_menu_action cmd_node_logs ;;
      4) FOLLOW=1 run_menu_action cmd_node_logs ;;
      5) run_menu_action cmd_node_hysteria ;;
      6)
        run_menu_action cmd_node_uninstall
        [[ -f $NODE_ENV ]] || return 0
        ;;
      0 | q | exit | "") return 0 ;;
      *) warn "Нет такого пункта" ;;
    esac
  done
}

# ---------- сертификат домена для нод (главный сервер) ----------

SHARED_TLS_DIR="$CONF_DIR/tls"

# Сертификаты доменов для нод (главный сервер). У каждой ноды может быть свой
# домен: сертификаты лежат в $SHARED_TLS_DIR/<домен>/, их пути вставляются в
# инбаунд, а панель сама отправляет файлы ноде и обновляет их после продления.
CF_TOKEN_FILE="$CONF_DIR/cloudflare.token"

# domain_cert_list печатает домены с датой окончания сертификата.
domain_cert_list() {
  local d end n=0
  for d in "$SHARED_TLS_DIR"/*/; do
    [[ -f $d/fullchain.pem ]] || continue
    d=${d%/}
    n=$((n + 1))
    end=$(openssl x509 -enddate -noout -in "$d/fullchain.pem" 2>/dev/null | cut -d= -f2)
    printf '  %s%d)%s %s  %s(до %s)%s\n' "$C_BOLD" "$n" "$C_RESET" "${d##*/}" "$C_DIM" "${end:-?}" "$C_RESET"
  done
  ((n)) || info "Доменов пока нет."
  return 0
}

# domain_cert_names выводит домены по одному в строке (в порядке списка).
domain_cert_names() {
  local d
  for d in "$SHARED_TLS_DIR"/*/; do
    [[ -f $d/fullchain.pem ]] && { d=${d%/}; echo "${d##*/}"; }
  done
  return 0
}

domain_cert_paths() {
  local dir="$SHARED_TLS_DIR/$1"
  printf '\n    %sCert file:%s       %s\n' "$C_BOLD" "$C_RESET" "$dir/fullchain.pem"
  printf '    %sKey file:%s        %s\n' "$C_BOLD" "$C_RESET" "$dir/privkey.pem"
  printf '    %sSNI:%s             %s\n' "$C_BOLD" "$C_RESET" "$1"
  printf '    %sPublic address:%s  %s\n\n' "$C_BOLD" "$C_RESET" "$1"
}

# domain_cert_used сообщает, указан ли путь домена в каком-нибудь инбаунде панели.
domain_cert_used() {
  local f
  for f in "$DATA_DIR/cloudrix.db" "$DATA_DIR/cloudrix.db-wal"; do
    [[ -f $f ]] && grep -aq "$SHARED_TLS_DIR/$1/" "$f" && return 0
  done
  return 1
}

cmd_domain_cert_add() {
  require_root
  detect_os
  [[ -f $ENV_FILE ]] || die "Панель не установлена."
  local domain=${CLOUDRIX_CERT_DOMAIN-} token=${CLOUDRIX_CF_TOKEN-} points zone acme="$HOME/.acme.sh/acme.sh" dir rc=0 log
  echo
  info "Домен должен быть в Cloudflare и уже указывать на IP ноды (A-запись, серое облако)."
  ask domain "Домен, например hy.example.com"
  domain=$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]' | sed 's#^https\?://##; s#/.*##; s/\.$//')
  [[ $domain =~ ^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$ ]] || die "Это не похоже на домен: $domain"
  dir="$SHARED_TLS_DIR/$domain"
  if [[ -f $dir/fullchain.pem ]]; then
    warn "Для $domain сертификат уже есть. Прежний будет удалён и выпущен новый (пути останутся теми же)."
    ((ASSUME_YES)) || ask_yes_no "Продолжить?" n || return 0
  fi
  points=$(getent ahostsv4 "$domain" 2>/dev/null | awk 'NR==1 {print $1}' || true)
  if [[ -n $points ]]; then info "$domain сейчас указывает на $points"; else warn "$domain пока ни на что не указывает — сертификат выпустится, но клиенты не подключатся, пока нет A-записи."; fi

  if [[ -z $token && -s $CF_TOKEN_FILE ]] && { ((ASSUME_YES)) || ask_yes_no "Использовать сохранённый токен Cloudflare?" y; }; then
    token=$(<"$CF_TOKEN_FILE")
  fi
  if [[ -z $token ]]; then
    info "Токен: dash.cloudflare.com → My Profile → API Tokens → Create Token → «Edit zone DNS»."
    ask_secret token "API-токен Cloudflare (ввод скрыт)"
    [[ -n $token ]] || die "Токен не введён."
  fi
  if [[ ! -s $CF_TOKEN_FILE || $(<"$CF_TOKEN_FILE") != "$token" ]] &&
    { ((ASSUME_YES)) || ask_yes_no "Сохранить токен на сервере для следующих доменов?" y; }; then
    (umask 077 && printf '%s\n' "$token" >"$CF_TOKEN_FILE")
  fi
  zone=$(CLOUDFLARE_API_TOKEN=$token "$BIN" cf-zone "$domain" 2>/dev/null || true)
  [[ -n $zone ]] || warn "Не удалось найти зону через API — acme.sh попробует сам."

  if ! command -v crontab >/dev/null; then
    [[ $PKG == apt ]] && install_packages cron || install_packages cronie
  fi
  if [[ ! -x $acme ]]; then
    info "Устанавливаю acme.sh"
    curl -fsSL https://get.acme.sh | sh -s >/dev/null || die "Не удалось установить acme.sh"
  fi

  info "Получаю сертификат Let's Encrypt для $domain через DNS (1–2 минуты)"
  log=$(mktemp)
  env CF_Token="$token" ${zone:+CF_Zone_ID="$zone"} "$acme" --issue --dns dns_cf -d "$domain" --keylength ec-256 \
    --server letsencrypt --force >"$log" 2>&1 || rc=$?
  if ((rc != 0 && rc != 2)); then
    tail -n 15 "$log" >&2
    rm -f "$log"
    die "Сертификат не выпущен. Лог выше."
  fi
  rm -f "$log"
  mkdir -p "$dir"
  "$acme" --install-cert -d "$domain" --ecc \
    --fullchain-file "$dir/fullchain.pem" \
    --key-file "$dir/privkey.pem" \
    --reloadcmd "chown -R $SERVICE_USER:$SERVICE_USER $SHARED_TLS_DIR; chmod 600 $dir/privkey.pem; systemctl restart $SERVICE" >/dev/null ||
    die "Не удалось сохранить сертификат."
  rm -f "$SHARED_TLS_DIR/current" # от прежней версии меню (один домен)
  chown -R "$SERVICE_USER:$SERVICE_USER" "$SHARED_TLS_DIR"
  chmod 700 "$SHARED_TLS_DIR" "$dir"
  chmod 600 "$dir/privkey.pem"
  systemctl restart "$SERVICE"

  ok "Сертификат для $domain выпущен (продлевается автоматически)."
  printf '\n  Вставьте в панели: Инбаунды → инбаунд на нужной ноде (например Hysteria2):\n'
  domain_cert_paths "$domain"
  info "Панель сама отправит сертификат ноде и обновит его после продления."
}

cmd_domain_cert_remove() {
  require_root
  local domain=${1-} n names=() acme="$HOME/.acme.sh/acme.sh"
  mapfile -t names < <(domain_cert_names)
  ((${#names[@]})) || { info "Доменов нет."; return 0; }
  if [[ -z $domain ]]; then
    domain_cert_list
    ask n "Номер домена для удаления"
    [[ $n =~ ^[0-9]+$ ]] && ((n >= 1 && n <= ${#names[@]})) || die "Нет такого номера: $n"
    domain=${names[n - 1]}
  fi
  [[ -f $SHARED_TLS_DIR/$domain/fullchain.pem ]] || die "Домена $domain нет в списке."
  if domain_cert_used "$domain"; then
    warn "Сертификат $domain указан в инбаунде панели — после удаления этот инбаунд перестанет работать. Сначала поменяйте или удалите его."
  fi
  warn "Будут удалены сертификат $domain и его автопродление."
  ((ASSUME_YES)) || ask_yes_no "Удалить?" n || return 0
  [[ -x $acme ]] && "$acme" --remove -d "$domain" --ecc >/dev/null 2>&1 || true
  rm -rf "${SHARED_TLS_DIR:?}/$domain" "$HOME/.acme.sh/${domain}_ecc"
  ok "Домен $domain удалён."
}

cmd_domain_cert_list() {
  local names d
  echo
  domain_cert_list
  mapfile -t names < <(domain_cert_names)
  if ((${#names[@]})); then
    printf '\n  Пути для панели:\n'
    for d in "${names[@]}"; do printf '\n  %s%s%s' "$C_BOLD" "$d" "$C_RESET"; domain_cert_paths "$d"; done
  fi
}

# cmd_domain_cert: подменю «Добавить / Список / Удалить» или команда
# domain-cert add|list|remove [домен].
cmd_domain_cert() {
  case ${1-} in
    add) cmd_domain_cert_add; return ;;
    list | ls) cmd_domain_cert_list; return ;;
    remove | rm | delete) cmd_domain_cert_remove "${2-}"; return ;;
    "") ;;
    *) die "Использование: domain-cert add | list | remove [домен]" ;;
  esac
  have_tty || die "Меню работает только в терминале. Команды: domain-cert add | list | remove [домен]"
  local choice
  while true; do
    printf '\n  %sСертификаты доменов для нод%s\n\n' "$C_BOLD" "$C_RESET"
    domain_cert_list
    printf '\n  %s1)%s Добавить домен\n' "$C_BOLD" "$C_RESET"
    printf '  %s2)%s Список и пути для панели\n' "$C_BOLD" "$C_RESET"
    printf '  %s3)%s Удалить домен\n' "$C_BOLD" "$C_RESET"
    printf '  %s0)%s Назад\n\n' "$C_BOLD" "$C_RESET"
    read -r -p "$(printf '%s?%s Выберите пункт: ' "$C_CYAN" "$C_RESET")" choice <"$TTY" || return 0
    case $choice in
      1) (cmd_domain_cert_add) || true ;;
      2) cmd_domain_cert_list ;;
      3) (cmd_domain_cert_remove) || true ;;
      0 | q | "") return 0 ;;
      *) warn "Нет такого пункта" ;;
    esac
  done
}

# ---------- меню ----------

menu() {
  require_root
  if [[ ! -f $ENV_FILE && -f $NODE_ENV ]]; then
    node_menu
    return
  fi
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
    printf '  %s7)%s Домены и сертификаты для нод (Cloudflare)%s\n' "$C_BOLD" "$C_RESET" "$(n=$(domain_cert_names | wc -l); ((n)) && printf ' %s— %d шт.%s' "$C_GREEN" "$n" "$C_RESET")"
    printf '  %s8)%s Сделать резервную копию базы\n' "$C_BOLD" "$C_RESET"
    printf '  %s9)%s Восстановить базу из копии\n' "$C_BOLD" "$C_RESET"
    printf '  %s10)%s Удалить панель\n' "$C_BOLD" "$C_RESET"
    printf '  %s0)%s Выход\n\n' "$C_BOLD" "$C_RESET"
    read -r -p "$(printf '%s?%s Выберите пункт: ' "$C_CYAN" "$C_RESET")" choice <"$TTY" || return 0
    case $choice in
      1) run_menu_action cmd_update ;;
      2) run_menu_action bash -c "systemctl restart $SERVICE && echo '✔ Перезапущено'" ;;
      3) run_menu_action cmd_logs ;;
      4) FOLLOW=1 run_menu_action cmd_logs ;;
      5) run_menu_action cmd_admin ;;
      6) run_menu_action cmd_cert ;;
      7) run_menu_action cmd_domain_cert ;;
      8) run_menu_action cmd_backup ;;
      9) run_menu_action cmd_restore ;;
      10)
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

  node --panel URL --node-id N --secret KEY
                 установить ноду (команду целиком даёт панель: «Ноды» → «Установить»)
  node-update    обновить агент ноды и ядра Xray / sing-box
  node-info      состояние ноды
  node-logs [-f] логи ноды
  node-uninstall удалить ноду
  domain-cert [add|list|remove ДОМЕН]  домены и сертификаты для нод (Cloudflare DNS)
  node-hysteria  Hysteria2 на ноде: домен в Cloudflare + сертификат Let's Encrypt

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
    if [[ -f $ENV_FILE || -f $NODE_ENV ]] && ! ((ASSUME_YES)) && have_tty; then
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
    node) cmd_node_install "${args[@]}" ;;
    node-update) cmd_node_update ;;
    node-info) cmd_node_info ;;
    node-logs) cmd_node_logs ;;
    node-uninstall) cmd_node_uninstall ;;
    node-hysteria) cmd_node_hysteria ;;
    domain-cert) cmd_domain_cert "${args[@]}" ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
