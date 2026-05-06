package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

type VisionConfig struct {
	LLM      LLMConfig      `json:"llm"`
	Prompt   string         `json:"prompt"`
	Images   ImagesConfig   `json:"images"`
	Database DatabaseConfig `json:"database"`
	Fields   []FieldDef     `json:"fields"`
}

type LLMConfig struct {
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	Concurrency int    `json:"concurrency"`
}

type ImagesConfig struct {
	Source     string   `json:"source"`
	Recursive  bool     `json:"recursive"`
	Extensions []string `json:"extensions"` // default [".png",".jpg",".jpeg",".webp"]
	FileList   string   `json:"file_list"`  // optional text file of paths
}

type DatabaseConfig struct {
	Path     string `json:"path"`
	Override bool   `json:"override"`
}

type FieldDef struct {
	FieldName string `json:"field_name"`
	Type      string `json:"type"`    // "caption", "timestamp", "free_text", "modified_at", "number"
	Default   string `json:"default"` // "current_timestamp" (only valid for "timestamp" type)
}

func LoadConfig(path string) (*VisionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Env var substitution
	dataStr := expandEnvVars(string(data))

	var cfg VisionConfig
	if err := json.Unmarshal([]byte(dataStr), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Apply defaults
	if cfg.Prompt == "" {
		cfg.Prompt = "describe the image in detail"
	}
	if len(cfg.Images.Extensions) == 0 {
		cfg.Images.Extensions = []string{".png", ".jpg", ".jpeg", ".webp"}
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "gpt-4o"
	}
	if cfg.LLM.Concurrency <= 0 {
		cfg.LLM.Concurrency = 1
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func expandEnvVars(input string) string {
	// Replaces ${VAR} with os.Getenv("VAR")
	re := regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)
	return re.ReplaceAllStringFunc(input, func(m string) string {
		varName := m[2 : len(m)-1]
		return os.Getenv(varName)
	})
}

var fieldNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validateConfig(cfg *VisionConfig) error {
	if cfg.LLM.BaseURL == "" {
		return fmt.Errorf("llm.base_url is required")
	}
	if cfg.Images.Source == "" && cfg.Images.FileList == "" {
		return fmt.Errorf("either images.source or images.file_list must be set")
	}
	if cfg.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}

	hasCaption := false
	seenFields := make(map[string]bool)
	seenFields["image_path"] = true // reserved

	for _, f := range cfg.Fields {
		if f.FieldName == "" {
			return fmt.Errorf("field_name cannot be empty")
		}
		if !fieldNameRe.MatchString(f.FieldName) {
			return fmt.Errorf("invalid field_name: %q (must be alphanumeric+underscore, no leading digit)", f.FieldName)
		}
		if seenFields[f.FieldName] {
			return fmt.Errorf("duplicate field_name: %q", f.FieldName)
		}
		seenFields[f.FieldName] = true

		switch f.Type {
		case "caption":
			hasCaption = true
		case "timestamp":
			if f.Default != "" && f.Default != "current_timestamp" {
				return fmt.Errorf("invalid default %q for timestamp field %q", f.Default, f.FieldName)
			}
		case "free_text", "modified_at", "number":
			if f.Default != "" {
				return fmt.Errorf("field %q of type %q does not support defaults", f.FieldName, f.Type)
			}
		default:
			return fmt.Errorf("invalid type %q for field %q", f.Type, f.FieldName)
		}
	}

	if !hasCaption {
		return fmt.Errorf("at least one field with type 'caption' must be present")
	}

	return nil
}
