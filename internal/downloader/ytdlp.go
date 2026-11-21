package downloader

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/yourusername/vidly-backend/internal/config"
)

type YtDlp struct {
	cfg *config.Config
}

func New(cfg *config.Config) *YtDlp {
	return &YtDlp{cfg: cfg}
}

// Update attempts to update yt-dlp in place and returns an error
// if the current installation is unusable; auto-update README'd behavior:
// failure to update trivially keeps the previous version working.
func (y *YtDlp) Update() error {
	cmd := exec.Command(y.cfg.YTDLPPath, "--update")
	out, err := cmd.CombinedOutput()
	fmt.Printf("ytdlp update: %s\n", out)
	return err
}

func (y *YtDlp) Version() (string, error) {
	out, err := exec.Command(y.cfg.YTDLPPath, "--version").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (y *YtDlp) Download(url, outputPath string) ([]byte, error) {
	// outputPath is like "/tmp/id.%(ext)s" — inject uploader name into template.
	// yt-dlp replaces %(uploader)s with the channel/creator name.
	template := outputPath
	// Insert uploader before extension placeholder: id_%(uploader|title)s.%(ext)s
	template = strings.Replace(template, ".%(ext)s", "_%(uploader|title)s.%(ext)s", 1)

	args := []string{
		"--restrict-filenames", // sanitize special chars in filenames
		"--no-playlist",
		"--no-warnings",
		"-o", template,
		"--newline",
		url,
	}
	cmd := exec.Command(y.cfg.YTDLPPath, args...)
	cmd.Dir = "/tmp"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("ytdlp: %w", err)
	}
	return out, nil
}
