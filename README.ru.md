# DocxCraft

[English](README.md)

Агентный харнесс (MCP-сервер + CLI + Go SDK) для **детерминированной** генерации, точечного редактирования и линтинга документов Microsoft Word (`.docx`).

Главная идея: править OpenXML **без потерь верстки** (in-memory ZIP + DOM), адресовать абзацы по стабильным якорям `w14:paraId`, а оформление задавать YAML-профилями (ГОСТ / Corporate / Legal).

## Возможности

- **Сборка из JSON AST** — LLM отдаёт структуру, Go собирает валидный `.docx` по профилю стилей
- **Патчи по якорям** — `replace_text`, `insert_after`, `delete`, tracked changes, клонирование формата
- **Склейка шаблонов** — `HealRuns` восстанавливает разорванные Word’ом плейсхолдеры `{{ … }}`
- **ECMA-376 normalizer** — порядок тегов в `pPr`/`rPr`/`tcPr`, чтобы Word не писал «файл повреждён»
- **Линтер DF-001…DF-020** — геометрия, заголовки, таблицы, шрифты; опциональный Visual Gate через LibreOffice → PDF/PNG
- **Песочница** — `--sandbox-dir`, `--max-size`, `--readonly`, `--whitelist`

## Требования

- Go **1.23+** (сборка проверена и на более новых версиях)
- Опционально: [LibreOffice](https://www.libreoffice.org/) для `lint --visual` (`soffice` в `PATH` или `LIBREOFFICE_PATH`)

## Установка

### Из исходников

```bash
git clone git@github.com:JuliossJunk/docxcraft.git
cd docxcraft
go build -o docxcraft ./cmd/docxcraft
```

Статическая сборка без CGO:

```bash
CGO_ENABLED=0 go build -o docxcraft ./cmd/docxcraft
```

### Готовые бинарники (v0.0.1)

Скачайте архивы на странице [GitHub Releases](https://github.com/JuliossJunk/docxcraft/releases) (`linux` / `darwin` / `windows`, `amd64` / `arm64`).

### Go-модуль

```bash
go get github.com/JuliossJunk/docxcraft@v0.0.1
```

Импорт: `github.com/JuliossJunk/docxcraft/pkg/docxcraft` (module path следует структуре репозитория, пока не опубликован vanity-путь).

## CLI

```bash
# Собрать документ из AST
docxcraft build report_ast.json -p gost_7_32_2017 -o report.docx

# Скелет с якорями (для агента)
docxcraft inspect contract.docx --json

# Аудит по профилю
docxcraft lint contract.docx -p legal_contract --strict

# С визуальным контуром (нужен LibreOffice)
docxcraft lint contract.docx -p legal_contract --visual --keep-visual

# Точечная правка
docxcraft patch draft.docx -a 1A4F09C1 --action replace_text --content "Новый текст" -o out.docx
docxcraft patch draft.docx -a 1A4F09C1 --action insert_after --content "Вставка" -o out.docx
docxcraft patch draft.docx -a 1A4F09C1 --action delete -o out.docx

# MCP-сервер (stdio)
docxcraft mcp --transport stdio --sandbox-dir ./workspace
```

Глобальные флаги безопасности: `--sandbox-dir`, `--max-size`, `--readonly`, `--whitelist`.

Коды выхода CLI: `0` — успех, `1` — ошибки линтера/валидации, `2` — фатальная ошибка (IO, ZIP, sandbox).

## Профили стилей

Встроены в `profiles/` (embed):

| Имя | Назначение |
| --- | --- |
| `gost_7_32_2017` | Отчёты по ГОСТ 7.32-2017 |
| `corporate_executive` | Корпоративный executive-стиль |
| `legal_contract` | Договоры / legal |

## Пример JSON AST

```json
{
  "title": "Техническое задание",
  "blocks": [
    {"type": "heading", "level": 1, "text": "1. Общие положения"},
    {"type": "paragraph", "text": "Настоящий документ определяет требования к системе."},
    {
      "type": "table",
      "headers": ["Модуль", "Статус"],
      "col_widths_cm": [8, 4],
      "rows": [["compiler", "готово"], ["linter", "готово"]]
    }
  ]
}
```

Типы блоков: `heading`, `paragraph`, `table`, `callout`, `list`, `page_break`.

## MCP-инструменты

При `docxcraft mcp` доступны:

| Инструмент | Назначение |
| --- | --- |
| `inspect_skeleton` | Компактный скелет с `w14:paraId` |
| `compile_ast` | AST + профиль → `.docx` |
| `patch_document` | Правка по якорю |
| `audit_document` | Линтер (+ опционально visual) |

## Go SDK

```go
import "github.com/JuliossJunk/docxcraft/pkg/docxcraft"

pkg, err := docxcraft.Open("contract.docx")
blocks, err := docxcraft.Inspect(pkg)
res, err := docxcraft.Build(astJSON, "corporate_executive")
report, err := docxcraft.Lint(pkg, linter.Options{ProfileName: "legal_contract"})
```

## Архитектура пакетов

```
cmd/docxcraft          — CLI / точка входа MCP
internal/archive       — lossless in-memory ZIP (.docx)
internal/oxml          — DOM-хелперы, ECMA-376 normalizer
internal/anchor        — индекс w14:paraId, InspectSkeleton
internal/profile       — YAML-профили, cm/pt → dxa
internal/compiler      — JSON AST → .docx
internal/patcher       — HealRuns, мутации по якорям
internal/linter        — DF-* правила, Visual Gate
internal/security      — sandbox / limits / whitelist
internal/mcpserver     — регистрация MCP-инструментов
pkg/docxcraft          — публичный SDK
specs/                 — технические спецификации
```

Подробные контракты — в [`AGENTS.md`](AGENTS.md) и [`specs/`](specs/).

## Разработка

См. [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
go test ./...
golangci-lint run ./...
CGO_ENABLED=0 go build ./...
```

Fixture для round-trip: `testdata/fixtures/attach_LAW_102740_44.docx`.

## Лицензия

[MIT](LICENSE) © 2026 JuliossJunk
