#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# inBeTwin — One-time server setup script
# Run as root (or with sudo) on a fresh Ubuntu 22.04 / 24.04 VPS.
#
# Usage:
#   sudo bash scripts/setup-server.sh --domain inbetwin.ru --email you@example.com
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Parse arguments ───────────────────────────────────────────────────────────
DOMAIN=""
EMAIL=""
DEPLOY_DIR="/opt/inbetwin"
DEPLOY_USER="deploy"

while [[ $# -gt 0 ]]; do
  case $1 in
    --domain) DOMAIN="$2"; shift 2 ;;
    --email)  EMAIL="$2";  shift 2 ;;
    --dir)    DEPLOY_DIR="$2"; shift 2 ;;
    --user)   DEPLOY_USER="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "$DOMAIN" || -z "$EMAIL" ]]; then
  echo "Usage: $0 --domain <domain> --email <email>"
  exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " inBeTwin server setup"
echo " Domain : $DOMAIN"
echo " Email  : $EMAIL"
echo " Dir    : $DEPLOY_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ── 1. System packages ────────────────────────────────────────────────────────
apt-get update -qq
apt-get install -y -qq curl git ufw fail2ban

# ── 2. Docker ─────────────────────────────────────────────────────────────────
if ! command -v docker &>/dev/null; then
  echo "→ Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
fi

docker --version
docker compose version

# ── 3. Deploy user ────────────────────────────────────────────────────────────
if ! id "$DEPLOY_USER" &>/dev/null; then
  useradd -m -s /bin/bash "$DEPLOY_USER"
  usermod -aG docker "$DEPLOY_USER"
  echo "→ Created user '$DEPLOY_USER' (add SSH key: /home/$DEPLOY_USER/.ssh/authorized_keys)"
fi

# ── 4. Project directory ──────────────────────────────────────────────────────
mkdir -p "$DEPLOY_DIR"
chown "$DEPLOY_USER:$DEPLOY_USER" "$DEPLOY_DIR"

if [[ ! -d "$DEPLOY_DIR/.git" ]]; then
  echo "→ Cloning repository..."
  sudo -u "$DEPLOY_USER" git clone https://gitlab.com/khdinova/inbetwinner.git "$DEPLOY_DIR"
fi

mkdir -p "$DEPLOY_DIR/secrets"
chmod 700 "$DEPLOY_DIR/secrets"
chown "$DEPLOY_USER:$DEPLOY_USER" "$DEPLOY_DIR/secrets"

# ── 5. Environment file ───────────────────────────────────────────────────────
if [[ ! -f "$DEPLOY_DIR/.env" ]]; then
  cp "$DEPLOY_DIR/.env.production.example" "$DEPLOY_DIR/.env"
  chown "$DEPLOY_USER:$DEPLOY_USER" "$DEPLOY_DIR/.env"
  chmod 600 "$DEPLOY_DIR/.env"
  echo ""
  echo "⚠️  IMPORTANT: Edit $DEPLOY_DIR/.env and fill in all CHANGE_ME values before proceeding."
  echo "   Then re-run this script."
  exit 0
fi

# ── 6. Nginx config — replace DOMAIN_PLACEHOLDER ─────────────────────────────
sed -i "s/DOMAIN_PLACEHOLDER/$DOMAIN/g" "$DEPLOY_DIR/nginx/conf.d/default.conf"
echo "→ Nginx config updated for domain: $DOMAIN"

# ── 7. Firewall ───────────────────────────────────────────────────────────────
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
echo "→ UFW enabled (SSH, HTTP, HTTPS)"

# ── 8. Start stack (HTTP only for certbot challenge) ─────────────────────────
echo "→ Starting services..."
cd "$DEPLOY_DIR"
sudo -u "$DEPLOY_USER" docker compose -f docker-compose.prod.yml up -d postgres redis rabbitmq minio
sudo -u "$DEPLOY_USER" docker compose -f docker-compose.prod.yml up -d nginx

# Wait for nginx to be ready
sleep 5

# ── 9. Issue Let's Encrypt certificate ───────────────────────────────────────
echo "→ Requesting TLS certificate for $DOMAIN..."
docker run --rm \
  -v "inbetwin_certbot_certs:/etc/letsencrypt" \
  -v "inbetwin_certbot_challenge:/var/www/certbot" \
  certbot/certbot certonly \
    --webroot \
    --webroot-path=/var/www/certbot \
    --email "$EMAIL" \
    --agree-tos \
    --no-eff-email \
    -d "$DOMAIN" \
    -d "www.$DOMAIN"

echo "→ Certificate issued."

# ── 10. Start all services ────────────────────────────────────────────────────
sudo -u "$DEPLOY_USER" docker compose -f docker-compose.prod.yml up -d

# ── 11. GitLab Runner (optional, for shell-based deploy) ─────────────────────
# If you prefer the runner to deploy directly on this machine instead of SSH,
# install and register it here.  Skip this block if you use SSH-based deploy.
if [[ "${INSTALL_GITLAB_RUNNER:-false}" == "true" ]]; then
  echo "→ Installing GitLab Runner..."
  curl -fsSL https://packages.gitlab.com/install/repositories/runner/gitlab-runner/script.deb.sh | bash
  apt-get install -y gitlab-runner
  usermod -aG docker gitlab-runner

  echo ""
  echo "Register the runner with your GitLab project:"
  echo "  gitlab-runner register \\"
  echo "    --url https://gitlab.com \\"
  echo "    --token <YOUR_RUNNER_TOKEN> \\"
  echo "    --executor shell \\"
  echo "    --description '${DOMAIN}-production'"
fi

# ── Print GitLab CI variable values ──────────────────────────────────────────
SERVER_IP=$(curl -s ifconfig.me)
HOST_KEY=$(ssh-keyscan -H "$SERVER_IP" 2>/dev/null | tail -1)

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " ✓ Setup complete!"
echo ""
echo " Landing : https://$DOMAIN"
echo " API     : https://$DOMAIN/api/v1/"
echo " Health  : https://$DOMAIN/health"
echo ""
echo " ── GitLab CI/CD → Settings → CI/CD → Variables ──"
echo " Add these as protected variables:"
echo ""
echo "  DEPLOY_HOST      = $SERVER_IP"
echo "  DEPLOY_USER      = $DEPLOY_USER"
echo "  DEPLOY_PORT      = 22"
echo "  DEPLOY_HOST_KEY  = $HOST_KEY"
echo "  DEPLOY_SSH_KEY   = <paste content of deploy private key>"
echo ""
echo " Generate deploy SSH key (run locally):"
echo "   ssh-keygen -t ed25519 -C deploy@inbetwin -f ~/.ssh/inbetwin_deploy"
echo "   # Add public key to /home/$DEPLOY_USER/.ssh/authorized_keys on this server"
echo "   # Add private key as DEPLOY_SSH_KEY variable in GitLab"
echo ""
echo " Next:"
echo "   1. Push repo to GitLab and add above variables"
echo "   2. Configure TELEGRAM_APP_ID + TELEGRAM_APP_HASH in .env"
echo "   3. Push to main → GitLab CI builds images + deploys"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
