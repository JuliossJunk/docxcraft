# Specification 01: OXML In-Memory Container, DOM & Stable Anchors

**Target Packages:** `internal/archive`, `internal/oxml`, `internal/anchor`  
**Status:** Draft / Core Architecture  
**Dependencies:** `archive/zip`, `bytes`, `crypto/rand`, `github.com/beevik/etree`

---

## 1. Назначение и ключевой инвариант (Lossless Guarantee)

Главное требование к уровню `oxml` — **гарантия сохранения 100% верстки и метаданных (Lossless Read-Modify-Write)**.
Если документ открыт и сохранен без вызовов мутаторов, сгенерированный `.docx` должен быть функционально идентичен исходному: все встроенные шрифты, графики, колонтитулы, настройки секций (`w:sectPr`), макросы и кастомные XML-части обязаны сохраниться байт-в-байт.

---

## 2. In-Memory ZIP Container (`internal/archive` + `internal/oxml`)

Файл `.docx` представляет собой ZIP-архив (Open Packaging Conventions, ECMA-376 Part 2). Никаких временных папок на диске (`/tmp`) при работе не создается.

### 2.1. Архитектура хранения в памяти (`Package`)
При открытии файла архив делится на два пула данных:
1. **`rawFiles map[string][]byte`** — бинарные ассеты (`word/media/*`, `docProps/*`, `[Content_Types].xml`, `_rels/*`) и XML-файлы, к которым еще не обращались. При сохранении они переписываются в выходной `zip.Writer` напрямую без парсинга.
2. **`xmlDocs map[string]*etree.Document`** — лениво распарсенные (lazy-loaded) DOM-деревья только тех частей, которые реально читаются или модифицируются в текущей сессии (например, `word/document.xml`, `word/styles.xml`).

```go
package oxml

import (
	"sync"
	"[github.com/beevik/etree](https://github.com/beevik/etree)"
)

type Package struct {
	mu       sync.RWMutex
	rawFiles map[string][]byte          // Неизмененные файлы архива (байт-в-байт)
	xmlDocs  map[string]*etree.Document // Активные распарсенные XML-деревья
	dirty    map[string]bool            // Флаг модификации для сериализации
	index    *AnchorIndex               // Индекс O(1) для поиска по w14:paraId
}
```

### 2.2. Жизненный цикл чтения и записи
1. **`Open(r io.ReaderAt, size int64) (*Package, error)`**:
   * Читает все записи из `zip.Reader`.
   * Защищается от **Zip Bomb**: суммарный размер распакованных данных жестко ограничен (по умолчанию `MaxUncompressedSize = 100 MB`).
   * Сразу загружает только `word/document.xml`, строит индекс якорей и запускает санитайзер шаблонных ранов.
2. **`GetXML(partName string) (*etree.Document, error)`**:
   * Если `partName` уже в `xmlDocs` — возвращает указатель.
   * Иначе берет байты из `rawFiles[partName]`, парсит через `etree.NewDocument()`, кладет в `xmlDocs` и помечает в `dirty`.
3. **`Save(w io.Writer) error`**:
   * Перед сериализацией вызывает **OXML Schema Normalizer** (см. `specs/03_ecma376_normalizer.md`) на всех `dirty` документах.
   * Для каждого файла из исходного архива: если он есть в `dirty`, сериализует `*etree.Document` в `zip.Writer` с компактным форматированием (без лишних переносов строк, которые Word может интерпретировать как пробелы в `<w:t>`). Иначе — пишет сырые байты из `rawFiles`.

### 2.3. Настройки `beevik/etree` (Критично для Word!)
По умолчанию XML-парсеры могут менять отступы или схлопывать пустые теги. Для `etree.Document` обязательно выставляются настройки:
```go
doc := etree.NewDocument()
doc.WriteSettings = etree.WriteSettings{
	CanonicalEndTags: false, // Self-closing теги (<w:b/>) обязательны для компактности
	CanonicalText:    true,  // Экранирование спецсимволов (&, <, >)
	CanonicalAttrVal: true,
}
// ВАЖНО: Никогда не вызывать doc.Indent() на word/document.xml!
// Искусственные пробелы и \n между <w:r> или внутри <w:t> ломают верстку в MS Word.
```

