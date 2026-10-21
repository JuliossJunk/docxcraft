# AGENTS.md — DocxCraft Architecture & Engineering Guidelines

## 1. Назначение проекта (Project Vision)

**DocxCraft** — это высокопроизводительный агентный харнесс (MCP-сервер + CLI + Go SDK) для детерминированной генерации, точечного редактирования и строгого линтинга документов Microsoft Word (`.docx`).

### Главная архитектурная цель
Превзойти существующие решения (Anthropic `docx skill`, `docx-rs`, Python-обвязки) за счет **Zero-Loss DOM Manipulation** (редактирование XML-дерева в памяти без потери оригинальной верстки) и отделения декларативного контента от строгих профилей стандартов (ГОСТ, Corporate, Legal).

---

## 2. Ключевые архитектурные столпы (Core Pillars)

1. **Lossless In-Memory ZIP/DOM Engine (`archive/zip` + `beevik/etree`):**
   * Никакой десериализации всего документа в жесткие структуры, уничтожающие неизвестные теги OpenXML.
   * Файл `.docx` распаковывается в оперативной памяти, модифицируются только целевые узлы в `word/document.xml`, `word/styles.xml`, `word/numbering.xml`, а остальные части архива (колонтитулы, графики, шрифты, макросы, кастомные XML) сохраняются байт-в-байт.
2. **Адресация через стабильные якоря (`w14:paraId` Stable Anchors):**
   * Запрещено редактировать существующие документы по порядковым индексам абзацев (`paragraph_index`), так как вставка или удаление текста сдвигает нумерацию.
   * Индексатор назначает и читает неизменяемые атрибуты `w14:paraId`. Все мутации, комментарии и клонирование стилей адресуются строго по `anchor_id`.
3. **Триада режимов работы для LLM:**
   * **Declarative AST Compiler:** Генерация сложных документов с нуля из типизированного JSON AST по правилам выбранного YAML-профиля стилей.
   * **Anchor Patcher & Format Cloner:** Точечная модификация существующих документов (`patch_by_anchor`, `copy_format_from`, `apply_tracked_change` через `<w:ins>`/`<w:del>`).
   * **Template Engine:** Инспекция и безопасное заполнение плейсхолдеров `{{ var }}` внутри текстовых прогонов (`<w:r>`) со склейкой разорванных Word'ом тегов.
4. **Автоматический санитайзер схемы (`ECMA-376 Element Order Normalizer`):**
   * Перед записью любого XML-узла пост-процессор принудительно сортирует дочерние элементы внутри `<w:pPr>`, `<w:rPr>`, `<w:tblPr>` и `<w:tcPr>` в строгом соответствии со спецификацией OpenXML (ECMA-376), гарантируя отсутствие ошибки «Файл поврежден» при открытии в MS Word.
5. **Двухконтурный Линтер (`Rule-Based AST Linter` + `Visual Gate`):**
   * **Контур 1 (Детерминированный):** Проверка геометрии страниц, полей, отсутствия пустых абзацев (`Enter` вместо отступов), непрерывности заголовков (`H1->H2`), ширин колонок таблиц и наличия `w:tblHeader`/`w:cantSplit`.
   * **Контур 2 (Визуальный, опциональный):** Конвертация через headless LibreOffice в PDF/PNG для VLM-проверки на предмет съехавших колонок и обрезанного текста.
6. **Enterprise Security & Sandbox:**
   * Встроенные флаги изоляции файловой системы (`--sandbox-dir`), лимиты размера файлов (`--max-size`), режим только для чтения (`--readonly`) и фильтрация доступных инструментов (`--whitelist`).

---

## 3. Технологический стек (Tech Stack)

* **Язык:** Go 1.23+
* **Работа с XML DOM и XPath:** `github.com/beevik/etree` (быстрый in-memory DOM с поддержкой пространств имен и XPath-запросов).
* **MCP Протокол:** `github.com/mark3labs/mcp-go` (генерация инструментов, ресурсов и JSON Schema для LLM).
* **CLI Фреймворк:** `github.com/spf13/cobra` (единый бинарник для запуска как `docxcraft mcp` и как CLI-утилиты `docxcraft lint/build/inspect`).
* **Конфигурация и Профили стилей:** `gopkg.in/yaml.v3` (загрузка профилей ГОСТ / Corporate).
* **Валидация и Схемы:** `github.com/invopop/jsonschema` + стандартный `encoding/json`.
* **Тестирование:** стандартный `testing` + `github.com/stretchr/testify` + Golden-тесты XML-фрагментов.

