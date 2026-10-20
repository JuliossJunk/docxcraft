# Specification 05: MCP Tools, CLI & Enterprise Security

**Target Packages:** `internal/mcpserver`, `internal/security`, `cmd/docxcraft`  
**Status:** Draft / Core Architecture  
**Dependencies:** `github.com/mark3labs/mcp-go`, `github.com/spf13/cobra`, `github.com/invopop/jsonschema`; ядро: `archive`, `oxml`, `anchor`, `profile`, `compiler`, `patcher`, `linter`

---

## 1. Назначение

DocxCraft предоставляет два равноправных транспорта поверх одного вычислительного ядра:

1. **MCP Server (stdio)** — инструменты для ИИ-агентов (Cursor, Claude Code и др.) через JSON-RPC / MCP.
2. **CLI (`docxcraft`)** — те же операции для людей, скриптов и CI/CD.

Оба транспорта обязаны проходить через `internal/security` (песочница путей, лимиты размера, readonly, whitelist инструментов) и работать с документами через **`DocumentSession`** (без глобального состояния).

---

## 2. DocumentSession (in-memory)

```go
package docxcraft // или internal session type used by mcpserver/cli

type DocumentSession struct {
	ID        string
	Package   *archive.Package
	ReadOnly  bool
	OpenedAt  time.Time
	SourcePath string // original path inside sandbox; may be empty for compile-from-AST
}
```

Правила:

- Одна операция CLI/MCP создаёт или открывает сессию, выполняет работу, при необходимости `Save`, затем закрывает сессию (v1 — без долгоживущего session store между вызовами).
- Параллельные горутины обрабатывают **разные** сессии; внутри сессии — mutex пакета (`Package.mu`).
- Запрещены пакетные глобальные `var currentDoc`.

---

## 3. Security (`internal/security`)

### 3.1. Глобальные флаги (CLI root / MCP server startup)

| Флаг | Тип | По умолчанию | Поведение |
| :--- | :--- | :--- | :--- |
| `--sandbox-dir` | string | пусто = cwd | Все `file_path` / `output_path` резолвятся и проверяются как пути **внутри** этого каталога (после `EvalSymlinks` + `filepath.Abs`). Выход за пределы → ошибка. |
| `--max-size` | int64 | `104857600` (100 MiB) | Максимальный размер входного `.docx` на диске и лимит распаковки (согласован с Zip Bomb guard в `archive`). |
| `--readonly` | bool | `false` | Запрещает `compile_ast`, `patch_document` и запись на диск; разрешены `inspect_skeleton`, `audit_document`. |
| `--whitelist` | string | пусто = все | CSV имён MCP-инструментов / CLI-команд (`inspect,lint,build`). Неперечисленные вызовы отклоняются. |

### 3.2. API

```go
package security

type Config struct {
	SandboxDir string
	MaxSize    int64
	ReadOnly   bool
	Whitelist  map[string]struct{} // empty map = allow all
}

func (c *Config) ResolvePath(userPath string) (abs string, err error)
func (c *Config) AllowTool(name string) bool
func (c *Config) AssertWritable() error // error if ReadOnly
func (c *Config) AssertFileSize(path string) error
```

Ошибки оборачиваются с контекстом для LLM, например:  
`security: path escapes sandbox-dir: ../secrets/a.docx`.

---

## 4. MCP Server (`internal/mcpserver`)

Запуск: `docxcraft mcp --transport stdio` (+ security flags).

Транспорт v1: **только stdio**. Аргументы инструментов — структуры с тегами `jsonschema` (`invopop/jsonschema` / mcp-go).

Встроенные имена профилей: `gost_7_32_2017`, `corporate_executive`, `legal_contract`.

### 4.1. `inspect_skeleton`

**Input:**

```json
{
  "file_path": "contracts/draft.docx"
}
```

| Поле | Required | Описание |
| :--- | :---: | :--- |
| `file_path` | yes | `.docx` внутри sandbox |

**Output:**

```json
{
  "ok": true,
  "file_path": "contracts/draft.docx",
  "blocks": [
    {
      "anchor": "1A4F09C1",
      "type": "p",
      "style": "Heading1",
      "text": "1. Предмет договора"
    },
    {
      "anchor": "tbl_7C01D2E9",
      "type": "tbl",
      "rows": 4,
      "cols": 3,
      "header_preview": ["№", "Наименование", "Сумма"]
    }
  ]
}
```

См. также `InspectSkeleton` в `specs/01_oxml_dom_and_anchors.md`.

### 4.2. `compile_ast`

**Input:**

```json
{
  "profile_name": "gost_7_32_2017",
  "ast_json": "{ ... }",
  "output_path": "out/report.docx",
  "run_linter": true
}
```

| Поле | Required | Описание |
| :--- | :---: | :--- |
| `profile_name` | yes | Имя YAML-профиля |
| `ast_json` | yes | Строка с JSON AST (spec 02) |
| `output_path` | yes | Куда сохранить `.docx` |
| `run_linter` | no | Default `true` — пост-аудит |