---

## 3. Работа с пространствами имен (XML Namespaces)

В `beevik/etree` префикс пространства имен хранится в поле `Element.Space`, а локальное имя тега — в `Element.Tag`.

### 3.1. Реестр стандартных префиксов
Харнесс обязан гарантировать наличие следующих объявлений в корневом теге `<w:document>`:

| Префикс (`Space`) | URI пространства имен | Назначение |
| :--- | :--- | :--- |
| `w` | `http://schemas.openxmlformats.org/wordprocessingml/2006/main` | Базовая разметка текста, таблиц и стилей |
| `w14` | `http://schemas.microsoft.com/office/word/2010/wordml` | Стабильные якоря `w14:paraId` и `w14:textId` |
| `r` | `http://schemas.openxmlformats.org/officeDocument/2006/relationships` | Ссылки на картинки, гиперссылки и колонтитулы |
| `wp` | `http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing` | Позиционирование изображений и фигур |
| `mc` | `http://schemas.openxmlformats.org/markup-compatibility/2006` | Совместимость версий (`mc:Ignorable="w14"`) |

### 3.2. Правило `mc:Ignorable`
Если в старом документе отсутствует пространство `xmlns:w14`, при первой индексации харнесс автоматически добавляет атрибут `xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` в `<w:document>` и дописывает `w14` в атрибут `mc:Ignorable`, чтобы любая версия MS Word открывала файл без предупреждений.

### 3.3. Правило пробелов `xml:space="preserve"`
Любой элемент `<w:t>`, содержащий начальные или конечные пробелы (например, `" договор "` или `"Отчет: "`), **обязан** получать атрибут `xml:space="preserve"`. Без этого MS Word при открытии файла молча съест крайние пробелы, и слова склеятся.

```go
func SetRunText(tElem *etree.Element, text string) {
	tElem.SetText(text)
	if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
		tElem.CreateAttr("xml:space", "preserve")
	} else {
		tElem.RemoveAttr("xml:space")
	}
}
```

---

## 4. Система стабильных якорей (`w14:paraId`)

Вместо хрупкой адресации по порядковому номеру (`paragraph[14]`), которая сбивается при первой же вставке или удалении строки, каждый блок документа адресуется по неизменяемому 8-символьному HEX-идентификатору.

### 4.1. Ограничения стандарта MS Word на `w14:paraId`
По спецификации `[MS-DOCX]`:
1. Значение `w14:paraId` — ровно **8 шестнадцатеричных символов в верхнем регистре** (`ST_LongHexNumber`, например `2A7F09BC`).
2. Значение `00000000` и значения `>= 0x80000000` (бит знака) зарезервированы внутренними механизмами Word.
3. Поэтому валидный диапазон генерации: **`0x00000001` — `0x7FFFFFFF`**.

### 4.2. Алгоритм индексации и генерации (`AnchorIndex`)
Индекс хранит прямые указатели на узлы `*etree.Element` в памяти, обеспечивая поиск за **$O(1)$** без повторного обхода дерева:

```go
type AnchorTarget struct {
	ID       string
	Kind     string         // "p" (абзац), "tbl" (таблица), "tr" (строка таблицы)
	Element  *etree.Element // Прямой указатель на узел в DOM
	Parent   *etree.Element // Указатель на контейнер (w:body или w:tc)
	Location string         // "word/document.xml", "word/header1.xml" и т.д.
}

type AnchorIndex struct {
	byID map[string]*AnchorTarget
}
```

**Процедура `EnsureAnchors(doc *etree.Document)`:**
1. Обходит все узлы `<w:p>` и `<w:tbl>` в документе (включая вложенные в ячейки таблиц).
2. Для `<w:p>`:
   * Проверяет атрибут `w14:paraId`.
   * Если атрибут есть, валиден и уникален — регистрирует его в `AnchorIndex`.
   * Если атрибута нет или обнаружен дубликат (бывает при копипасте внутри Word) — генерирует новый уникальный ID (`0x00000001..0x7FFFFFFF`), записывает в `w14:paraId` и `w14:textId` и сохраняет в индекс.