---

## 4. Структура репозитория (Repository Layout)

Проект следует стандарту **Standard Go Project Layout** с жестким разделением вычислительного ядра (`internal/`) и транспортных адаптеров (`cmd/`, `pkg/`):

```text
docxcraft/
├── AGENTS.md                    # Настоящий манифест и правила для ИИ-агентов и разработчиков
├── go.mod
├── go.sum
├── Makefile                     # Сборка, линтеры кода, прогон тестов и генерация схем
├── cmd/
│   └── docxcraft/
│       └── main.go              # Точка входа CLI (команды: mcp, build, patch, lint, inspect)
├── profiles/                    # Встроенные YAML-профили стандартов оформления (embed.FS)
│   ├── gost_7_32_2017.yaml
│   ├── corporate_executive.yaml
│   └── legal_contract.yaml
├── specs/                       # Детальные технические спецификации узлов системы (см. Раздел 6)
│   ├── 01_oxml_dom_and_anchors.md
│   ├── 02_style_profiles_and_ast.md
│   ├── 03_ecma376_normalizer.md
│   ├── 04_linter_and_visual_gate.md
│   └── 05_mcp_tools_and_security.md
├── internal/
│   ├── archive/                 # In-memory чтение/запись .docx (ZIP-контейнер)
│   ├── oxml/                    # Обертки над beevik/etree, работа с неймспейсами, нормализатор тегов
│   ├── anchor/                  # Индексация документа, генерация и поиск по w14:paraId
│   ├── profile/                 # Парсер и модели YAML-профилей стилей (единицы измерения: twips/dxa, pt, cm)
│   ├── compiler/                # Сборка нового документа из JSON AST + Style Profile
│   ├── patcher/                 # Точечные мутации: copy_format_from, Track Changes (w:ins/w:del), шаблоны
│   ├── linter/                  # Движок правил проверки документа + интеграция с LibreOffice Headless
│   ├── security/                # Песочница путей, лимиты памяти и whitelist инструментов
│   └── mcpserver/               # Регистрация инструментов и хендлеров для mark3labs/mcp-go
├── pkg/
│   └── docxcraft/               # Публичный Go SDK для прямого импорта в другие Go-проекты
└── testdata/
    ├── fixtures/                # Эталонные .docx файлы (сложные таблицы, шаблоны с разорванными тегами)
    └── golden/                  # Ожидаемые XML-слепки для регрессионного тестирования
```

---

## 5. Стили ведения репозитория и Код-стайл (Engineering & Repo Standards)

### 5.1. Правила написания Go-кода
1. **Никаких глобальных состояний:** Все операции с документами инкапсулируются в структуре `DocumentSession` (in-memory контейнер). Это позволяет безопасно обрабатывать десятки документов параллельно в горутинах.
2. **Единицы измерения (Single Source of Truth):**
   * Внутри пакета `internal/oxml` все геометрические расчеты (поля, отступы, ширины ячеек) ведутся **исключительно в `dxa` (twips — двадцатых долях пункта)** и `half-points` (для размеров шрифтов).
   * Конвертация из пользовательских единиц (`cm`, `mm`, `in`, `pt`) происходит строго на границе парсинга YAML-профиля или JSON AST в пакете `internal/profile/units.go`.
3. **Строгая обработка ошибок:**
   * Запрещено использовать `panic` внутри библиотечного кода.
   * Ошибки должны оборачиваться через `fmt.Errorf("oxml normalizer: %w", err)` и содержать понятный для LLM контекст (например, какой именно `anchor_id` не найден или какое правило профиля нарушено), чтобы агент мог самостоятельно исправить свой запрос.
4. **Безопасная работа с XML (`beevik/etree`):**
   * Запрещено формировать сложные XML-узлы конкатенацией строк (`fmt.Sprintf("<w:p>...")`). Используются только типизированные конструкторы узлов `etree.NewElement()` и хелперы пакета `internal/oxml`.
   * Перед сохранением любого измененного узла `<w:pPr>`, `<w:rPr>`, `<w:tblPr>`, `<w:tcPr>` **обязателен** вызов `oxml.NormalizeElementOrder(elem)`.

