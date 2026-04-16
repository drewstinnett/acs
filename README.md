# acs

A command-line tool for downloading, OCR-processing, and browsing **ACS (Army Correspondence School / American Chemical Society / etc.) microfilm** hosted on the [Internet Archive](https://archive.org).

`acs` searches the Internet Archive for microfilm items, downloads their page images, runs Tesseract OCR locally, and stores everything in a SQLite database. You can then browse the results through a built-in web UI or export them as a static, searchable website.

## Features

- **Search** the Internet Archive and save matching items to a local catalog
- **Download** microfilm page images in parallel, with resume support for partial items
- **OCR** pages with Tesseract (configurable language, parallel workers)
- **Serve** a local web browser for reading and searching the archive
- **Export** the entire archive as a static site (works with [Pagefind](https://pagefind.app) for full-text search) — deployable to GitHub Pages

## Requirements

- Go 1.25+
- [Tesseract OCR](https://github.com/tesseract-ocr/tesseract)
- [ImageMagick](https://imagemagick.org) (for image conversion)

On Debian/Ubuntu:

```sh
sudo apt install tesseract-ocr imagemagick
```

## Install

```sh
go install github.com/drewstinnett/acs@latest
```

Or build from source:

```sh
git clone https://github.com/drewstinnett/acs
cd acs
go build -o acs .
```

## Quick start

```sh
# 1. Find some microfilm on Internet Archive and save matches to the catalog
acs search "army correspondence school" --save

# 2. Download page images for everything pending
acs download --workers 4

# 3. Run OCR on the downloaded pages
acs ocr --workers 2 --lang eng

# 4. Browse the results in your web browser
acs serve --open
```

## Commands

| Command | Description |
|---|---|
| `acs search <query>` | Search Internet Archive; `--save` adds results to the catalog |
| `acs download` | Download images for pending/partial items (`--item` for a single id) |
| `acs ocr` | Run Tesseract OCR on downloaded pages (`--rerun` to reprocess) |
| `acs serve` | Start the local web UI (default `http://127.0.0.1:8080`) |
| `acs export --out-dir <dir>` | Generate a static site from the database |

Run `acs <command> --help` for the full flag list.

## Static site export

After exporting, build a Pagefind search index over the output:

```sh
acs export --out-dir public
npx pagefind --site public
```

The `public/` directory is then a self-contained, searchable static site. A GitHub Actions workflow under `.github/workflows/` builds and publishes it to GitHub Pages.

## Storage layout

By default everything lives under `~/.acs/`:

```
~/.acs/
├── acs.db          # SQLite catalog (items, pages, OCR text)
└── images/         # Downloaded page images
```

Override with `--db` and `--data-dir` on any command.

## License

See repository for license details.