3. Для `<w:tbl>`:
   * В OpenXML у самого тега `<w:tbl>` нет нативного `w14:paraId`, но он есть у строк `<w:tr w14:paraId="...">`.
   * Для таблицы якорем назначается либо `w14:paraId` ее первой строки с префиксом `tbl_`, либо кастомный атрибут в памяти, мапящийся на `<w:tbl>`.

```go
func generateValidParaID(existing map[string]struct{}) string {
	var b [4]byte
	for {
		_, _ = rand.Read(b[:])
		b[0] &= 0x7F // Гарантируем, что значение < 0x80000000
		id := fmt.Sprintf("%02X%02X%02X%02X", b[0], b[1], b[2], b[3])
		if id != "00000000" {
			if _, exists := existing[id]; !exists {
				return id
			}
		}
	}
}
```

### 4.3. Скелет документа для LLM (`InspectSkeleton`)
Когда агент вызывает инструмент чтения документа, харнесс не отдает ему сырой XML. Он возвращает компактный массив якорей:
```json
[
  {"anchor": "1A4F09C1", "type": "p", "style": "Heading1", "text": "1. Предмет договора"},
  {"anchor": "3B8E12A4", "type": "p", "style": "Normal", "text": "Исполнитель {{ contractor_name }} обязуется..."},
  {"anchor": "tbl_7C01D2E9", "type": "tbl", "rows": 4, "cols": 3, "header_preview": ["№", "Наименование", "Сумма"]}
]
```
Агент может вставить 10 новых абзацев после `"1A4F09C1"`, а затем обратиться к `"3B8E12A4"` — и попадет точно в цель.

---

## 5. Алгоритм склейки разорванных ранов (`Run Coalescing & Template Healing`)

### 5.1. В чем проблема MS Word (Split Runs Problem)
Когда человек набирает в Word шаблон `{{ client_name }}`, редактор (из-за проверки орфографии, смены раскладки или автосохранения) разбивает эту строку на несколько независимых тегов `<w:r>`:

```xml
<!-- Как это выглядит внутри сырого word/document.xml -->
<w:p w14:paraId="3B8E12A4">
  <w:r><w:rPr><w:b/></w:rPr><w:t>{{ </w:t></w:r>
  <w:ProofErr w:type="spellStart"/>
  <w:r><w:rPr><w:b/></w:rPr><w:t>client_</w:t></w:r>
  <w:ProofErr w:type="spellEnd"/>
  <w:r><w:rPr><w:b/></w:rPr><w:t>name</w:t></w:r>
  <w:r><w:rPr><w:b/></w:rPr><w:t> }}</w:t></w:r>
</w:p>
```
Если искать переменные простым поиском по `<w:t>`, ни один тег не совпадет с регулярным выражением `\{\{.*?\}\}`.

### 5.2. Двухфазный алгоритм `HealRuns(p *etree.Element)`

Алгоритм запускается автоматически при открытии документа (или перед рендером шаблона) для каждого `<w:p>`:

#### Фаза 1: Очистка мусорных маркеров и слияние идентичных ранов
1. Удаляются все сервисные теги орфографии и грамматики (`<w:proofErr>`, `<w:bookmarkStart>` / `<w:bookmarkEnd>` для временных системных закладок `_GoBack`), стоящие между `<w:r>`.
2. Игнорируются атрибуты ревизий `w:rsidR`, `w:rsidRPr`, `w:rsidRDefault` (именно из-за разных `rsid` Word дробит одинаково отформатированный текст на десятки ранов).
3. Если два соседних `<w:r>` имеют **идентичное дерево `<w:rPr>`** (сравнение канонического сериализованного XML блока `<w:rPr>`) и оба содержат только простые `<w:t>` (без `<w:br/>`, `<w:tab/>`, `<w:drawing>`), текст второго рана дописывается в `<w:t>` первого рана (с вызовом `SetRunText` для `xml:space="preserve"`), а второй `<w:r>` удаляется из DOM.

