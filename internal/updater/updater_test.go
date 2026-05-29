package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUpdater(t *testing.T) *Updater {
	t.Helper()
	tmpDir := t.TempDir()
	return &Updater{
		currentPath: filepath.Join(tmpDir, "current"),
		backupPath:  filepath.Join(tmpDir, "backup"),
		downloadDir: tmpDir,
		client:      &http.Client{},
		logger:      zerolog.Nop(),
	}
}

func TestUpdaterVerifyChecksum(t *testing.T) {
	u := newTestUpdater(t)
	data := []byte("test binary content")

	hash := sha256.Sum256(data)
	validChecksum := hex.EncodeToString(hash[:])

	t.Run("correct checksum passes", func(t *testing.T) {
		err := u.verifyChecksum(data, validChecksum)
		assert.NoError(t, err)
	})

	t.Run("incorrect checksum fails", func(t *testing.T) {
		err := u.verifyChecksum(data, "0000000000000000000000000000000000000000000000000000000000000000")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("empty data with correct checksum", func(t *testing.T) {
		emptyHash := sha256.Sum256([]byte{})
		emptyChecksum := hex.EncodeToString(emptyHash[:])
		err := u.verifyChecksum([]byte{}, emptyChecksum)
		assert.NoError(t, err)
	})
}

func TestUpdaterVerifySignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	u := newTestUpdater(t)
	u.publicKey = pub

	data := []byte("data to sign")
	validSig := ed25519.Sign(priv, data)

	t.Run("valid signature passes", func(t *testing.T) {
		err := u.verifySignature(data, validSig)
		assert.NoError(t, err)
	})

	t.Run("invalid signature fails", func(t *testing.T) {
		badSig := make([]byte, ed25519.SignatureSize)
		err := u.verifySignature(data, badSig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature verification failed")
	})

	t.Run("wrong data fails", func(t *testing.T) {
		err := u.verifySignature([]byte("wrong data"), validSig)
		assert.Error(t, err)
	})

	t.Run("wrong key fails", func(t *testing.T) {
		otherPub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		u.publicKey = otherPub
		defer func() { u.publicKey = pub }()

		err = u.verifySignature(data, validSig)
		assert.Error(t, err)
	})
}

func TestUpdaterDownload(t *testing.T) {
	binaryContent := []byte("fake binary content for testing")

	t.Run("successful download", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(binaryContent)
		}))
		defer srv.Close()

		u := newTestUpdater(t)
		data, err := u.download(srv.URL)
		require.NoError(t, err)
		assert.Equal(t, binaryContent, data)
	})

	t.Run("server error fails", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		u := newTestUpdater(t)
		_, err := u.download(srv.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code")
	})

	t.Run("invalid URL fails", func(t *testing.T) {
		u := newTestUpdater(t)
		_, err := u.download("http://localhost:1")
		assert.Error(t, err)
	})

	t.Run("empty response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		u := newTestUpdater(t)
		data, err := u.download(srv.URL)
		require.NoError(t, err)
		assert.Empty(t, data)
	})
}

func TestUpdaterSwap(t *testing.T) {
	tmpDir := t.TempDir()

	currentPath := filepath.Join(tmpDir, "current")
	backupPath := filepath.Join(tmpDir, "backup")
	newPath := filepath.Join(tmpDir, "new")

	require.NoError(t, os.WriteFile(currentPath, []byte("old content"), 0o644))
	require.NoError(t, os.WriteFile(newPath, []byte("new content"), 0o755))

	u := &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
		downloadDir: tmpDir,
		logger:      zerolog.Nop(),
	}

	err := u.swap(newPath)
	require.NoError(t, err)

	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, "old content", string(backupData))

	currentData, err := os.ReadFile(currentPath)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(currentData))

	_, err = os.Stat(newPath)
	assert.True(t, os.IsNotExist(err), "new file should be removed after swap")
}

func TestUpdaterSwapRollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	currentPath := filepath.Join(tmpDir, "current")
	backupPath := filepath.Join(tmpDir, "backup")

	require.NoError(t, os.WriteFile(currentPath, []byte("original"), 0o644))

	u := &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
		downloadDir: tmpDir,
		logger:      zerolog.Nop(),
	}

	err := u.swap(filepath.Join(tmpDir, "nonexistent"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "installing new binary failed")

	data, err := os.ReadFile(currentPath)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data), "original should be restored after failed swap")
}

func TestRollback(t *testing.T) {
	tmpDir := t.TempDir()

	currentPath := filepath.Join(tmpDir, "current")
	backupPath := filepath.Join(tmpDir, "backup")

	require.NoError(t, os.WriteFile(backupPath, []byte("backup content"), 0o644))

	u := &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
		downloadDir: tmpDir,
		logger:      zerolog.Nop(),
	}

	err := u.Rollback()
	require.NoError(t, err)

	data, err := os.ReadFile(currentPath)
	require.NoError(t, err)
	assert.Equal(t, "backup content", string(data))
}

func TestRollbackNoBackup(t *testing.T) {
	tmpDir := t.TempDir()

	u := &Updater{
		currentPath: filepath.Join(tmpDir, "current"),
		backupPath:  filepath.Join(tmpDir, "nonexistent"),
		downloadDir: tmpDir,
		logger:      zerolog.Nop(),
	}

	err := u.Rollback()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "backup not found")
}
