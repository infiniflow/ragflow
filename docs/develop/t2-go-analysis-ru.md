# T2: широкий наблюдательный анализ Go

Дата обновления: 2026-09-08. Статус: report-only наблюдатель реализован для всего Go-модуля; это часть T2, а не компиляция, архитектурный gate, проверка мёртвого кода или завершение T2.

## Запуск

Из корня репозитория в PowerShell:

```powershell
.venv\Scripts\python.exe -B tools/quality/inspect_go.py --output output/quality/go-analysis.json
.venv\Scripts\python.exe -m pytest test/unit_test/tools/quality/test_inspect_go.py -q
```

Профили задаются в `tools/quality/go-analysis.yaml`. Наблюдатель не требует установленного `go`: он читает package/import headers, build constraints и directives собственным консервативным parser. Это позволяет сформировать граф на текущем Windows host, где Go toolchain отсутствует, но не позволяет объявлять синтаксис, типы, CGO linkage или тесты проверенными. Сборка и package tests остаются за `build.sh` и [build-profile planner](t2-go-build-profiles-ru.md).

Код выхода 0 означает только `OBSERVED`: объявленный статический граф собран без известных parser/resolution/profile gaps. Код 2 и `INCOMPLETE` означает ошибку package/import header, build expression, локального target, entrypoint, provenance или входного снимка. `policy_status` всегда `NOT_EVALUATED`, `dead_code_status` — `NOT_ANALYZED`; `OBSERVED` не является `PASS` ARC-01/02/03, DEAD-01 или BUILD-01.

## Что анализируется

- Все неигнорируемые `.go` в текущем T0: runtime и test-файлы, package names, точные imports/aliases, blank/dot imports, локальные edges модуля `ragflow`, внешние imports и `import "C"`.
- `//go:build`, legacy `// +build` с отрицанием, comma-AND, space-OR и многострочным AND, стандартные filename constraints GOOS/GOARCH и release tags до объявленной версии профиля; некорректное legacy-выражение даёт `INCOMPLETE`.
- Два раздельных Linux/amd64-профиля: основной CGO и видимый no-CGO вариант. Взаимоисключающие реализации не смешиваются в одном cycle graph.
- Реальные entrypoints `cmd/ragflow_server.go` и `cmd/ragflow-cli.go`. Для CLI явно записано исключение build constraint: штатный `build.sh` компилирует этот файл напрямую, несмотря на conventional tag `ignore`. No-CGO профиль не объявляется результатом `build.sh`.
- Package reachability от точных imports каждого standalone entrypoint, reverse consumers собственных пакетов и strongly connected components внутри каждого активного профиля.
- Directives `//go:embed`, `//go:generate`, `//go:linkname` и другие `//go:*` как registration/integration evidence.

Tests индексируются и входят в общий reverse-consumer inventory, но не в runtime reachability. Entrypoint package учитывается как корень, однако соседний standalone `main`-файл не добавляет свои imports другому entrypoint. Внешние модули не обходятся транзитивно. Embed assets, linkname target, generated outputs и C headers не проверяются.

Parser намеренно не является заменой `go/parser`, compiler или `go list`: после import declarations он не проверяет полную грамматику, types, init side effects, symbol usage, buildable package set и native linkage. Поэтому отсутствие цикла или достижимость пакета остаются статическим наблюдением. Недостижимый пакет либо отсутствие входящего import не означают dead code: tests, tools, standalone binaries, генераторы, reflection, linker directives и внешние consumers требуют отдельных доказательств.

## Проверки реализации

Одиннадцать фикстур проверяют aliases/blank imports, raw literals, header-only build constraints и directives; семантику отрицания/AND/OR и ошибку legacy `+build`; разделение CGO/no-CGO; reachability, reverse consumers и cycle signals; отсутствие ложного цикла между взаимоисключающими variants; ошибки build expression и локальной цели; T0 provenance, output path, исходные байты, профиль и конкурентное изменение. Wrapper проходит Ruff и не запускает Go-код.

Инструмент записывает SHA всех Go sources, policy, `go.mod` и самого wrapper, а также HEAD, upstream base и T0 snapshot fingerprint. Отчёт разрешён только в игнорируемом пути репозитория и создаётся после повторной проверки исходных байтов; исходники он не меняет.

## Выполненный прогон

Проиндексировано **1 318** файлов: 769 runtime и 549 test-файлов в **105** каталогах пакетов. Записано 7 595 import edges, включая 1 627 локальных edges и 5 CGO imports. Текущий T0 относит 16 Go-файлов в 7 пакетах к `core_change`/`extension`; все семь либо достижимы в основном CGO-профиле, либо являются его entrypoint package. Это не dead-code verdict.

Профиль `linux-cgo` выбрал 757 runtime-файлов в 102 пакетах и 327 уникальных package edges. От двух entrypoints достижимы 59 пакетов, 43 не достигнуты; package cycles не найдены. Для `cmd/ragflow-cli.go` один раз использовано документированное constraint override.

Профиль `linux-no-cgo` выбрал 746 runtime-файлов в 101 пакете и 322 package edges. От server entrypoint достижимы 54 пакета, 47 не достигнуты; package cycles также не найдены. Эти числа не подтверждают сборку no-CGO варианта.

Отдельно записаны 7 реальных `go:embed` directives, 1 `go:generate` и 1 test-only `go:linkname`. Итог: `OBSERVED / NOT_EVALUATED / NOT_ANALYZED`, код выхода 0, 0 incomplete reasons. Актуальный локальный отчёт: `output/quality/go-analysis.json`.

[Объединяющий runtime-graph classifier](t2-runtime-graph-classification-ru.md) завершил этот срез: два production main, два developer-tool main и девять directives классифицированы точно; package cycles и недостижимых owned packages нет. TypeScript signals и дополнительные frontend roots разобраны в том же отчёте. Три selector scope остаются report-only, без baseline/ignore и без архитектурного/dead-code verdict. CI enforcement относится к T3.
