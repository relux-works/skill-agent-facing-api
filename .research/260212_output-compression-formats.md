# Output Compression Formats for LLM Agents

**Date:** 2026-02-12
**Epic:** EPIC-260212-1d8i05 (output-compression)
**Source:** [TOON vs JSON — dev.to](https://dev.to/akki907/toon-vs-json-the-new-format-designed-for-ai-nk5)

---

## TOON (Token-Oriented Object Notation)

### Concept

Schema-once + values-only для однородных данных. Вместо повторения ключей в каждом объекте — объявление schema один раз, далее CSV-style строки значений.

### Syntax

```
# JSON:
{"users": [
  {"id": 1, "name": "Alice", "role": "admin", "salary": 75000},
  {"id": 2, "name": "Bob", "role": "user", "salary": 65000}
]}

# TOON:
users[2]{id,name,role,salary}:
1,Alice,admin,75000
2,Bob,user,65000
```

### Key Features

- **Tabular array optimization** — schema declared once, rows as values
- **Smart quoting** — кавычки только когда необходимо (delimiter в значении, leading/trailing spaces)
- **Indentation-based nesting** — YAML-like, без лишних скобок
- **Explicit array lengths** — `[N]` улучшает accuracy парсинга LLM

### Benchmarks (из статьи)

| Dataset | JSON tokens | TOON tokens | Reduction |
|---------|-------------|-------------|-----------|
| GitHub Repos (100 records) | 15,145 | 8,745 | **42.3%** |
| Analytics (180 days) | 10,977 | 4,507 | **58.9%** |
| E-commerce Orders | 257 | 166 | **35.4%** |
| **Average** | | | **~46.3%** |

### LLM Accuracy (154 вопросов, 4 модели)

| Format | Accuracy |
|--------|----------|
| TOON | 70.1% |
| JSON | 65.4% |

Модели: GPT-5 Nano, Claude Haiku, Gemini Flash, Grok.

---

## Альтернативы

### CSV/TSV

- Ещё компактнее чем TOON (~30% лучше для flat data по комментариям)
- Проблема: нет стандартного способа для nested data
- Delimiter choice влияет на tokenization (tab vs comma vs pipe)

### YAML-табличный

- Более читаемый чем TOON для людей
- Но overhead на indentation для больших списков

### Custom compact DSL

- Можно сделать формат специфичный для agentquery
- Максимальная компрессия, но нужен парсер на стороне агента

---

## Применимость к agentquery

### Где максимальный выигрыш

1. **`list()` с множеством записей** — однородные объекты с одинаковыми полями → schema-once идеально
2. **`schema()` output** — структурированные описания полей/операций → табличный формат
3. **`Search()` results** — повторяющиеся file paths → группировка по файлам

### Где JSON лучше оставить

1. **`get()` с nested data** — единичный объект, нет повторения ключей
2. **Error responses** — простые объекты, компрессия минимальна
3. **Heterogeneous batch results** — разные типы в одном batch

### Преимущество agentquery

Schema уже известна на этапе сериализации (Schema[T] знает все зарегистрированные поля). Header можно генерить автоматически из FieldSelector — не нужен отдельный schema declaration синтаксис.

---

## Архитектурная заметка (из обсуждения с пользователем)

**Конфигурация при регистрации схемы** — пользователь хочет указывать output mode на уровне Schema:

```go
schema := agentquery.NewSchema[Task](agentquery.SchemaConfig{
    OutputFormat: agentquery.FormatCompact, // or FormatJSON, FormatAuto
})
```

Это позволяет domain-specific выбор: CLI для людей → JSON (pretty), CLI для агентов → compact. Формат определяется один раз при создании Schema, а не per-query.

---

## TODO

- [ ] Benchmark текущего JSON output (baseline)
- [ ] Prototype табличного формата для list()
- [ ] Сравнить token counts: JSON vs TOON vs CSV vs custom
- [ ] Определить оптимальный delimiter (comma vs tab vs pipe)
- [ ] Тесты accuracy парсинга LLM на наших данных
