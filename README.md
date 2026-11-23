# pin-saver-cli

A command-line tool to download Pinterest boards via the Pinterest API v5.

## Features

- **Pinterest API v5 Compatibility**: Properly handles Pin responses and selects the highest resolution image (`originals` > `1200x` > `736x` > `600x` > `400x300` > `150x150`).
- **Full Pagination Support**: Seamlessly traverses bookmarks to retrieve all pins on large boards without hitting page limits.
- **Concurrent Downloads**: Fast parallel downloading with configurable worker concurrency.
- **Resilient & Atomic Downloads**: Saves files using temporary staging to prevent incomplete or corrupt images on network failure.
- **Deduplication**: Automatically skips already downloaded images so you can re-run the tool to sync newly added pins.
- **Interactive & CLI Flags**: Run interactively or supply arguments via CLI flags and environment variables.
- **Board Discovery**: Easily list your Pinterest boards and their numeric IDs directly from the CLI.


## Installation & Build

Ensure you have [Go](https://go.dev/) (1.23+) installed.

```bash
# Clone the repository
git clone https://github.com/ohSystemmm/pin-saver-cli.git
cd pin-saver-cli

# Build binary
go build -o pin-saver.exe .
```


## Authentication

Generate an access token in the [Pinterest Developer Portal](https://developers.pinterest.com/apps/):
- Required scopes: `boards:read` and `pins:read` (or `boards:read_secret` / `pins:read_secret` for secret boards).
- You can provide your token via the `-token` flag, or set the environment variable:
  ```bash
  export PINTEREST_ACCESS_TOKEN="your_access_token_here"
  # Or on Windows PowerShell:
  $env:PINTEREST_ACCESS_TOKEN = "your_access_token_here"
  ```

---

## Usage

### 1. Interactive Mode
Simply run the executable without flags, and it will prompt you for any missing inputs:

```bash
./pin-saver.exe
```

When prompted for the board ID, you can type `list` to view all your boards and their numeric IDs.

### 2. Command-Line Flags

```bash
# Download pins from a specific board to ./downloads (5 concurrent downloads)
./pin-saver.exe -token "YOUR_TOKEN" -board "123456789012345678"

# Custom output directory and concurrency
./pin-saver.exe -board "123456789012345678" -dir "./my_board_images" -concurrency 10

# List all boards for the authenticated account and exit
./pin-saver.exe -list-boards
```

### Available Flags

| Flag | Type | Description | Default |
|---|---|---|---|
| `-token` | string | Pinterest access token / API key (or `PINTEREST_ACCESS_TOKEN` env) | `""` |
| `-board` | string | Numeric Pinterest Board ID | `""` |
| `-dir` | string | Destination directory to save images | `./downloads` |
| `-concurrency` | int | Number of concurrent downloads | `5` |
| `-list-boards` | bool | List user's boards and exit | `false` |

---

## Running Tests

Run the unit tests:

```bash
go test -v ./...
```
