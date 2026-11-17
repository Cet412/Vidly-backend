# Vidly Backend

Backend Go untuk aplikasi Android Vidly — pengunduh video/audio dari Instagram & TikTok via share-intent.

## Arsitektur
- **Framework:** Gin
- **Rate limiting:** token bucket in-memory, IP-based, 5 unduhan/jam (failed download tidak memakai token)
- **Downloader:** yt-dlp wrapper (best format, `--no-playlist`)
- **Proxy:** Nginx + Let's Encrypt (HTTPS, HTTP dialihkan ke HTTPS)
- **Service:** systemd (`vidly.service`)
- **API versioning:** `/api/v1/...`

## Struktur
```
cmd/server/          entrypoint, graceful shutdown, server timeouts
internal/api/        routes + handlers
internal/config/     konfigurasi via environment variable
internal/downloader/ wrapper yt-dlp
internal/ratelimit/  token bucket rate limiter
internal/middleware/ structured logger + panic handler (error generik ke client)
scripts/             setup_ec2.sh (one-time server setup)
.github/workflows/   CI (test) + CD (deploy by branch: staging->staging, main->production)
```

## Menjalankan lokal
```bash
cp .env.example .env
go run ./cmd/server
curl http://localhost:8080/health
```

Butuh `yt-dlp` terpasang lokal untuk test download penuh.

## Endpoint
| Method | Path              | Deskripsi                                  |
|--------|-------------------|--------------------------------------------|
| GET    | /health           | health check (200)                         |
| POST   | /api/v1/download  | body: `{"url": "..."}` — streaming file    |

Contoh:
```bash
curl -X POST http://localhost:8080/api/v1/download \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.tiktok.com/@user/video/123"}' -o video.mp4
```

## GitHub Secrets (repo settings)
| Secret          | Sumber                                    |
|-----------------|-------------------------------------------|
| SERVER_HOST     | IP public EC2                             |
| SERVER_USER     | `ubuntu`                                  |
| SERVER_SSH_KEY  | private key (.pem) khusus deploy, bukan key login utama |

Workflow deploy otomatis jalan saat push ke `staging` (deploy staging) atau `main` (deploy production).
Deploy gagal otomatis jika `/health` tidak 200.

## Setup EC2 (step-by-step)
1. Login AWS Console -> EC2 -> Launch Instance
   - Region terdekat (jkt misal: ap-southeast-3)
   - AMI: Ubuntu 22.04 LTS (Free Tier eligible)
   - Type: t2.micro
   - Key pair: buat baru `.pem`, simpan aman
   - Security Group:
     - SSH (22): My IP saja
     - HTTP (80): Anywhere
     - HTTPS (443): Anywhere
     - **JANGAN buka 8080 dari internet**
   - Storage: 30GB gp3 (free tier max)
2. Connect: `ssh -i key.pem ubuntu@<IP>`
3. Upload script: `scp -i key.pem scripts/setup_ec2.sh ubuntu@<IP>:/tmp`
4. Run: `sudo bash /tmp/setup_ec2.sh`
5. Buat file `.env` di server (`/opt/vidly/.env`) berdasarkan `.env.example`
6. Setelah pertama kali deploy via GitHub Actions:
   `sudo systemctl enable --now vidly && status vidly`
7. Let's Encrypt: `sudo certbot --nginx -d <domain>` — HTTPS + auto redirect
8. UptimeRobot -> monitor https://domain/health

## Prioritas Keamanan yang Sudah Diimplementasi
- HTTPS via Nginx + certbot (redirect otomatis)
- Secret via env file, gitignored; CI pakai GitHub Secrets
- Validasi input: URL length & whitelist host (Instagram/TikTok)
- Error generik ke client, detail di log
- Security Group minimal, ufw enabled, SSH key-only, root login off
- systemd auto-restart, graceful shutdown, server timeouts
- health endpoint, log rotation
