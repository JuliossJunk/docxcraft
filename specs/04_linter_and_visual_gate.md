# Specification 04: Rule-Based Linter & Visual Gate

**Target Package:** `internal/linter`  
**Status:** Draft / Core Architecture  
**Dependencies:** `internal/archive`, `internal/oxml`, `internal/anchor`, `internal/profile`; optional: LibreOffice headless

---

## 1. Назначение двухконтурного линтера

Линтер DocxForge обеспечивает **детерминированную самокоррекцию** для LLM и CI: после `compile_ast` / `patch_document` агент получает машинночитаемый отчёт с кодами правил и `anchor_id`, а не размытый текст «документ выглядит плохо».

Два контура:

1. **Контур 1 — Rule-Based AST/DOM Linter (обязательный):** чистый Go, без внешних процессов. Проверяет геометрию страницы, поля, пустые абзацы, иерархию заголовков, таблицы (`w:tblHeader`, `w:cantSplit`, ширины колонок), соответствие шрифтов/интервалов выбранному YAML-профилю.
2. **Контур 2 — Visual Gate (опциональный):** конвертация `.docx` → PDF/PNG через `LibreOffice --headless` для последующей VLM-проверки (съехавшие колонки, обрезанный текст). Если LibreOffice недоступен, контур пропускается с `severity_gate.status = "skipped"`.

Потребители: CLI `docxforge lint`, MCP-инструмент `audit_document` (см. `specs/05_mcp_tools_and_security.md`).

---

## 2. Модель отчёта (JSON Report Schema)

```json
{
  "is_valid": false,
  "profile_name": "gost_7_32_2017",
  "issue_count": 2,
  "issues": [
    {
      "code": "DF-004",
      "severity": "error",
      "anchor_id": "3B8E12A4",
      "location": "word/document.xml",
      "message": "Empty paragraph used for vertical spacing instead of paragraph spacing properties",
      "suggested_fix": "Remove empty paragraph or set w:spacing/@w:before|/w:after from the style profile; do not insert blank w:p for gaps"
    }
  ],
  "visual_gate": {
    "status": "skipped",
    "reason": "libreoffice_not_found",
    "pdf_path": "",
    "png_paths": []
  }
}
```

### 2.1. Поля

| Поле | Тип | Описание |
| :--- | :--- | :--- |
| `is_valid` | bool | `true`, если нет issues с `severity` ∈ {`error`, `fatal`} |
| `profile_name` | string | Имя профиля, с которым сверяли документ |
| `issue_count` | int | `len(issues)` |
| `issues[]` | array | Список нарушений |
| `issues[].code` | string | Код `DF-XXX` |
| `issues[].severity` | string | `info` \| `warning` \| `error` \| `fatal` |
| `issues[].anchor_id` | string | `w14:paraId` / `tbl_…` якорь; пустая строка, если нарушение на уровне секции/документа |
| `issues[].location` | string | Часть пакета (`word/document.xml`, …) |
| `issues[].message` | string | Человекочитаемое описание для LLM |
| `issues[].suggested_fix` | string | Конкретное действие по исправлению |
| `visual_gate` | object | Результат контура 2 (всегда присутствует) |

### 2.2. Severity и exit codes CLI

| Severity | Влияет на `is_valid` | CLI exit (при `--strict`) |
| :--- | :---: | :---: |
| `info` | нет | 0 |
| `warning` | нет (да при `--strict`) | 0 / 1 |
| `error` | да → `false` | 1 |
| `fatal` | да → `false` | 2 |

Без `--strict`: warnings не роняют exit code. С `--strict`: любой `warning`+ даёт exit `1`.

---

## 3. Каталог правил DF-001 … DF-020

Все геометрические сравнения ведутся во **внутренних единицах `dxa` (twips)** после загрузки профиля через `internal/profile/units.go`. Допуск по умолчанию: **±20 dxa**, если правило не задаёт иное.

