# 🎨 parq-vision

```
  ____   _    ____   ___   __     _____ ____ ___ ___  _   _ 
 |  _ \ / \  |  _ \ / _ \  \ \   / /_ _/ ___|_ _/ _ \| \ | |
 | |_) / _ \ | |_) | | | |  \ \ / / | |\___ \| | | | |  \| |
 |  __/ ___ \|  _ <| |_| |   \ V /  | | ___) | | |_| | |\  |
 |_| /_/   \_\_| \_\\__\_\    \_/  |___|____/___\___/|_| \_|
```

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
      "type": "free_text"
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

1. **`image_path`**: Automatically included as the primary key by the tool (no configuration required). Maps to each processed file's absolute path.
2. **`prompt`**: Configured as `"free_text"`. Because `parq-vision` focus is vision captioning, this optional string field is initialized as `NULL`.
3. **`description`**: Configured as `"caption"`. This field stores the rich description generated by the Vision LLM.
4. **`created_at`**: Configured as `"timestamp"` with `default: "current_timestamp"`. Automatically captures the UTC date and time when the row was created.
5. **`modified_at`**: Configured as `"modified_at"`. Initialized as `NULL` and automatically updated with the current timestamp if the record is reprocessed/overridden later.

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

The `type` property in `vision.json` is a **behavioral type** that controls how `parq-vision` populates that column. Under the hood, these map directly to native **Parquet Schema Types** inside the database file.

Here is the complete list of all **5** supported configuration field types:

#### 1. `caption`
- **Resulting Parquet Schema Type:** `STRING` (or `BYTE_ARRAY` with UTF-8 logical type)
- **Behavior:** Automatically populated with the text description generated by the Vision LLM. *(You must define at least one `caption` field).*
- **Best Use Case:** Storing the primary model output, descriptive tags, or visual analysis.

#### 2. `free_text`
- **Resulting Parquet Schema Type:** `STRING` (or `BYTE_ARRAY` with UTF-8 logical type)
- **Behavior:** Created as an empty string/text column initialized to `NULL`. The LLM does not write to this field.
- **Best Use Case:** Custom user notes, category tags, or original metadata prompts you want to fill later using external scripts.

#### 3. `timestamp`
- **Resulting Parquet Schema Type:** `TIMESTAMP(isAdjustedToUTC=true,unit=NANOS)` (`INT64`)
- **Behavior:** Captures the UTC timestamp when the row is first successfully written. Requires `"default": "current_timestamp"` in the field configuration.
- **Best Use Case:** Recording exactly when an image was processed.

#### 4. `modified_at`
- **Resulting Parquet Schema Type:** `TIMESTAMP(isAdjustedToUTC=true,unit=NANOS)` (`INT64`)
- **Behavior:** Initialized as `NULL`. Automatically updates to the current UTC timestamp whenever a row is overridden or updated (using the `--override` flag).
- **Best Use Case:** Auditing changes, reprocessing runs, or modifications in your dataset.

#### 5. `number`
- **Resulting Parquet Schema Type:** `DOUBLE`
- **Behavior:** Created as a floating-point numeric column initialized to `NULL`. The LLM does not write to this field.
- **Best Use Case:** Aesthetic scoring, CLIP matching scores, or numeric evaluation metrics.

> [!NOTE]
> **Schema Type vs. Configuration Type:**
> Parquet is a data storage format with its own limited set of physical data types (`STRING`, `TIMESTAMP`, `DOUBLE`, etc.). 
> - Both `caption` and `free_text` map to `STRING` in the database, but `caption` is populated by the LLM while `free_text` starts as `NULL`.
> - Both `timestamp` and `modified_at` map to `TIMESTAMP` in the database, but `timestamp` is set when created while `modified_at` is set when updated.

**Note**: `image_path` is always automatically included as the primary key (`STRING` type).

---

## Environment Variables
Fields like `api_key`, `base_url`, and `prompt` support `${VAR}` substitution. This allows you to keep secrets out of your configuration files.

---

## License
MIT