### 5.2. Тестирование и Контроль качества (QA)
1. **Golden XML Testing:** Каждая функция компилятора или патчера покрывается тестом, который сравнивает сгенерированное поддерево XML (`<w:tbl>`, `<w:p>`) с эталонным файлом в `testdata/golden/`.
2. **Round-Trip Integrity Test:** Обязательный тест для `internal/archive` и `internal/oxml`: открыть сложный реальный договор `.docx` из `testdata/fixtures/`, изменить один абзац по `w14:paraId`, сохранить и проверить, что все остальные файлы архива и нетронутые ветки XML остались идентичными оригиналу.
3. **Линтинг кода:** Перед любым коммитом код должен проходить `golangci-lint run` и `go test -race ./...`.

### 5.3. Правила ведения Git и Коммитов
* Используется стандарт **Conventional Commits**:
  * `feat(anchor): implement w14:paraId indexing and XPath lookup`
  * `feat(oxml): add ECMA-376 child tag sorter for w:tcPr`
  * `fix(template): merge split runs inside {{ placeholder }} tags`
  * `docs(specs): add specification for style profile schema`
* Ветка `main` всегда должна собираться в единый статический бинарник (`CGO_ENABLED=0 go build`).

---

## 6. Карта спецификаций системы (Where to Find Specifications)

При реализации любого модуля разработчик или ИИ-агент **обязан** сверяться с соответствующей технической спецификацией в директории `specs/`. Ниже приведена маршрутная карта документации, которая определяет контракты каждого компонента:

| Файл спецификации | За какой модуль отвечает | Что описывает спецификация |
| :--- | :--- | :--- |
| **`specs/01_oxml_dom_and_anchors.md`** | `internal/archive`, `internal/oxml`, `internal/anchor` | Архитектура In-Memory ZIP, работа с пространствами имен WordprocessingML (`w:`, `w14:`, `r:`), алгоритм генерации и поиска стабильных якорей `w14:paraId`, алгоритм склейки разорванных ранов (`Run Coalescing`) для шаблонизатора. |
| **`specs/02_style_profiles_and_ast.md`** | `internal/profile`, `internal/compiler`, `profiles/*.yaml` | Полная схема YAML-профиля стилей (ГОСТ / Corporate) и JSON Schema входного AST документа (блоки: заголовки, абзацы, умные таблицы, списки, выноски, разрывы страниц). Правила расчета ширин колонок таблиц. |
| **`specs/03_ecma376_normalizer.md`** | `internal/oxml/normalizer.go`, `internal/patcher` | Таблицы приоритетов сортировки дочерних XML-тегов по стандарту ECMA-376 для `<w:pPr>`, `<w:rPr>`, `<w:tblPr>`, `<w:trPr>`, `<w:tcPr>`. Спецификация клонирования форматов (`copy_format_from`) и разметки рецензирования (`<w:ins>`, `<w:del>`). |
| **`specs/04_linter_and_visual_gate.md`** | `internal/linter` | Каталог детерминированных правил аудита документа (коды ошибок `DF-001` .. `DF-020`), структура JSON-отчета линтера для самокоррекции LLM, протокол вызова `LibreOffice --headless` для визуального рендера. |
| **`specs/05_mcp_tools_and_security.md`** | `internal/mcpserver`, `internal/security`, `cmd/docxcraft` | Сигнатуры всех MCP-инструментов (входные/выходные JSON-контракты), дизайн CLI-команд, работа in-memory сессий документов и правила песочницы (`--sandbox-dir`, `--readonly`, `--whitelist`). |

---

## 7. Алгоритм работы для ИИ-агентов (Workflow for Coding Agents)

Если вы — ИИ-агент, работающий над кодовой базой DocxCraft:
1. **Перед написанием кода** прочитайте соответствующий документ в папке `specs/`. Не выдумывайте собственные названия XML-тегов или структуры JSON AST — строго следуйте контрактам из спецификаций.
2. **При добавлении новой мутации XML** убедитесь, что она:
   * Работает через `anchor_id` (`w14:paraId`), а не через индекс массива.
   * Вызывает нормализатор порядка элементов `oxml.NormalizeElementOrder()`.
   * Сопровождается unit-тестом в соответствующем пакете.
3. **Зависимости:** Не добавляйте тяжелые внешние библиотеки или C-зависимости (`cgo`). Проект должен оставаться чистым Go-приложением (`CGO_ENABLED=0`), компилируемым под Linux, macOS и Windows за секунды.

---

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `JuliossJunk/docxcraft` (via `gh`). See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical roles map 1:1 to tracker labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` + `docs/adr/`. See `docs/agents/domain.md`.
