package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourusername/vidly-backend/internal/config"
	"github.com/yourusername/vidly-backend/internal/downloader"
	"github.com/yourusername/vidly-backend/internal/ratelimit"
)

var supportedHosts = regexp.MustCompile(`(?i)^(www\.)?(instagram\.com|instagr\.am|tiktok\.com|vt\.tiktok\.com|vm\.tiktok\.com|youtube\.com|youtu\.be)$`)

type Handler struct {
	cfg      *config.Config
	limiter  *ratelimit.Limiter
	ytdlp    *downloader.YtDlp
}

func NewHandler(cfg *config.Config, limiter *ratelimit.Limiter, ytdlp *downloader.YtDlp) *Handler {
	return &Handler{cfg: cfg, limiter: limiter, ytdlp: ytdlp}
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
}

type downloadRequest struct {
	URL string `json:"url"`
}

// Download handles POST /api/v1/download.
// Flow: validate input -> rate limit -> run yt-dlp -> stream file -> cleanup.
// A failed download refunds the rate-limit token.
func (h *Handler) Download(c *gin.Context) {
	var req downloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "format request tidak valid")
		return
	}

	if len(req.URL) == 0 || len(req.URL) > h.cfg.MaxURLLength {
		respondError(c, http.StatusBadRequest, "URL tidak valid")
		return
	}

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respondError(c, http.StatusBadRequest, "URL tidak valid")
		return
	}
	if !supportedHosts.MatchString(parsed.Hostname()) {
		respondError(c, http.StatusBadRequest, "platform belum didukung")
		return
	}

	ip := c.ClientIP()
	if !h.limiter.Allow(ip) {
		respondError(c, http.StatusTooManyRequests, "batas unduhan tercapai, coba lagi dalam 1 jam")
		return
	}

	if err := os.MkdirAll(h.cfg.TempDir, 0o750); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "terjadi kesalahan"})
		return
	}
	id, err := randomID()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "terjadi kesalahan")
		return
	}
	// yt-dlp adds its own extension; use an output template.
 tmpl := filepath.Join(h.cfg.TempDir, id+".%(ext)s")

	out, err := h.ytdlp.Download(req.URL, tmpl)
	if err != nil {
		h.limiter.Refund(ip) // failed download does not consume quota
		fmt.Printf("ytdlp failed: %s\n", string(out))
		respondError(c, http.StatusBadGateway, "gagal mengunduh dari sumber, coba lagi nanti")
		return
	}

	path, err := findOutputFile(h.cfg.TempDir, id)
	if err != nil {
		h.limiter.Refund(ip)
		respondError(c, http.StatusBadGateway, "gagal mengunduh dari sumber, coba lagi nanti")
		return
	}
	defer os.Remove(path)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	c.Header("Content-Length", fmt.Sprintf("%d", fileSize(path)))
	c.File(path)
}

// findOutputFile locates the file yt-dlp wrote, whose extension may vary.
func findOutputFile(dir, id string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, id+".*"))
	if err != nil || len(matches) == 0 {
		return "", errors.New("output file not found")
	}
	return matches[0], nil
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func respondError(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}