**Output:**

```json
{
  "ok": true,
  "output_path": "out/report.docx",
  "lint_report": { "is_valid": true, "profile_name": "gost_7_32_2017", "issue_count": 0, "issues": [], "visual_gate": { "status": "skipped", "reason": "disabled" } }
}
```

При `--readonly` → ошибка до записи. При `ok=false` — поле `error` с сообщением валидации AST/профиля.

### 4.3. `patch_document`

**Input:**

```json
{
  "file_path": "contracts/draft.docx",
  "anchor_id": "1A4F09C1",
  "action": "insert_after",
  "content": "Новый абзац после предмета договора.",
  "output_path": "contracts/draft_patched.docx",
  "copy_format_from": "1A4F09C1"
}
```

| Поле | Required | Описание |
| :--- | :---: | :--- |
| `file_path` | yes | Исходный `.docx` |
| `anchor_id` | yes | `w14:paraId` или `tbl_…` |
| `action` | yes | `replace_text` \| `insert_after` \| `delete` \| `apply_tracked_change` |
| `content` | no | Текст для replace/insert/tracked |
| `output_path` | yes | Путь результата |
| `copy_format_from` | no | Якорь-донор стиля (`CloneStyleFrom`) |
| `author` | no | Для `apply_tracked_change` (default `DocxCraft`) |

**Output:**

```json
{
  "ok": true,
  "output_path": "contracts/draft_patched.docx",
  "new_anchors": ["2F91A0B3"],
  "affected_anchor": "1A4F09C1"
}
```

### 4.4. `audit_document`

**Input:**

```json
{
  "file_path": "contracts/draft.docx",
  "profile_name": "corporate_executive",
  "enable_visual": false,
  "strict": false
}
```

**Output:** полный JSON-отчёт из `specs/04_linter_and_visual_gate.md` (§2), плюс обёртка:

```json
{
  "ok": true,
  "report": { "is_valid": false, "profile_name": "corporate_executive", "issue_count": 1, "issues": [ ... ], "visual_gate": { ... } }
}
```

---

## 5. CLI (`cmd/docxcraft`)

Библиотека: **Cobra**. Корневые persistent flags = security flags из §3.1.

### 5.1. Дерево команд

```bash
docxcraft [command] [flags]
```

| Команда | Флаги | Описание |
| :--- | :--- | :--- |
| **`mcp`** | `--transport stdio` | Запуск MCP-сервера (не `serve`) |
| **`build`** | `-p/--profile`, `-o/--output`, `--lint` | Компиляция JSON AST → `.docx` |
| **`inspect`** | `--json` | Скелет якорей в stdout |
| **`lint`** | `-p/--profile`, `--strict`, `--visual` | Аудит по spec 04 |
| **`patch`** | `-a/--anchor`, `--action`, `--content`, `-o/--output`, `--copy-format-from` | Точечная правка по якорю |

Whitelist сопоставляет имена команд: `mcp`, `build`, `inspect`, `lint`, `patch`.

### 5.2. Примеры

```bash
docxcraft build report_ast.json --profile gost_7_32_2017 -o final_report.docx

docxcraft lint contract.docx --profile corporate_executive --strict

docxcraft inspect proposal.docx --json

docxcraft mcp --transport stdio --sandbox-dir ./workspace --readonly=false

docxcraft patch draft.docx -a 1A4F09C1 --action replace_text --content "Обновлённый текст" -o out.docx
```

### 5.3. Exit codes

| Code | Meaning |
| :---: | :--- |
| **0** | Успех; линтер без error/fatal (и без warning при `--strict`) |
| **1** | Validation / lint error |
| **2** | Fatal: IO, ZIP, sandbox escape, AST parse, OXML |

---

## 6. Связь пакетов

```text
cmd/docxcraft
    → internal/security   (флаги, ResolvePath, AllowTool)
    → internal/mcpserver  (только команда mcp)
    → internal/compiler | patcher | linter | archive
```

Публичный SDK: `pkg/docxcraft` реэкспортирует высокоуровневые функции (`Open`, `Build`, `Lint`, `Patch`) без MCP-зависимости — для встраивания в другие Go-проекты.

---

## 7. Требования к тестам

1. **`TestSecuritySandboxEscape`**: путь `../outside.docx` при `--sandbox-dir` → ошибка.
2. **`TestSecurityReadonlyBlocksWrite`**: `compile_ast` / `build` при `--readonly` → отказ.
3. **`TestWhitelistFiltersTools`**: whitelist=`inspect,lint` → `patch` запрещён.
4. **`TestMCPToolSchemas`**: у всех четырёх инструментов генерируется валидная JSON Schema.
5. **`TestCLIExitCodes`**: lint с DF-error → exit 1; битый ZIP → exit 2; чистый файл → 0.
6. **`TestMCPToolExecution`**: вызов через stdio pipes (мок ядра допустим) → корректный JSON ответ.