| Код | Severity | Правило | Как детектировать | Suggested fix (смысл) |
| :--- | :--- | :--- | :--- | :--- |
| **DF-001** | error | Неверный размер страницы относительно профиля | `w:pgSz` `@w:w`/`@w:h` ≠ профиль (A4 = 11906×16838 dxa и т.д.) | Выставить `sectPr/pgSz` по профилю |
| **DF-002** | error | Поля страницы не соответствуют профилю | `w:pgMar` top/bottom/left/right вне допуска | Синхронизировать `pgMar` с YAML `page.margins` |
| **DF-003** | warning | Отсутствует `w:sectPr` в `w:body` | Нет завершающего `sectPr` | Добавить секцию с геометрией профиля |
| **DF-004** | error | Пустой абзац как вертикальный отступ | `<w:p>` без видимого текста (и без осмысленного `w:drawing`) между контентными блоками | Удалить пустой `w:p`; использовать `w:spacing` |
| **DF-005** | error | Нарушена иерархия заголовков | После `HeadingN` встречается `HeadingM` с `M > N+1` (прыжок уровней) | Вставить промежуточный уровень или понизить `M` |
| **DF-006** | warning | Заголовок без следующего контента | `Heading*` сразу перед другим `Heading*` того же/высшего уровня или концом body | Добавить абзац или объединить заголовки |
| **DF-007** | error | Сумма ширин колонок таблицы > ширины контентной области | Σ `tcW` / `tblGrid` > (`pgSz.w` − left − right) | Пересчитать `col_widths` / `tcW` в dxa |
| **DF-008** | error | У таблицы отсутствуют явные ширины колонок | Нет `w:tblGrid/w:gridCol` или все `tcW` type=`auto` при профиле, требующем fixed | Задать `gridCol` и `tcW` в dxa |
| **DF-009** | error | Нет `w:tblHeader` у строки шапки | Первая строка таблицы с заголовочным контентом без `trPr/tblHeader` | Добавить `<w:tblHeader/>` |
| **DF-010** | warning | Нет `w:cantSplit` у строк таблицы | Строка без `trPr/cantSplit` | Добавить `<w:cantSplit/>` на строки данных/шапки |
| **DF-011** | error | Шрифт body не из профиля | Доминирующий `rFonts` абзацев Normal ≠ `typography.body_font` | Выставить `rFonts` / стиль Normal по профилю |
| **DF-012** | error | Кегль body вне допуска профиля | `sz` (half-points) ≠ `body_size_pt * 2` ± допуск | Исправить `w:sz` / `w:szCs` |
| **DF-013** | warning | Межстрочный интервал не по профилю | `spacing/@w:line` / `@w:lineRule` расходятся с `line_spacing` | Выставить spacing из профиля |
| **DF-014** | error | Заголовок H1/H2/H3: кегль или bold не по профилю | Стили Heading1..3 vs блок `headings` в YAML | Применить свойства из профиля |
| **DF-015** | warning | Абзац без `w14:paraId` | После индексации всё ещё есть `<w:p>` без валидного paraId | Вызвать `EnsureAnchors` и сохранить |
| **DF-016** | error | Дубликат `w14:paraId` | Один и тот же ID на двух узлах | Перегенерировать конфликтующие ID |
| **DF-017** | warning | Разорванный шаблонный плейсхолдер | В абзаце есть фрагменты `{{` / `}}`, но нет цельного `{{…}}` в одном `<w:t>` | Запустить `HealRuns` |
| **DF-018** | error | Ячейка таблицы без `tcW` при fixed-layout | `tblW`/`tblLayout` fixed, у `tc` нет `tcW` | Проставить ширину ячейки |
| **DF-019** | info | Документ содержит tracked changes | Присутствуют `w:ins` / `w:del` | Ожидаемо при рецензировании; иначе принять/отклонить правки |
| **DF-020** | fatal | Повреждённый / нечитаемый пакет | ZIP/XML не открывается, Zip Bomb, отсутствует `word/document.xml` | Передать валидный `.docx`; проверить `--max-size` |

Правила **DF-001…DF-014** требуют загруженного `StyleProfile`. **DF-015…DF-020** работают и без профиля (профиль тогда опционален; при отсутствии — только structural rules).

---

## 4. API пакета `internal/linter`

