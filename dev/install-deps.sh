#!/usr/bin/env bash
# Install PostgreSQL (required) and optionally Redis for a KiriVers host.
# Does not download the KiriVers binary. Windows is not supported.
set -euo pipefail

print_next_steps() {
  cat <<'EOF'

PostgreSQL is installed. Typical listen port: 5432.
  Debian/Ubuntu (apt): cluster user is often "postgres"; create a role and database for KiriVers.
  RHEL/Fedora (dnf/yum): after postgresql-setup, the service user is typically "postgres".

Next:
  1. Download the KiriVers-<OS>-<Arch>-<hash>.zip archive for your platform from
     https://github.com/Kirizu-Official/KiriVers/releases and unpack it
     (the inner binary is kirivers-<os>-<arch>; the hash in the file name changes
     with every release, so pick by prefix, not by a bookmarked link).
  2. Follow the manual install page in the documentation
     (copy the three YAML files, create the first admin, start the process).

Redis notes:
  - Single process, modest traffic: you may skip Redis and keep cache.driver=memory.
  - Multiple KiriVers processes, or cache.driver=redis: Redis must answer PING at
    *startup* or KiriVers will not listen. A later Redis outage falls back to
    in-process memory and reconnects; the process does not exit immediately.
EOF
}

unsupported() {
  echo "No apt-get, yum, or dnf found. Use the manual install path or Docker." >&2
  exit 1
}

install_postgres_apt() {
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql postgresql-contrib
  systemctl enable postgresql || systemctl enable postgresql.service || true
  systemctl start postgresql || systemctl start postgresql.service || true
}

install_redis_apt() {
  DEBIAN_FRONTEND=noninteractive apt-get install -y redis-server
  systemctl enable redis-server || systemctl enable redis || true
  systemctl start redis-server || systemctl start redis || true
}

install_postgres_dnf() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y postgresql-server postgresql
    if [[ ! -d /var/lib/pgsql/data ]] && command -v postgresql-setup >/dev/null 2>&1; then
      postgresql-setup --initdb || postgresql-setup initdb || true
    fi
    systemctl enable postgresql
    systemctl start postgresql
    return
  fi
  yum install -y postgresql-server postgresql
  if command -v postgresql-setup >/dev/null 2>&1; then
    postgresql-setup --initdb || postgresql-setup initdb || true
  fi
  systemctl enable postgresql
  systemctl start postgresql
}

install_redis_dnf() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y redis
  else
    yum install -y redis
  fi
  systemctl enable redis || systemctl enable redis-server || true
  systemctl start redis || systemctl start redis-server || true
}

uname_s="$(uname -s 2>/dev/null || echo unknown)"
case "${uname_s}" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT)
    echo "This script does not support Windows. Use the manual install guide." >&2
    exit 1
    ;;
esac

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

pkg=""
if command -v apt-get >/dev/null 2>&1; then
  pkg="apt"
elif command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1; then
  pkg="dnf"
else
  unsupported
fi

echo "Installing PostgreSQL via ${pkg}…"
if [[ "${pkg}" == "apt" ]]; then
  install_postgres_apt
else
  install_postgres_dnf
fi

echo
echo "Redis is optional."
echo "  Skip: single KiriVers process, cache.driver=memory (default)."
echo "  Install: multiple KiriVers processes, or you will set cache.driver=redis."
echo "  If cache.driver=redis, startup Ping must succeed or the process will not listen."
echo "  A runtime Redis outage falls back to in-process memory and reconnects."
echo
printf "Install Redis as well? [y/N] "
read -r reply || reply=""
case "${reply}" in
  y|Y|yes|YES)
    echo "Installing Redis…"
    if [[ "${pkg}" == "apt" ]]; then
      install_redis_apt
    else
      install_redis_dnf
    fi
    ;;
  *)
    echo "Skipping Redis."
    ;;
esac

print_next_steps
