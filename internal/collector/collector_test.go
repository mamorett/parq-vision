package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectImages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "collector-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create some test files
	files := []string{
		"img1.png",
		"img2.jpg",
		"img3.webp",
		"other.txt",
		"subdir/img4.png",
	}

	for _, f := range files {
		path := filepath.Join(tempDir, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte("test"), 0644))
	}

	extensions := []string{".png", ".jpg", ".webp"}

	t.Run("Non-recursive collection", func(t *testing.T) {
		imgs, err := CollectImages(tempDir, false, extensions, "")
		require.NoError(t, err)
		assert.Len(t, imgs, 3)
	})

	t.Run("Recursive collection", func(t *testing.T) {
		imgs, err := CollectImages(tempDir, true, extensions, "")
		require.NoError(t, err)
		assert.Len(t, imgs, 4)
	})

	t.Run("Subset of extensions", func(t *testing.T) {
		imgs, err := CollectImages(tempDir, true, []string{".webp"}, "")
		require.NoError(t, err)
		assert.Len(t, imgs, 1)
		assert.True(t, filepath.Base(imgs[0]) == "img3.webp")
	})

	t.Run("File list collection", func(t *testing.T) {
		listPath := filepath.Join(tempDir, "list.txt")
		content := filepath.Join(tempDir, "img1.png") + "\n" + filepath.Join(tempDir, "img2.jpg")
		require.NoError(t, os.WriteFile(listPath, []byte(content), 0644))

		imgs, err := CollectImages("", false, extensions, listPath)
		require.NoError(t, err)
		assert.Len(t, imgs, 2)
	})
}
