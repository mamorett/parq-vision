# 🎨 parq-vision

[![Go Version](https://img.shields.io/github/go-mod/go-version/trithemius/parq-vision)](https://github.com/trithemius/parq-vision)

`parq-vision` is a config-driven tool for generating high-quality image captions using Vision LLMs and storing them in an efficient Parquet database. It replaces the old ComfyUI/A1111 metadata extraction pipeline with a modern, LLM-powered vision system.

## Overview

Instead of relying on fragile metadata from specific generation tools, `parq-vision` "looks" at your images using multimodal models (like GPT-4o, Claude 3.5 Sonnet, or local models via Ollama) and generates rich, descriptive captions. It allows you to define a custom schema for your metadata, including timestamps, scores, and free-text fields.

## Installation

### Prerequisites
- Go 1.24 or higher

### From Source
```bash
go install github.com/trithemius/parq-vision/cmd/parq-vision@latest
```

## Quick Start

1. Create a `vision.json` configuration file:
   ```json
   {
     "llm": {
       "base_url": "https://api.openai.com/v1",
       "api_key": "${OPENAI_API_KEY}",
       "model": "gpt-4o"
     },
     "images": {
       "source": "./my_images",
       "recursive": true
     },
     "database": {
       "path": "./dataset.parquet"
     },
     "fields": [
       { "field_name": "caption", "type": "caption" },
       { "field_name": "created_at", "type": "timestamp", "default": "current_timestamp" }
     ]
   }
   ```
2. Run the tool:
   ```bash
   parq-vision -c vision.json
   ```
3. Inspect your results:
   ```bash
   # Using python/pandas
   python -c "import pandas as pd; print(pd.read_parquet('dataset.parquet'))"
   ```

## CLI Reference

| Flag | Default | Description |
|---|---|---|
| `-c`, `--config` | *(required)* | Path to `vision.json` |
| `--resize` | `0` | Resize images to this many megapixels (e.g. `1.0`) in-memory before sending to LLM. Maintains aspect ratio. Never modifies original files. `0` disables resizing. |
| `-h`, `--help` | — | Show usage information. |

## Configuration Reference (`vision.json`)

### `llm` (Mandatory)
| Key | Type | Description |
|---|---|---|
| `base_url` | string | OpenAI-compatible API base URL. Supports `${ENV_VAR}`. |
| `api_key` | string | API key. Supports `${ENV_VAR}`. |
| `model` | string | Model name (default: `"gpt-4o"`). |

### `images` (Mandatory)
| Key | Type | Description |
|---|---|---|
| `source` | string | Directory or file path. |
| `recursive` | boolean | Recurse into subdirectories (default: `false`). |
| `extensions` | string[] | List of extensions to match (default: `[".png", ".jpg", ".jpeg", ".webp"]`). |
| `file_list` | string | Path to a text file with one image path per line. |

### `database` (Mandatory)
| Key | Type | Description |
|---|---|---|
| `path` | string | Path to the output Parquet file. |
| `override` | boolean | If true, re-process images already in the database (default: `false`). |

### `fields` (Mandatory)
At least one field of type `caption` must be defined.

| Field Type | Behavior |
|---|---|
| `caption` | Stored as a string. Filled by the LLM response. |
| `timestamp` | Stored as int64 (nanoseconds). If `default: "current_timestamp"`, it records when the row was created. |
| `free_text` | Stored as a string. Initialized as NULL/Empty. |
| `modified_at` | Stored as int64 (nanoseconds). Automatically updated when a row is overridden. |
| `number` | Stored as float64. Initialized as NULL. |

**Note**: `image_path` is always included as the primary key.

## Environment Variables
Fields like `api_key`, `base_url`, and `prompt` support `${VAR}` substitution. This allows you to keep secrets out of your configuration files.

## License
MIT