#### Фаза 2: Принудительная склейка шаблонных скобок `{{ ... }}` и `{% ... %}`
Бывает, что пользователь случайно выделил жирным только одну скобку `{{`, из-за чего `<w:rPr>` у кусков переменной разный, и Фаза 1 их не слила.
1. Собирается виртуальная строка всего текста абзаца `fullText` вместе с картой смещений (какой индекс символа какому `<w:r>` и `<w:t>` принадлежит).
2. Регулярным выражением `(\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\})` находятся все шаблонные конструкции в `fullText`.
3. Для каждого совпадения `[startOffset, endOffset]`:
   * Определяется стартовый ран `rStart` и конечный ран `rEnd`.
   * Если `rStart != rEnd`:
     * Текст до `startOffset` в `rStart` + сам шаблонный тег `{{ var }}` + остаток текста в `rEnd` собираются воедино.
     * Весь текст шаблонного тега помещается в `<w:t>` стартового рана `rStart` (наследуя его форматирование `<w:rPr>`, либо форматирование рана, где находилось само имя переменной `var`).
     * Промежуточные раны между `rStart` и `rEnd` очищаются/удаляются.

```text
До HealRuns:   [Run1: "Сумма: {"] -> [ProofErr] -> [Run2: "{ total_"] -> [Run3: "price }} руб."]
После HealRuns:[Run1: "Сумма: {{ total_price }} руб."] (с сохранением шрифта и xml:space="preserve")
```

---

## 6. Мутаторы уровня DOM (Базовые примитивы для следующих слоев)

Пакет `internal/oxml` экспортирует строго детерминированные примитивы, которыми пользуются `compiler`, `patcher` и `mcpserver`:

| Метод | Сигнатура и поведение |
| :--- | :--- |
| **`CloneStyleFrom`** | `CloneStyleFrom(srcAnchor, dstAnchor string) error` — находит оба узла за $O(1)$, делает глубокую копию (`Copy()`) ветки `<w:pPr>` (и `<w:rPr>` первого рана) из источника и заменяет ими стили целевого узла, сохраняя его `w14:paraId`. |
| **`InsertAfterAnchor`** | `InsertAfterAnchor(anchorID string, newElements ...*etree.Element) ([]string, error)` — вставляет новые узлы в родительский контейнер сразу после `anchorID`, присваивает им свежие `w14:paraId`, обновляет `AnchorIndex` и возвращает список новых якорей. |
| **`ReplaceParagraphText`** | `ReplaceParagraphText(anchorID string, newText string, keepFormat bool) error` — заменяет текст абзаца с сохранением `<w:pPr>` и `<w:rPr>` первого рана. |
| **`ApplyTrackedEdit`** | `ApplyTrackedEdit(anchorID, newText, author string, ts time.Time) error` — оборачивает старый ран в `<w:del w:id="..." w:author="..." w:date="...">` (меняя `<w:t>` на `<w:delText>`), а рядом вставляет `<w:ins>` с новым текстом. |

---

## 7. Требования к модульным тестам (`internal/oxml/*_test.go`)

1. **`TestRoundTripLossless`**: Открыть сложный реальный `.docx` (с таблицами, колонтитулами, картинками и секциями), прогнать `Open()` -> `Save()`, проверить что все файлы архива на месте, а `word/document.xml` валиден.
2. **`TestAnchorStability`**: Вставить 5 абзацев в середину документа через `InsertAfterAnchor`, убедиться что `w14:paraId` всех старых абзацев ниже по тексту остались прежними и находятся за $O(1)$.
3. **`TestHealSplitTemplateRuns`**: Подать искусственно раздробленный XML вида `<w:r><w:t>{</w:t></w:r><w:proofErr/><w:r><w:t>{ user_name }</w:t></w:r><w:r><w:t>}</w:t></w:r>`, вызвать `HealRuns` и проверить, что на выходе ровно один `<w:r><w:t>{{ user_name }}</w:t></w:r>`.