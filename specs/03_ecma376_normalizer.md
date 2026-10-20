# Specification 03: OXML Schema Normalizer & ECMA-376 Sequence Enforcer

**Target Packages:** `internal/oxml` (`normalizer.go`), `internal/patcher`  
**Status:** Draft / Core Architecture  
**Dependencies:** `github.com/beevik/etree`

---

## 1. Назначение и проблема («Word found unreadable content»)

Спецификация OpenXML (ECMA-376) жестко регламентирует порядок дочерних элементов внутри таких контейнеров свойств, как `<w:pPr>` (свойства абзаца), `<w:rPr>` (свойства текста), `<w:tblPr>` (свойства таблицы) и `<w:tcPr>` (свойства ячейки).

Если LLM или кастомный скрипт сгенерирует XML, где `<w:shd>` (заливка) идет перед `<w:tcBorders>` (границами), или `<w:spacing>` (отступы абзаца) нарушает последовательность, **MS Word при открытии файла выдает критическую ошибку**:
> *«Word found unreadable content in filename.docx. Do you want to recover the contents of this document?»*

Нормализатор работает как автоматический компилятор-санитайзер: перед тем как упаковать измененный XML-документ обратно в ZIP-архив, он приводит порядок всех дочерних тегов в строкое соответствие со схемой ECMA-376.

---

## 2. Таблицы приоритетов и последовательности (ECMA-376 Rank Maps)

Для каждого управляющего контейнера задается фиксированный порядок сортировки дочерних элементов. Теги, не вошедшие в таблицу приоритетов, сохраняют свой относительный порядок и сдвигаются в конец контейнера.

### 2.1. Контейнер свойств абзаца (`<w:pPr>`)
Критически важный порядок для отступов, стилей, выравнивания и межстрочных интервалов:

| Приоритет | Имя тега (`etree` LocalName) | Назначение |
| :---: | :--- | :--- |
| 1 | `pStyle` | Ссылка на стиль абзаца (`Heading1`, `Normal`) |
| 2 | `keepNext` | Запрет разрыва страницы со следующим абзацем |
| 3 | `keepLines` | Запрет разрыва строк внутри абзаца |
| 4 | `pageBreakBefore` | Принудительный разрыв страницы перед абзацем |
| 5 | `widowControl` | Контроль висячих строк |
| 6 | `numPr` | Свойства списка (нумерация, уровень) |
| 7 | `pBdr` | Границы абзаца |
| 8 | `shd` | Заливка фона абзаца |
| 9 | `spacing` | Межстрочные интервалы (`line`, `before`, `after`) |
| 10 | `ind` | Отступы слева, справа, красная строка (`left`, `right`, `firstLine`) |
| 11 | `jc` | Выравнивание текста (`left`, `center`, `right`, `both`) |
| 12 | `rPr` | Свойства текста по умолчанию для абзаца |
| 13 | `sectPr` | Параметры раздела, привязанные к абзацу |

### 2.2. Контейнер свойств текста / рана (`<w:rPr>`)
Порядок шрифтов, начертаний, цветов и размеров:

| Приоритет | Имя тега | Назначение |
| :---: | :--- | :--- |
| 1 | `rStyle` | Стиль символов |
| 2 | `rFonts` | Гарнитура шрифта (Ascii, HAnsi, Cs) |
| 3 | `b` / `bCs` | Жирное начертание |
| 4 | `i` / `iCs` | Курсив |
| 5 | `strike` / `dstrike` | Зачеркивание |
| 6 | `u` | Подчеркивание |
| 7 | `shd` | Заливка текста |
| 8 | `color` | Цвет текста (HEX) |
| 9 | `sz` / `szCs` | Кегль шрифта (в полупунктах) |
| 10 | `spacing` | Межсимвольный интервал |
| 11 | `lang` | Языковая локаль |

### 2.3. Контейнер свойств ячейки таблицы (`<w:tcPr>`)
Порядок ширин, слияний и границ ячеек:

