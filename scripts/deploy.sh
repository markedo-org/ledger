#!/usr/bin/env bash
# Deploy the task-ledger binary to VPS Services.
# Usage (from this repo or via the meta-repo wrapper):
#   ./scripts/deploy.sh
#   ./scripts/deploy.sh --bootstrap
set -euo pipefail

LEDGER_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
META_ROOT="$(cd "$LEDGER_ROOT/../.." && pwd)"
HOST_IP="62.238.47.32"
LISTEN="${LEDGER_LISTEN:-127.0.0.1:8787}"
APEX="task-ledger.com"
REMOTE_DIR="/opt/ledger"
UNIT_NAME="ledger"

BOOTSTRAP=0
if [[ "${1:-}" == "--bootstrap" ]]; then
  BOOTSTRAP=1
elif [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  cat <<'EOF'
Usage: scripts/deploy.sh [--bootstrap]

  Cross-compiles linux/amd64, copies the binary to VPS Services, restarts systemd.

  --bootstrap   First time: ledger user, /opt/ledger, env file, nginx, certbot.
                Does not overwrite an existing ledger.env.

  Listen address defaults to 127.0.0.1:8787 (8080 is taken on this box).
  Override with LEDGER_LISTEN.
EOF
  exit 0
elif [[ -n "${1:-}" ]]; then
  echo "error: unknown argument $1" >&2
  exit 1
fi

if [[ -f "$META_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$META_ROOT/.env"
  set +a
fi
if [[ -f "$LEDGER_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$LEDGER_ROOT/.env"
  set +a
fi

KEY="${HETZNER_SSH_IDENTITY:-$HOME/.ssh/id_rsa}"
USER_HOST="${HETZNER_SSH_USER:-lg}@$HOST_IP"
SECRETS="$META_ROOT/.secrets/task-ledger-hosted.env"

echo "==> Build linux/amd64"
mkdir -p "$LEDGER_ROOT/dist"
(
  cd "$LEDGER_ROOT"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=$(cat VERSION) -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%d)" \
    -o dist/ledger-linux-amd64 ./cmd/ledger
)

echo "==> Sync binary -> $USER_HOST:~/ledger-bin"
rsync -av -e "ssh -i $KEY" \
  "$LEDGER_ROOT/dist/ledger-linux-amd64" \
  "$USER_HOST:~/ledger-bin"

ensure_operator_token() {
  if [[ ! -f "$SECRETS" ]]; then
    echo "error: missing $SECRETS" >&2
    exit 1
  fi
  if ! grep -q '^LEDGER_OPERATOR_TOKEN=.' "$SECRETS"; then
    umask 077
    echo "LEDGER_OPERATOR_TOKEN=lgr_$(openssl rand -hex 24)" >> "$SECRETS"
    echo "==> Added LEDGER_OPERATOR_TOKEN to $SECRETS (not printed)"
  fi
  echo "==> Ensure operator token on box (append only if missing)"
  frag="$(mktemp)"
  grep '^LEDGER_OPERATOR_TOKEN=' "$SECRETS" > "$frag"
  chmod 600 "$frag"
  rsync -a -e "ssh -i $KEY" "$frag" "$USER_HOST:~/ledger.operator.env"
  rm -f "$frag"
  ssh -i "$KEY" "$USER_HOST" "REMOTE_DIR=$REMOTE_DIR bash -s" <<'REMOTE'
set -euo pipefail
if sudo grep -q '^LEDGER_OPERATOR_TOKEN=.' "$REMOTE_DIR/ledger.env" 2>/dev/null; then
  echo "keeping existing operator token in $REMOTE_DIR/ledger.env"
else
  sudo sh -c "cat $HOME/ledger.operator.env >> $REMOTE_DIR/ledger.env"
  sudo chown root:ledger "$REMOTE_DIR/ledger.env"
  sudo chmod 0640 "$REMOTE_DIR/ledger.env"
  echo "appended operator token to $REMOTE_DIR/ledger.env"
fi
rm -f "$HOME/ledger.operator.env"
REMOTE
}

if [[ "$BOOTSTRAP" -eq 1 ]]; then
  if [[ ! -f "$SECRETS" ]]; then
    TOKEN="lgr_$(openssl rand -hex 24)"
    OPTOKEN="lgr_$(openssl rand -hex 24)"
    umask 077
    mkdir -p "$(dirname "$SECRETS")"
    cat > "$SECRETS" <<ENV
# Hosted task-ledger on VPS Services. Gitignored. Do not commit.
LEDGER_BOOT_TOKEN=$TOKEN
LEDGER_OPERATOR_TOKEN=$OPTOKEN
LEDGER_HTML_AUTH=1
LEDGER_SECURE_COOKIES=1
LEDGER_ROOT=url
LEDGER_ROOT_URL=https://www.task-ledger.com
LEDGER_LISTEN=$LISTEN
ENV
    echo "==> Wrote $SECRETS (tokens stored there, not printed)"
  else
    echo "==> Reusing existing $SECRETS"
  fi

  echo "==> Bootstrap $APEX on $HOST_IP"
  rsync -av -e "ssh -i $KEY" "$SECRETS" "$USER_HOST:~/ledger.env.incoming"
  ssh -i "$KEY" "$USER_HOST" "LISTEN=$LISTEN APEX=$APEX REMOTE_DIR=$REMOTE_DIR UNIT_NAME=$UNIT_NAME bash -s" <<'REMOTE'
set -euo pipefail
if ! id -u ledger >/dev/null 2>&1; then
  sudo useradd --system --home "$REMOTE_DIR" --shell /usr/sbin/nologin ledger
fi
sudo mkdir -p "$REMOTE_DIR"
sudo chown ledger:ledger "$REMOTE_DIR"
sudo install -o ledger -g ledger -m 0755 "$HOME/ledger-bin" "$REMOTE_DIR/ledger"
rm -f "$HOME/ledger-bin"

if [[ ! -f "$REMOTE_DIR/ledger.env" ]]; then
  sudo install -o root -g ledger -m 0640 "$HOME/ledger.env.incoming" "$REMOTE_DIR/ledger.env"
else
  echo "keeping existing $REMOTE_DIR/ledger.env"
fi
rm -f "$HOME/ledger.env.incoming"

sudo tee /etc/systemd/system/"$UNIT_NAME".service >/dev/null <<UNIT
[Unit]
Description=task-ledger
After=network.target

[Service]
Type=simple
User=ledger
Group=ledger
WorkingDirectory=$REMOTE_DIR
EnvironmentFile=$REMOTE_DIR/ledger.env
ExecStart=$REMOTE_DIR/ledger -listen $LISTEN -db $REMOTE_DIR/ledger.sqlite
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now "$UNIT_NAME"
sleep 1
sudo systemctl --no-pager --full status "$UNIT_NAME" || true

sudo tee /etc/nginx/sites-available/"$APEX".conf >/dev/null <<NGINX
upstream ledger_app {
    server $LISTEN;
    keepalive 16;
}

server {
    listen 80;
    listen [::]:80;
    server_name $APEX;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location /mcp {
        proxy_pass http://ledger_app;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Authorization \$http_authorization;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location / {
        proxy_pass http://ledger_app;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Authorization \$http_authorization;
        proxy_set_header Connection "";
        proxy_read_timeout 60s;
    }

    client_max_body_size 2m;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;
}
NGINX

sudo ln -sfn /etc/nginx/sites-available/"$APEX".conf /etc/nginx/sites-enabled/"$APEX".conf
sudo nginx -t
sudo systemctl reload nginx

sudo certbot --nginx \
  -d "$APEX" \
  --non-interactive --agree-tos -m lg@markedo.com \
  --redirect

sudo tee /etc/nginx/sites-available/"$APEX".conf >/dev/null <<NGINX
upstream ledger_app {
    server $LISTEN;
    keepalive 16;
}

server {
    listen 80;
    listen [::]:80;
    server_name $APEX;
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name $APEX;

    ssl_certificate /etc/letsencrypt/live/$APEX/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$APEX/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;

    location /health {
        proxy_pass http://ledger_app;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header Connection "";
    }

    location /mcp {
        proxy_pass http://ledger_app;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Authorization \$http_authorization;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location / {
        proxy_pass http://ledger_app;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Authorization \$http_authorization;
        proxy_set_header Connection "";
        proxy_read_timeout 60s;
    }

    client_max_body_size 2m;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;
}
NGINX

sudo nginx -t
sudo systemctl reload nginx
echo "Bootstrap done: https://$APEX"
REMOTE
  ensure_operator_token
  ssh -i "$KEY" "$USER_HOST" "sudo systemctl restart $UNIT_NAME"
else
  ensure_operator_token
  echo "==> Install binary and restart $UNIT_NAME"
  ssh -i "$KEY" "$USER_HOST" "REMOTE_DIR=$REMOTE_DIR UNIT_NAME=$UNIT_NAME bash -s" <<'REMOTE'
set -euo pipefail
if [[ ! -x "$REMOTE_DIR/ledger" && ! -f "$REMOTE_DIR/ledger.env" ]]; then
  echo "error: $REMOTE_DIR is not bootstrapped; re-run with --bootstrap" >&2
  exit 1
fi
sudo install -o ledger -g ledger -m 0755 "$HOME/ledger-bin" "$REMOTE_DIR/ledger"
rm -f "$HOME/ledger-bin"
sudo systemctl restart "$UNIT_NAME"
sleep 1
sudo systemctl --no-pager --full status "$UNIT_NAME"
echo "Restarted $UNIT_NAME"
REMOTE
fi
