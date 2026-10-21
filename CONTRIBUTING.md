# Contributing to DocxCraft

Thanks for contributing. This project prioritizes **lossless OpenXML edits**, **stable `w14:paraId` anchors**, and **agent-friendly errors**. Please read [`AGENTS.md`](AGENTS.md) and the specs under [`specs/`](specs/) before changing behaviour.

Translations: [README (English)](README.md) · [README (Русский)](README.ru.md)

## Development setup

1. Install **Go 1.23+**.
2. Clone the repo and fetch modules:

   ```bash
   git clone git@github.com:JuliossJunk/docxcraft.git
   cd docxcraft
   go mod download
   ```

3. Optional: install [golangci-lint](https://golangci-lint.run/) (v2+).

4. Optional: install [LibreOffice](https://www.libreoffice.org/) for visual-gate tests (`lint --visual`).

## Build

```bash
# Local binary
go build -o docxcraft ./cmd/docxcraft

# Static binary (preferred for releases)
CGO_ENABLED=0 go build -o docxcraft ./cmd/docxcraft

# Cross-platform release archives into dist/
make release VERSION=v0.0.1
```

On Windows PowerShell without `make`:

```powershell
$env:CGO_ENABLED = "0"
go build -ldflags "-X main.version=v0.0.1" -o docxcraft.exe ./cmd/docxcraft

# Cross-platform archives
.\scripts\release.ps1 -Version v0.0.1
```

## Test

```bash
go test ./...
```

Use `-race` only when CGO is available (typical on Linux/macOS CI):

```bash
go test -race ./...
```

Focused packages while iterating:

```bash
go test ./internal/oxml/ ./internal/compiler/ -count=1
```

Round-trip fixture: `testdata/fixtures/attach_LAW_102740_44.docx`.

## Lint

```bash
golangci-lint run ./...
```

Fix reported issues before opening a PR. Prefer clear `fmt.Errorf("…: %w", err)` wrapping so LLM agents can recover.

## Coding guidelines (short)

Full rules: [`AGENTS.md`](AGENTS.md) §5.

- No package-level mutable globals for documents; keep state in session/package types.
- Geometry inside `internal/oxml` is **dxa / half-points** only; convert units at profile/AST boundaries (`internal/profile`).
- Do **not** build XML with string concatenation — use `etree` / `internal/oxml` helpers.
- After mutating `pPr` / `rPr` / `tblPr` / `tcPr`, rely on save-time `oxml.NormalizeElementOrder` (via `archive.Package.Save`).
- Address existing content by **`anchor_id` (`w14:paraId`)**, never by paragraph index.
- Keep `CGO_ENABLED=0` builds working; do not add C dependencies.
- Prefer golden XML / round-trip tests for compiler and patcher changes.

## Pull requests

1. Open an issue (or pick one labelled `ready-for-agent` / `ready-for-human`) describing the change.
2. Branch from `main`: `feat/…`, `fix/…`, or `docs/…`.
3. Keep commits small and use [Conventional Commits](https://www.conventionalcommits.org/):

   - `feat(anchor): …`
   - `fix(compiler): …`
   - `docs(readme): …`
   - `refactor(oxml): …`
   - `test(linter): …`

4. Ensure locally:

   ```bash
   go test ./...
   golangci-lint run ./...
   CGO_ENABLED=0 go build ./cmd/docxcraft
   ```

5. PR description should say **what** changed and **why**, with before/after notes for CLI/MCP behaviour when relevant.

## Specs and domain docs

- Behaviour contracts: `specs/01_…` … `specs/05_…`
- Agent tracker / triage / glossary layout: `docs/agents/`
- Do not invent OpenXML tag names or AST shapes that contradict the specs

## Release packages

Maintainers cut a release by tagging `vX.Y.Z` and attaching archives from `make release`:

```bash
make release VERSION=v0.0.1
# Windows: .\scripts\release.ps1 -Version v0.0.1
gh release create v0.0.1 ./dist/*.zip ./dist/*.tar.gz --generate-notes
```

Pushing a `v*` tag also triggers `.github/workflows/release.yml`, which builds the same archives on GitHub Actions.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
