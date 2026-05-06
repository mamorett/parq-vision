package vision

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResizeImageIfNeeded(t *testing.T) {
	t.Run("Small image remains unchanged", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		require.NoError(t, err)

		resized, mime, err := resizeImageIfNeeded(buf.Bytes(), "image/png", 1000000)
		require.NoError(t, err)
		assert.Equal(t, buf.Bytes(), resized)
		assert.Equal(t, "image/png", mime)
	})

	t.Run("Large image is resized", func(t *testing.T) {
		// 2000x1000 = 2MP
		img := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
		// Fill with some color
		for x := 0; x < 2000; x++ {
			for y := 0; y < 1000; y++ {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}

		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		require.NoError(t, err)

		resized, mime, err := resizeImageIfNeeded(buf.Bytes(), "image/png", 1000000)
		require.NoError(t, err)
		assert.NotEqual(t, buf.Bytes(), resized)
		assert.Equal(t, "image/jpeg", mime)

		// Decode and check size
		img2, _, err := image.Decode(bytes.NewReader(resized))
		require.NoError(t, err)
		assert.LessOrEqual(t, img2.Bounds().Dx()*img2.Bounds().Dy(), 1000000+1000) // allow small margin
	})
}
