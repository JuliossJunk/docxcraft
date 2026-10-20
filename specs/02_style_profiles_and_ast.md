# Specification 02: Declarative JSON AST, YAML Style Profiles & Compiler

**Target Packages:** `internal/profile`, `internal/compiler`, `profiles/*.yaml`  
**Status:** Draft / Core Architecture  
**Dependencies:** `internal/oxml`, `gopkg.in/yaml.v3`

---

## 1. Назначение и концепция

Декларативный компилятор решает главную проблему слабых или средних LLM: модели часто ошибаются в синтаксисе кода, но отлично генерируют структурированные данные (JSON).

Вместо написания скриптов агент передает в харнесс:
1. **Имя профиля стиля** (например, `"gost_7_32_2017"`, `"corporate_executive"` или `"legal_contract"`).
2. **Дерево контента (AST)** в виде типизированного JSON.

Компилятор на Go детерминированно собирает из этого идеальный `.docx`, применяя отступы, шрифты, таблицы и выравнивания в соответствии со стандартом.

---

## 2. YAML-профили стилей (`internal/profile` + `profiles/`)

Профили хранятся в папке `profiles/` в виде YAML-файлов (embed.FS) и описывают всю визуальную и геометрическую дизайн-систему документа. Конвертация пользовательских единиц (`cm`, `mm`, `in`, `pt`) в `dxa` / half-points выполняется в `internal/profile/units.go`.

### 2.1. Структура профиля (`StyleProfile`)
```yaml
name: "corporate_executive"
description: "Modern executive style with deep navy accents and clean typography"

page:
  size: "A4" # A4 | Letter
  margins:
    top_cm: 2.54
    bottom_cm: 2.54
    left_cm: 2.54
    right_cm: 2.54

palette:
  primary: "#1E3A8A"    # Deep Navy (Headings, Accent borders)
  secondary: "#475569"  # Slate Gray (Subheadings, Table headers)
  body_text: "#1F2937"  # Charcoal (Main text)
  muted_bg: "#F1F5F9"   # Light tint for callouts and table headers

typography:
  body_font: "Calibri"
  heading_font: "Aptos Display"
  body_size_pt: 11.0
  line_spacing: 1.15
  paragraph_space_after_pt: 6.0

headings:
  h1:
    size_pt: 16.0
    bold: true
    space_before_pt: 12.0
    space_after_pt: 4.0
    keep_with_next: true
  h2:
    size_pt: 13.0
    bold: true
    space_before_pt: 8.0
    space_after_pt: 3.0
    keep_with_next: true
```
## 2.2. Загрузчик профилей (`internal/profile/loader.go`)

Менеджер профилей кэширует YAML-файлы при старте приложения. Если запрашиваемый профиль не найден, компилятор возвращает ошибку (без скрытого fallback). Встроенные профили: `gost_7_32_2017`, `corporate_executive`, `legal_contract`.

---

# 3. Декларативная JSON AST схема (`internal/compiler/ast`)

Документ состоит из метаданных и плоского или иерархического списка блоков (`Block`).

## 3.1. Структура блоков AST

Поддерживаемые типы блоков в JSON:

| Тип блока (`type`) | Описание | Основные поля |
| --- | --- | --- |
| `heading` | Заголовок уровня 1–3 | `level` (1, 2, 3), `text`, `anchor_id` (опционально) |
| `paragraph` | Обычный текстовый абзац | `text`, `runs` (массив стилизованных кусочков текста), `alignment` (`left`, `center`, `right`, `both`) |
| `table` | Таблица с данными | `headers` ([]string), `rows` ([][]string), `col_widths_cm` ([]float64) |
| `callout` | Выноска (плашка с цитатой или важным примечанием) | `title`, `text`, `variant` (`info`, `warning`, `quote`) |
| `list` | Нумерованный или маркированный список | `list_type` (`bullet`, `numbered`), `items` ([]string) |
| `page_break` | Принудительный разрыв страницы | — |

## 3.2. Пример полной AST-структуры в JSON

```json
{
  "title": "Техническое задание на разработку",
  "metadata": {
    "author": "DocxCraft Engine",
    "version": "1.0"
  },
  "blocks": [
    {
      "type": "heading",
      "level": 1,
      "text": "1. Общие положения"
    },
    {
      "type": "paragraph",
      "text": "Настоящий документ определяет требования к системе автоматической генерации документов."
    },
    {
      "type": "callout",
      "variant": "info",
      "title": "Важное примечание",
      "text": "Все таблицы должны соответствовать стандартам адаптивной верстки."
    },
    {
      "type": "table",
      "col_widths_cm": [3.0, 9.0, 4.5],
      "headers": ["Модуль", "Описание", "Статус"],
      "rows": [
        ["oxml", "Работа с ZIP и DOM", "Готово"],
        ["compiler", "Генерация из AST", "В разработке"]
      ]
    }
  ]
}

```

---

# 4. Архитектура компилятора (`internal/compiler`)

Компилятор создает новый чистый документ с дефолтным шаблоном Word, применяет настройки страницы из профиля, а затем пошагово транслирует каждый узел AST в OXML-элементы.

## 4.1. Рендеринг параграфов и инлайн-стилей (`renderParagraph`)

* Абзацы получают отступы (`spacing`, `ind`) и выравнивание (`jc`) из `StyleProfile`.
* Поддерживается массив `runs` для выделения жирным, курсивом или цветом отдельных слов внутри абзаца:
```json
"runs": [
  {"text": "Система поддерживает ", "bold": false},
  {"text": "строгую типизацию", "bold": true, "color": "1E3A8A"}
]

```



## 4.2. Рендеринг таблиц (`renderTable`)

При трансляции таблицы компилятор автоматически вызывает методы из спецификаций `oxml`:

1. Устанавливает явные ширины колонок (`tcW`) в twips для каждой ячейки каждой строки.
2. Применяет цвет паллеты (`secondary` или `muted_bg`) для шапки таблицы.
3. Добавляет теги `<w:cantSplit/>` для строк и `<w:tblHeader/>` для шапки таблицы (предотвращает разрыв строк на стыке страниц).
4. Проставляет внутренние отступы ячеек (`tcMar`) и тонкие светлые границы (`tcBorders`).

## 4.3. Рендеринг выносок (`renderCallout`)

Выноска компилируется в специальную таблицу размером 1x1:

* Левая граница устанавливается толстой (ширина `3pt`, цвет равен `primary` из профиля).
* Верхняя, правая и нижняя границы удаляются.
* Фоновый цвет ячейки заливается цветом `muted_bg`.
* Внутри ячейки создается абзац с курсивным или уменьшенным текстом.

---

# 5. Интеграция и поток выполнения (Pipeline)

```
[JSON AST / LLM Input] 
         ↓
[Go AST Validator (encoding/json + typed structs)]
         ↓
[StyleProfile Loader (YAML)]
         ↓
[DocxCompiler Engine]
         ↓ (Генерация DOM через oxml)
[Schema Normalizer (Spec 03)]
         ↓
[Готовый .docx файл]

```

---

# 6. Требования к тестам (`internal/compiler/*_test.go`)

1. **`TestCompileSimpleAST`**: Подать минимальный JSON с заголовком и абзацем, скомпилировать с профилем `corporate_executive`, проверить отсутствие ошибок и наличие созданных элементов в XML.
2. **`TestTableFormattingAndSplitting`**: Скомпилировать документ с таблицей из 20 строк, проверить наличие тегов `cantSplit` и корректность расчета ширин колонок.