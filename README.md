# 🎨 parq-vision

<img src="logo.png" alt="parq-vision Logo" width="400" />

[![Go Version](https://img.shields.io/github/go-mod/go-version/mamorett/parq-vision)](https://github.com/mamorett/parq-vision)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`parq-vision` is a config-driven tool for generating high-quality image captions using Vision LLMs and storing them in an efficient Parquet database.

## Overview

`parq-vision` "looks" at your images using multimodal models (like GPT-4o, Claude 3.5 Sonnet, or local models via Ollama) and generates rich, descriptive captions. It allows you to define a custom schema for your metadata, including timestamps, scores, and free-text fields.

---

## ✨ Features

- **🚀 High Performance:** Go-based execution for quick file scanning, concurrency-controlled LLM querying, and Parquet serialization.
- **🧠 Multimodal LLM Captions:** Leverages advanced vision models to understand and richly describe image contents.
- **📋 Schema Customization:** Fully configurable database schema (timestamps, scores, custom properties, automatic modification tracking).
- **🎯 Idempotency:** Automatically checks existing records and skips already-processed images unless `--override` is active.
- **⚡ Advanced Terminal Feedback:** Displays an adaptive custom progress bar showing rate speed (`img/s`), estimated remaining time, and dynamic status labels (`✓`, `SKIP`, `✗`) mapped to currently processed files.
- **🛑 Graceful Shutdown:** Full signal handling (`Ctrl+C`) ensures that database changes are committed and safely closed if processing is interrupted.

---

## 🛠 Installation & Setup

### Prerequisites
- **Go**: Version 1.24+ is recommended (as specified in `go.mod`). [Download Go](https://go.dev/doc/install).

### 🛠 Installing gödel
This project uses [gödel](https://github.com/palantir/godel), a powerful build tool for Go. You have two options for using it:

#### 1. Using the project wrapper (Recommended)
You do not need to install gödel globally. The repository includes a `godelw` (gödel wrapper) script that automatically downloads and manages the correct version of gödel for this project.

To initialize gödel and check the version:
```bash
chmod +x godelw
./godelw version
```

#### 2. Installing the gödel CLI (Optional)
If you wish to use gödel in your own projects, you can install the `godelinit` tool to set up the wrapper in any repository:
```bash
go install github.com/palantir/godel/v2/godelinit@latest
```

### 🏗 Building from Source
1. **Clone the repository:**
   ```bash
   git clone https://github.com/mamorett/parq-vision.git
   cd parq-vision
   ```

2. **Build the application:**
   ```bash
   ./godelw build
   ```
   The compiled binary will be available in the workspace.

---

## Quick Start

1. Create a `vision.json` configuration file:
   ```json
    {
      "llm": {
        "base_url": "https://api.openai.com/v1",
        "api_key": "${OPENAI_API_KEY}",
        "model": "gpt-4o"
      },
      "prompt": "describe the image in detail",
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
3. Inspect your results natively:
   ```bash
   # Inspect both schema and all records
   parq-vision --inspect dataset.parquet

   # Inspect only the database schema
   parq-vision --schema dataset.parquet
   ```

---

## Advanced Schema Example

Below is an example config file `vision.json` setup to create a custom schema:

```json
{
  "llm": {
    "base_url": "https://api.openai.com/v1",
    "api_key": "${OPENAI_API_KEY}",
    "model": "gpt-4o"
  },
  "prompt": "describe the image in detail",
  "images": {
    "source": "/path/to/my/images",
    "recursive": true
  },
  "database": {
    "path": "./dataset.parquet"
  },
  "fields": [
    {
      "field_name": "prompt",
      "type": "prompt"
    },
    {
      "field_name": "description",
      "type": "caption"
    },
    {
      "field_name": "created_at",
      "type": "timestamp",
      "default": "current_timestamp"
    },
    {
      "field_name": "modified_at",
      "type": "modified_at"
    }
  ]
}
```

### Column Definitions in the Resulting Parquet Database:

1. **`id`**: Automatically included primary key (no configuration required). Stores the auto-generated unique UUID string identifier.
2. **`image_path`**: Automatically included column (no configuration required). Maps to each processed file's full absolute path.
3. **`prompt`**: Configured as `"prompt"`. This field stores the actual prompt instruction sent to the Vision LLM (e.g. `"describe the image in detail"`).
4. **`description`**: Configured as `"caption"`. This field stores the rich description generated by the Vision LLM.
5. **`created_at`**: Configured as `"timestamp"` with `default: "current_timestamp"`. Automatically captures the UTC date and time when the row was created.
6. **`modified_at`**: Configured as `"modified_at"`. Initialized as `NULL` and automatically updated with the current timestamp if the record is reprocessed/overridden later.

---

## CLI Reference

| Flag | Shorthand | Description |
|------|-----------|-------------|
| `-c`, `-config` | | Path to `vision.json` config file. (Required) |
| `-j`, `-concurrency` | `-j` | Number of parallel LLM workers (overrides config). |
| `-r`, `-recursive` | `-r` | Scan for images recursively in subdirectories. Overrides config. |
| `-b`, `-batch` | `-b` | Save progress to the Parquet file every X images. `0` disables periodic saving. |
| `-o`, `-override` | `-o` | Force re-processing of images already in the database (overrides config). |
| `-stop` | | Stop processing after X images. `0` disables (process all). |
| `-resize` | | Resize images to target Megapixels (e.g. `1.0`) in-memory. Maintains aspect ratio. `0` disables resizing. |
| `--inspect` | | Path to a Parquet database file to inspect/print natively. |
| `--schema` | | Path to a Parquet database file to inspect its schema natively. |
| `-h`, `-help` | | Show usage information. |

---

## Configuration Reference (`vision.json`)

### `prompt` (Optional)
The prompt sent to the Vision LLM to guide caption generation.

| Key | Type | Description |
|---|---|---|
| `prompt` | string | The instruction for the vision model (default: `"describe the image in detail"`). Supports environment variable substitution (e.g. `${VISION_PROMPT}`). |

### `llm` (Mandatory)
| Key | Type | Description |
|---|---|---|
| `base_url` | string | OpenAI-compatible API base URL. Supports `${ENV_VAR}`. |
| `api_key` | string | API key. Supports `${ENV_VAR}`. (Optional for some local endpoints). |
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

The `fields` array allows you to define custom columns for your Parquet file. Under the hood, these map to standard physical Parquet types, while their behavior is controlled by their configuration type.

#### Automatic Primary Key Columns:
These columns are automatically generated and populated by the tool for every row (no configuration required):

| Field Name | Parquet Schema Type | Behavior / Description |
|---|---|---|
| `id` | `STRING` | **Primary Key**. Auto-generated unique UUID string (e.g. `d4b8e219-c189-4978-8314-e5ad08ea5c63`) for record identifier consistency. |
| `image_path` | `STRING` | The full absolute path of the processed image file (e.g. `/path/to/my/images/ComfyUI_00006_.png`), resolved by the file collector. |

#### Configurable Field Types (in `vision.json`):

| Config Field `type` | Resulting Parquet Type | Behavior / How it is Populated |
|---|---|---|
| `caption` | `STRING` | Populated automatically with the description generated by the Vision LLM. *(Required to have at least one)* |
| `prompt` | `STRING` | Populated automatically with the actual LLM prompt instruction used for that run (e.g. `"describe the image in detail"`). |
| `free_text` | `STRING` | Created as an empty string column initialized to `NULL` (the LLM does not write to this field). Useful for manually writing notes/prompts later. |
| `timestamp` | `TIMESTAMP` | Captures the UTC timestamp when the row is first successfully written. Requires `"default": "current_timestamp"` in the field configuration. |
| `modified_at` | `TIMESTAMP` | Initialized as `NULL`. Automatically updates to the current UTC timestamp whenever a row is overridden or updated (using the `--override` flag). |
| `number` | `DOUBLE` | Created as a floating-point numeric column initialized to `NULL`. Useful for scoring or CLIP metrics. |

> [!NOTE]
> **Schema Type vs. Configuration Type:**
> Parquet is a data storage format with its own limited set of physical data types (`STRING`, `TIMESTAMP`, `DOUBLE`, etc.). 
> - The config types `caption`, `prompt`, and `free_text` all map to `STRING` in the database, but `caption` is populated with the generated LLM description, `prompt` is populated with the config's LLM instruction, and `free_text` starts as `NULL`.
> - Both `timestamp` and `modified_at` map to `TIMESTAMP` in the database, but `timestamp` is set when created while `modified_at` is set when updated.

---

## Environment Variables
Fields like `api_key`, `base_url`, and `prompt` support `${VAR}` substitution. This allows you to keep secrets out of your configuration files.

---

## License
MIT
