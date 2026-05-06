package integration

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trithemius/parq-vision/internal/collector"
	"github.com/trithemius/parq-vision/internal/config"
	"github.com/trithemius/parq-vision/internal/parquet"
	"github.com/trithemius/parq-vision/internal/vision"
)

func TestFullPipeline(t *testing.T) {
	// 1. Mock OpenAI Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Check that we got a valid request
		var reqBody map[string]any
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		fmt.Fprint(w, `{
			"choices": [{
				"message": {
					"content": "a mock caption"
				}
			}]
		}`)
	}))
	defer server.Close()

	// 2. Setup temp environment
	tempDir, err := os.MkdirTemp("", "parq-vision-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	imgPath := filepath.Join(tempDir, "test.png")
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	f, err := os.Create(imgPath)
	require.NoError(t, err)
	err = png.Encode(f, img)
	require.NoError(t, err)
	f.Close()

	dbPath := filepath.Join(tempDir, "out.parquet")
	cfgContent := fmt.Sprintf(`{
		"llm": {
			"base_url": %q,
			"api_key": "dummy",
			"model": "gpt-4o"
		},
		"images": { "source": %q },
		"database": { "path": %q },
		"fields": [ 
			{ "field_name": "caption", "type": "caption" },
			{ "field_name": "created_at", "type": "timestamp", "default": "current_timestamp" }
		]
	}`, server.URL, tempDir, dbPath)
	
	cfgPath := filepath.Join(tempDir, "vision.json")
	err = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	require.NoError(t, err)

	// 3. Execute Pipeline Logic
	cfg, err := config.LoadConfig(cfgPath)
	require.NoError(t, err)

	imagePaths, err := collector.CollectImages(cfg.Images.Source, cfg.Images.Recursive, cfg.Images.Extensions, "")
	require.NoError(t, err)
	assert.Len(t, imagePaths, 1)

	db, err := parquet.NewDynamicParquetDB(cfg.Database.Path, cfg.Fields)
	require.NoError(t, err)

	client := vision.NewVisionClient(cfg.LLM)
	for _, p := range imagePaths {
		caption, err := client.DescribeImage(p, cfg.Prompt, 0)
		require.NoError(t, err)

		row := map[string]any{
			"image_path": p,
		}
		for _, f := range cfg.Fields {
			switch f.Type {
			case "caption":
				row[f.FieldName] = caption
			case "timestamp":
				if f.Default == "current_timestamp" {
					row[f.FieldName] = time.Now().UTC()
				}
			}
		}
		err = db.AppendRows([]map[string]any{row}, false)
		require.NoError(t, err)
	}
	err = db.Close()
	require.NoError(t, err)

	// 4. Verify Output
	db2, err := parquet.NewDynamicParquetDB(dbPath, cfg.Fields)
	require.NoError(t, err)
	assert.True(t, db2.Exists(imgPath))
	row, ok := db2.GetRow(imgPath)
	assert.True(t, ok)
	assert.Equal(t, "a mock caption", row["caption"])
	assert.NotNil(t, row["created_at"])
}