| Приоритет | Имя тега | Назначение |
| :---: | :--- | :--- |
| 1 | `cnfStyle` | Условное форматирование ячейки |
| 2 | `tcW` | Ширина ячейки (`w:w`, `w:type`) |
| 3 | `gridSpan` | Объединение колонок (`colspan`) |
| 4 | `vMerge` | Объединение строк (`rowspan`) |
| 5 | `tcBorders` | Границы ячейки (Top, Left, Bottom, Right) |
| 6 | `shd` | Заливка фона ячейки |
| 7 | `tcMar` | Внутренние отступы ячейки (Padding) |
| 8 | `vAlign` | Вертикальное выравнивание (`top`, `center`, `bottom`) |

---

## 3. Алгоритм нормализации (`NormalizeElementOrder`)

Алгоритм работает рекурсивно или итеративно перед сохранением каждого измененного XML-файла (`word/document.xml`, `word/styles.xml` и т.д.):

```go
package normalizer

import (
	"sort"
	"[github.com/beevik/etree](https://github.com/beevik/etree)"
)

var pPrOrder = map[string]int{
	"pStyle":           1,
	"keepNext":         2,
	"keepLines":        3,
	"pageBreakBefore":  4,
	"widowControl":     5,
	"numPr":            6,
	"pBdr":             7,
	"shd":              8,
	"spacing":          9,
	"ind":              10,
	"jc":               11,
	"rPr":              12,
	"sectPr":           13,
}

var tcPrOrder = map[string]int{
	"cnfStyle":  1,
	"tcW":       2,
	"gridSpan":  3,
	"vMerge":    4,
	"tcBorders": 5,
	"shd":       6,
	"tcMar":     7,
	"vAlign":    8,
}

func NormalizeContainer(parent *etree.Element, orderMap map[string]int) {
	// Отбираем только дочерние элементы в пространстве имен "w"
	children := parent.ChildElements()
	if len(children) <= 1 {
		return
	}

	// Сортируем с сохранением стабильности порядка (stable sort) для равных приоритетов
	sort.SliceStable(children, func(i, j int, ) bool {
		tagI := children[i].Tag
		tagJ := children[j].Tag

		rankI, existsI := orderMap[tagI]
		rankJ, existsJ := orderMap[tagJ]

		if existsI && existsJ {
			return rankI < rankJ
		}
		if existsI && !existsJ {
			return true // Известные теги идут раньше неизвестных
		}
		if !existsI && existsJ {
			return false
		}
		return false // Относительный порядок неизвестных тегов сохраняется
	})

	// Переупорядочиваем элементы в исходном etree.Element
	// 1. Очищаем детей
	parent.RemoveChildren()
	// 2. Добавляем отсортированные обратно
	for _, child := range children {
		parent.AddChild(child)
	}
}
```

---

## 4. Автоматическая интеграция в пайплайн сохранения

Нормализатор вызывается **абсолютно прозрачно** для пользователя и агента внутри метода `Package.Save()`:

```go
func (p *Package) Save(w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for path, doc := range p.xmlDocs {
		if !p.dirty[path] {
			continue
		}

		// Автоматический проход по всему DOM документу перед сериализацией
		doc.FindElements("//w:pPr").Each(func(el *etree.Element) {
			NormalizeContainer(el, pPrOrder)
		})
		doc.FindElements("//w:rPr").Each(func(el *etree.Element) {
			NormalizeContainer(el, rPrOrder)
		})
		doc.FindElements("//w:tcPr").Each(func(el *etree.Element) {
			NormalizeContainer(el, tcPrOrder)
		})
		// Аналогично для w:tblPr, w:trPr и т.д.
	}

	// Запись в zip-архив...
}
```

---

## 5. Требования к тестам (`internal/oxml/normalizer_test.go`)

1. **`TestSchemaNormalization`**: Создать намеренно поврежденный XML-фрагмент с нарушенным порядком тегов внутри `<w:tcPr>` (например, `<w:shd>` перед `<w:tcW>`), прогнать через `NormalizeContainer`, проверить строгое соответствие порядку из таблицы `tcPrOrder`.
2. **`TestRoundTripNoCorruption`**: Сгенерировать документ с помощью таблиц и стилей, умышленно переставив свойства в коде, сохранить через нормализатор и программно проверить валидность структуры XML-файла.