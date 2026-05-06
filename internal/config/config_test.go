package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("Valid full config", func(t *testing.T) {
		content := `{
			"llm": {
				"base_url": "https://api.openai.com/v1",
				"api_key": "sk-123",
				"model": "gpt-4o"
			},
			"prompt": "custom prompt",
			"images": {
				"source": "./images",
				"recursive": true
			},
			"database": {
				"path": "./output.parquet"
			},
			"fields": [
				{ "field_name": "caption", "type": "caption" }
			]
		}`
		tmpFile, err := os.CreateTemp("", "vision-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		cfg, err := LoadConfig(tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.BaseURL)
		assert.Equal(t, "custom prompt", cfg.Prompt)
		assert.True(t, cfg.Images.Recursive)
		assert.ElementsMatch(t, []string{".png", ".jpg", ".jpeg", ".webp"}, cfg.Images.Extensions)
	})

	t.Run("Env var substitution", func(t *testing.T) {
		os.Setenv("TEST_API_KEY", "env-secret")
		defer os.Unsetenv("TEST_API_KEY")

		content := `{
			"llm": {
				"base_url": "https://api.openai.com/v1",
				"api_key": "${TEST_API_KEY}"
			},
			"images": { "source": "./" },
			"database": { "path": "out.parquet" },
			"fields": [ { "field_name": "c", "type": "caption" } ]
		}`
		tmpFile, err := os.CreateTemp("", "vision-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString(content)
		tmpFile.Close()

		cfg, err := LoadConfig(tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, "env-secret", cfg.LLM.APIKey)
	})

	t.Run("Missing required fields", func(t *testing.T) {
		content := `{
			"llm": { "api_key": "k" },
			"images": { "source": "./" },
			"database": { "path": "out.parquet" },
			"fields": [ { "field_name": "c", "type": "caption" } ]
		}`
		tmpFile, err := os.CreateTemp("", "vision-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString(content)
		tmpFile.Close()

		_, err = LoadConfig(tmpFile.Name())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "llm.base_url is required")
	})

	t.Run("Invalid field name", func(t *testing.T) {
		content := `{
			"llm": { "base_url": "http://localhost", "api_key": "k" },
			"images": { "source": "./" },
			"database": { "path": "out.parquet" },
			"fields": [
				{ "field_name": "1invalid", "type": "caption" }
			]
		}`
		tmpFile, err := os.CreateTemp("", "vision-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString(content)
		tmpFile.Close()

		_, err = LoadConfig(tmpFile.Name())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field_name")
	})

	t.Run("Missing caption field", func(t *testing.T) {
		content := `{
			"llm": { "base_url": "http://localhost", "api_key": "k" },
			"images": { "source": "./" },
			"database": { "path": "out.parquet" },
			"fields": [
				{ "field_name": "ts", "type": "timestamp" }
			]
		}`
		tmpFile, err := os.CreateTemp("", "vision-*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString(content)
		tmpFile.Close()

		_, err = LoadConfig(tmpFile.Name())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one field with type 'caption' must be present")
	})
}
