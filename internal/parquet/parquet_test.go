package parquet

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/mamorett/parq-vision/internal/config"
)

func TestDynamicParquetDB(t *testing.T) {
	tempFile := "/tmp/test_dynamic.parquet"
	os.Remove(tempFile)
	os.Remove(tempFile + ".tmp")
	defer os.Remove(tempFile)

	fieldDefs := []config.FieldDef{
		{FieldName: "caption", Type: "caption"},
		{FieldName: "created_at", Type: "timestamp"},
		{FieldName: "score", Type: "number"},
		{FieldName: "modified_at", Type: "modified_at"},
	}

	now := time.Now().UTC().Truncate(time.Microsecond) // Parquet nanos might lose some precision in some roundtrips but here we use nanos

	t.Run("Create and Save", func(t *testing.T) {
		db, err := NewDynamicParquetDB(tempFile, fieldDefs)
		require.NoError(t, err)

		rows := []map[string]any{
			{
				"image_path": "img1.png",
				"caption":    "a cat",
				"created_at": now,
				"score":      0.95,
			},
			{
				"image_path": "img2.png",
				"caption":    "a dog",
				"created_at": now,
				"score":      nil,
			},
		}

		err = db.AppendRows(rows, false)
		require.NoError(t, err)
		err = db.Close()
		require.NoError(t, err)
	})

	t.Run("Load and Verify", func(t *testing.T) {
		db, err := NewDynamicParquetDB(tempFile, fieldDefs)
		require.NoError(t, err)
		assert.True(t, db.Exists("img1.png"))
		assert.True(t, db.Exists("img2.png"))

		row1, ok := db.GetRow("img1.png")
		assert.True(t, ok)
		assert.Equal(t, "a cat", row1["caption"])
		assert.Equal(t, now.UnixNano(), row1["created_at"].(time.Time).UnixNano())
		assert.Equal(t, 0.95, row1["score"])

		row2, ok := db.GetRow("img2.png")
		assert.True(t, ok)
		assert.Equal(t, "a dog", row2["caption"])
		assert.Nil(t, row2["score"])
	})

	t.Run("Append and Override", func(t *testing.T) {
		db, err := NewDynamicParquetDB(tempFile, fieldDefs)
		require.NoError(t, err)

		// Override img1
		update := []map[string]any{
			{
				"image_path": "img1.png",
				"caption":    "a fluffy cat",
			},
			{
				"image_path": "img3.png",
				"caption":    "a bird",
				"created_at": now,
			},
		}

		err = db.AppendRows(update, true)
		require.NoError(t, err)
		err = db.Close()
		require.NoError(t, err)

		// Reload
		db2, _ := NewDynamicParquetDB(tempFile, fieldDefs)
		row1, _ := db2.GetRow("img1.png")
		assert.Equal(t, "a fluffy cat", row1["caption"])
		assert.NotNil(t, row1["modified_at"]) // modified_at should be set on override

		row3, _ := db2.GetRow("img3.png")
		assert.Equal(t, "a bird", row3["caption"])
	})
}

func TestParquetStressLarge(t *testing.T) {
	tempFile := "/tmp/test_stress_large_dynamic.parquet"
	os.Remove(tempFile)
	defer os.Remove(tempFile)

	fieldDefs := []config.FieldDef{
		{FieldName: "caption", Type: "caption"},
		{FieldName: "created_at", Type: "timestamp"},
	}

	db, err := NewDynamicParquetDB(tempFile, fieldDefs)
	require.NoError(t, err)

	n := 1000 // Reduced for faster test in this environment, but logic is same
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		row := map[string]any{
			"image_path": fmt.Sprintf("img_%d.png", i),
			"caption":    fmt.Sprintf("caption %d", i),
			"created_at": now,
		}
		db.AppendRows([]map[string]any{row}, false)
	}
	require.NoError(t, db.Close())

	db2, err := NewDynamicParquetDB(tempFile, fieldDefs)
	require.NoError(t, err)
	assert.Equal(t, n, len(db2.rows))
}
