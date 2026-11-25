package downloader

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/yourusername/vidly-backend/internal/config"
)

type YtDlp struct {
	cfg *config.Config
}

func New(cfg *config.Config) *YtDlp {
	return &YtDlp{cfg: cfg}
}

// Update attempts to update yt-dlp in place.
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

// Download runs yt-dlp with hardened arguments.
// Uses exec.CommandContext for timeout/cancellation support.
func (y *YtDlp) Download(ctx context.Context, url, outputPath string) ([]byte, error) {
	args := []string{
		"--ignore-config",       // prevent reading attacker-controlled config
		"--no-playlist",         // prevent playlist expansion
		"--max-downloads", "1",  // extra safety
		"--no-warnings",
		"--restrict-filenames",  // sanitize special chars
		"--max-filesize", "100M",
		"--socket-timeout", "15",
		"--retries", "3",
		"-o", outputPath,
		"--",                    // prevent argument injection
		url,
	}

	cmd := exec.CommandContext(ctx, y.cfg.YTDLPPath, args...)
	cmd.Dir = filepath.Dir(outputPath)

	out, err := cmd.CombinedOutput()

	if err != nil {
		return out, fmt.Errorf("ytdlp: %w", err)
	}
	return out, nil
}