```go
package linter

type Options struct {
	ProfileName   string
	Strict        bool
	EnableVisual  bool
	VisualTimeout time.Duration // default 60s
	WorkDir       string        // temp dir for LO output; empty → os.TempDir
}

type Report struct {
	IsValid     bool         `json:"is_valid"`
	ProfileName string       `json:"profile_name"`
	IssueCount  int          `json:"issue_count"`
	Issues      []Issue      `json:"issues"`
	VisualGate  VisualResult `json:"visual_gate"`
}

type Issue struct {
	Code          string `json:"code"`
	Severity      string `json:"severity"`
	AnchorID      string `json:"anchor_id"`
	Location      string `json:"location"`
	Message       string `json:"message"`
	SuggestedFix  string `json:"suggested_fix"`
}

func Audit(pkg *archive.Package, opts Options) (*Report, error)
```

`Audit`:

1. Загружает профиль (если `ProfileName` задан).
2. Гарантирует индекс якорей (read-only проход; мутации ID — только по явному флагу в будущем; в v1 линтер **не пишет** в документ).
3. Прогоняет DF-001…DF-020.
4. Если `EnableVisual` — вызывает Visual Gate и заполняет `visual_gate`.
5. Вычисляет `is_valid`.

---

## 5. Visual Gate (LibreOffice Headless)

### 5.1. Протокол вызова

```text
soffice|libreoffice --headless --norestore --nolockcheck \
  --convert-to pdf --outdir <WorkDir> <input.docx>
```

Опционально PDF → PNG (первая страница или все):

```text
soffice --headless --convert-to png --outdir <WorkDir> <input.pdf>
```

Исполняемый файл ищется в порядке: `LIBREOFFICE_PATH` env → `soffice` / `libreoffice` в `PATH` → типичные пути Windows/macOS/Linux.

### 5.2. Статусы `visual_gate.status`

| Status | Когда |
| :--- | :--- |
| `ok` | Конвертация успешна, пути к артефактам заполнены |
| `skipped` | `EnableVisual=false` или LO не найден (`reason`: `disabled` / `libreoffice_not_found`) |
| `timeout` | Превышен `VisualTimeout` |
| `error` | Ненулевой exit LO или пустой выходной файл (`reason` = stderr summary) |

Visual Gate **сам по себе не добавляет DF-коды**. Он только готовит артефакты; VLM-анализ — ответственность внешнего агента. При `status=error|timeout` в `issues` можно добавить `info`-запись без кода DF (или расширить каталог позже); в v1 — только поле `visual_gate`.

### 5.3. Безопасность

- Входной путь к `.docx` обязан пройти `internal/security` sandbox (если вызван из CLI/MCP).
- `WorkDir` создаётся с уникальным префиксом и удаляется вызывающим кодом после чтения артефактов (кроме явного `--keep-visual`).
- Запрещён сетевой доступ со стороны процесса LO (флаги `--norestore`; не передавать URL).

---

## 6. Интеграция с CLI и MCP

| Интерфейс | Поведение |
| :--- | :--- |
| `docxforge lint FILE -p PROFILE [--strict] [--visual]` | Печатает JSON-отчёт в stdout; exit 0/1/2 по §2.2 |
| MCP `audit_document` | Возвращает тот же JSON как structured content |

Компилятор (`compile_ast` / `docxforge build`) **может** опционально вызывать `Audit` post-build и включать отчёт в ответ (см. spec 05).

---

## 7. Требования к тестам (`internal/linter/*_test.go`)

1. **`TestDF004EmptyParagraph`**: фикстура с пустым `<w:p>` между двумя абзацами → issue `DF-004`, корректный `anchor_id`.
2. **`TestDF005HeadingSkip`**: H1 сразу за ним H3 → `DF-005`.
3. **`TestDF009TblHeaderMissing`**: таблица без `tblHeader` → `DF-009`.
4. **`TestReportIsValidAggregation`**: смесь warning/error → `is_valid=false`; только warning без strict → `is_valid=true`.
5. **`TestVisualGateSkippedWithoutLO`**: `EnableVisual=true`, бинарь отсутствует → `visual_gate.status=skipped`, линтер не падает.
6. **Golden:** JSON-отчёт для фиксированной фикстуры в `testdata/golden/linter_report.json` (сравнение без путей Visual Gate).
