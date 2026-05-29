package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
)

const (
	downloadTimeout = 5 * time.Minute
	maxDownloadSize = 500 * 1024 * 1024 // 500MB
)

// UpdateRequest contains the information needed to apply an update.
type UpdateRequest struct {
	Version     string
	DownloadURL string
	SHA256      string
	Signature   []byte
}

// Updater handles downloading, verifying, and applying binary updates.
type Updater struct {
	currentPath string
	backupPath  string
	downloadDir string
	publicKey   ed25519.PublicKey
	client      *http.Client
	logger      zerolog.Logger
}

// New creates a new Updater instance.
func New(currentPath, backupPath, downloadDir string, pub ed25519.PublicKey, logger zerolog.Logger) *Updater {
	return &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
		downloadDir: downloadDir,
		publicKey:   pub,
		client: &http.Client{
			Timeout: downloadTimeout,
		},
		logger: logger.With().Str("component", "updater").Logger(),
	}
}

// Apply downloads, verifies, and applies the update.
func (u *Updater) Apply(req UpdateRequest) error {
	u.logger.Info().Str("version", req.Version).Str("url", req.DownloadURL).Msg("starting update")

	data, err := u.download(req.DownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if err := u.verifyChecksum(data, req.SHA256); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := u.verifySignature(data, req.Signature); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	tmpPath, err := u.writeTemp(data)
	if err != nil {
		return fmt.Errorf("writing temp file failed: %w", err)
	}

	if err := u.swap(tmpPath); err != nil {
		return fmt.Errorf("swap failed: %w", err)
	}

	u.logger.Info().Str("version", req.Version).Msg("update applied successfully")
	return nil
}

// Rollback restores the binary from the backup.
func (u *Updater) Rollback() error {
	u.logger.Info().Msg("rolling back to backup")

	if _, err := os.Stat(u.backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if err := os.Rename(u.backupPath, u.currentPath); err != nil {
		return fmt.Errorf("rollback rename failed: %w", err)
	}

	u.logger.Info().Msg("rollback completed")
	return nil
}

// download fetches content from the given URL with size limits.
func (u *Updater) download(url string) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %w", err)
	}

	if int64(len(data)) > maxDownloadSize {
		return nil, fmt.Errorf("download exceeds maximum size of %d bytes", maxDownloadSize)
	}

	return data, nil
}

// verifyChecksum checks the SHA256 hash of the data against the expected value.
func (u *Updater) verifyChecksum(data []byte, expected string) error {
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])

	if !bytes.Equal([]byte(actual), []byte(expected)) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// verifySignature validates the ed25519 signature of the data.
func (u *Updater) verifySignature(data, sig []byte) error {
	if !ed25519.Verify(u.publicKey, data, sig) {
		return fmt.Errorf("ed25519 signature verification failed")
	}

	return nil
}

// writeTemp writes the data to a temporary file in the download directory.
func (u *Updater) writeTemp(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp(u.downloadDir, "update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file failed: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing temp file failed: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("closing temp file failed: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0o755); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("setting permissions failed: %w", err)
	}

	return tmpFile.Name(), nil
}

// swap replaces the current binary with the new one, keeping a backup.
func (u *Updater) swap(newPath string) error {
	if err := os.Rename(u.currentPath, u.backupPath); err != nil {
		return fmt.Errorf("backing up current binary failed: %w", err)
	}

	if err := os.Rename(newPath, u.currentPath); err != nil {
		// Attempt to restore backup on failure.
		if rbErr := os.Rename(u.backupPath, u.currentPath); rbErr != nil {
			return fmt.Errorf("installing new binary failed and rollback also failed: install=%w, rollback=%v", err, rbErr)
		}
		return fmt.Errorf("installing new binary failed (backup restored): %w", err)
	}

	return nil
}
