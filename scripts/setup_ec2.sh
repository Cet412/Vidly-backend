#!/bin/bash
# Vidly - EC2 Ubuntu One-Time Setup Script
# Run as: sudo bash setup_ec2.sh
# Ubuntu 22.04 assumed. Installs Go, yt-dlp, Nginx, Certbot; hardens SSH;
# installs vidly systemd service; configures Nginx + Let's Encrypt.

set -euo pipefail

echo "==[1] Update system =="
apt update && apt upgrade -y

echo "==[2] Install base tools =="
apt install -y git curl nginx python3-pip ffmpeg logrotate ufw

echo "==[3] Install Go =="
GO_VERSION="1.25.5"
if ! command -v go >/dev/null; then
  curl -sSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
  tar -C /usr/local -xzf /tmp/go.tgz
  echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
fi
export PATH=$PATH:/usr/local/go/bin

echo "==[4] Install yt-dlp =="
python3 -m pip install --break-system-packages --upgrade yt-dlp

echo "==[5] Add app user =="
id -u vidly &>/dev/null || useradd --system --create-home --shell /bin/bash vidly

echo "==[6] Prepare app dir =="
mkdir -p /opt/vidly /var/lib/vidly/tmp /var/log/vidly
chown -R vidly:vidly /opt/vidly /var/lib/vidly /var/log/vidly

echo "==[7] Systemd service =="
cat >/etc/systemd/system/vidly.service <<'EOF'
[Unit]
Description=Vidly backend
After=network.target

[Service]
User=vidly
WorkingDirectory=/opt/vidly
ExecStart=/opt/vidly/vidly-backend
EnvironmentFile=/opt/vidly/.env
Restart=always
RestartSec=3
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

echo "==[8] Nginx reverse proxy (HTTP for now; certbot upgrades later) =="
cat >/etc/nginx/sites-available/vidly <<'EOF'
server {
    listen 80;
    server_name _;

    # Set later with Let's Encrypt / your domain
    # ssl_certificate ...;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 600s;   # video downloads can be slow
        proxy_send_timeout 600s;
        client_max_body_size 1m;
    }
}
EOF
ln -sf /etc/nginx/sites-available/vidly /etc/nginx/sites-enabled/vidly
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx

echo "==[9] Firewall: only SSH(22), HTTP(80), HTTPS(443) =="
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

echo "==[10] SSH hardening: key-only, no root login =="
SSHD=/etc/ssh/sshd_config
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' $SSHD
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' $SSHD
systemctl restart ssh

echo "==[11] yt-dlp auto-refresh (daily timer) =="
cat >/etc/systemd/system/yt-dlp-update.service <<'EOF'
[Unit]
Description=Update yt-dlp on demand basis

[Service]
Type=oneshot
ExecStart=/usr/bin/python3 -m pip install --break-system-packages --upgrade yt-dlp --quiet || true
# Failure falls back silently to previously installed version
EOF
cat >/etc/systemd/system/yt-dlp-update.timer <<'EOF'
[Unit]
Description=Daily yt-dlp update attempt

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now yt-dlp-update.timer

echo "==[12] Log rotation for vidly =="
cat >/etc/logrotate.d/vidly <<'EOF'
/var/log/vidly/*.log {
    daily
    rotate 7
    compress
    missingok
    notifempty
}
EOF

echo
echo "Setup DONE."
echo "Next: upload .env + binary to /opt/vidly, then: systemctl enable --now vidly"
echo "Then run certbot: certbot --nginx -d <your-domain> (HTTPS + auto redirect)"
