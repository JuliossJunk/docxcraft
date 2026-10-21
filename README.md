# DocxCraft

[Русский](README.ru.md)

Agent harness (MCP server + CLI + Go SDK) for **deterministic** generation, targeted editing, and linting of Microsoft Word (`.docx`) documents.

Core idea: edit OpenXML **without losing layout** (in-memory ZIP + DOM), address paragraphs via stable `w14:paraId` anchors, and drive formatting from YAML style profiles (GOST / Corporate / Legal).

## Features

- **Build from JSON AST** — the LLM supplies structure; Go produces a valid `.docx` using a style profile
- **Anchor patches** — `replace_text`, `insert_after`, `delete`, tracked changes, format cloning
- **Template healing** — `HealRuns` restores Word-split `{{ … }}` placeholders
- **ECMA-376 normalizer** — correct child order in `pPr`/`rPr`/`tcPr` so Word does not report a corrupt file
- **Linter DF-001…DF-020** — geometry, headings, tables, fonts; optional Visual Gate via LibreOffice → PDF/PNG
- **Sandbox** — `--sandbox-dir`, `--max-size`, `--readonly`, `--whitelist`

## Requirements

- Go **1.23+** (builds verified on newer versions too)
- Optional: [LibreOffice](https://www.libreoffice.org/) for `lint --visual` (`soffice` on `PATH`, or `LIBREOFFICE_PATH`)

## Install

### From source

```bash
git clone git@github.com:JuliossJunk/docxcraft.git
cd docxcraft
go build -o docxcraft ./cmd/docxcraft
```

Static build without CGO:

```bash
CGO_ENABLED=0 go build -o docxcraft ./cmd/docxcraft
```

### Pre-built binaries (v0.0.1)

Download archives from the [GitHub Releases](https://github.com/JuliossJunk/docxcraft/releases) page (`linux` / `darwin` / `windows`, `amd64` / `arm64`).

### Go module

```bash
go get github.com/JuliossJunk/docxcraft@v0.0.1
```

Import path: `github.com/JuliossJunk/docxcraft/pkg/docxcraft` 

## CLI

```bash
# Build a document from AST
docxcraft build report_ast.json -p gost_7_32_2017 -o report.docx

# Skeleton with anchors (for agents)
docxcraft inspect contract.docx --json

# Audit against a profile
docxcraft lint contract.docx -p legal_contract --strict

# Visual gate (LibreOffice required)
docxcraft lint contract.docx -p legal_contract --visual --keep-visual

# Targeted edit
docxcraft patch draft.docx -a 1A4F09C1 --action replace_text --content "New text" -o out.docx
docxcraft patch draft.docx -a 1A4F09C1 --action insert_after --content "Insert" -o out.docx
docxcraft patch draft.docx -a 1A4F09C1 --action delete -o out.docx

# MCP server (stdio)
docxcraft mcp --transport stdio --sandbox-dir ./workspace
```

Global security flags: `--sandbox-dir`, `--max-size`, `--readonly`, `--whitelist`.

CLI exit codes: `0` success, `1` linter/validation errors, `2` fatal (IO, ZIP, sandbox).

## Style profiles

Embedded under `profiles/` (`embed`):

| Name | Purpose |
| --- | --- |
| `gost_7_32_2017` | GOST 7.32-2017 reports |
| `corporate_executive` | Corporate executive style |
| `legal_contract` | Contracts / legal |

## JSON AST example

```json
{
  "title": "Technical specification",
  "blocks": [
    {"type": "heading", "level": 1, "text": "1. Overview"},
    {"type": "paragraph", "text": "This document defines system requirements."},
    {
      "type": "table",
      "headers": ["Module", "Status"],
      "col_widths_cm": [8, 4],
      "rows": [["compiler", "done"], ["linter", "done"]]
    }
  ]
}
```

Block types: `heading`, `paragraph`, `table`, `callout`, `list`, `page_break`.

## MCP tools

With `docxcraft mcp`:

| Tool | Purpose |
| --- | --- |
| `inspect_skeleton` | Compact skeleton with `w14:paraId` |
| `compile_ast` | AST + profile → `.docx` |
| `patch_document` | Edit by anchor |
| `audit_document` | Linter (+ optional visual) |

## Go SDK

```go
import "github.com/JuliossJunk/docxcraft/pkg/docxcraft"

pkg, err := docxcraft.Open("contract.docx")
blocks, err := docxcraft.Inspect(pkg)
res, err := docxcraft.Build(astJSON, "corporate_executive")
report, err := docxcraft.Lint(pkg, linter.Options{ProfileName: "legal_contract"})
```

## Package layout

```
cmd/docxcraft          — CLI / MCP entrypoint
internal/archive       — lossless in-memory ZIP (.docx)
internal/oxml          — DOM helpers, ECMA-376 normalizer
internal/anchor        — w14:paraId index, InspectSkeleton
internal/profile       — YAML profiles, cm/pt → dxa
internal/compiler      — JSON AST → .docx
internal/patcher       — HealRuns, anchor mutations
internal/linter        — DF-* rules, Visual Gate
internal/security      — sandbox / limits / whitelist
internal/mcpserver     — MCP tool registration
pkg/docxcraft          — public SDK
specs/                 — technical specifications
```

Contracts live in [`AGENTS.md`](AGENTS.md) and [`specs/`](specs/).

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
go test ./...
golangci-lint run ./...
CGO_ENABLED=0 go build ./...
```

Round-trip fixture: `testdata/fixtures/attach_LAW_102740_44.docx`.

## License

[MIT](LICENSE) © 2026 JuliossJunk
