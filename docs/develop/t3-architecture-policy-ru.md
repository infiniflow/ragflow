# T3: selector и aggregate архитектурной политики

Дата: 2026-09-08. Статус: `REPORT_ONLY_BOOTSTRAP`. Реализованы локально проверенные selector, aggregate и workflow с устойчивым именем `architecture-policy`. Это начало T3, а не его приёмка: checker ещё отсутствует в базовом commit текущего изменения, workflow не запускался в GitHub, trusted base-policy execution и required branch protection не настроены и не проверены.

## Контракт

`tools/quality/check_architecture_policy.py` ничего не исправляет, не выполняет выбранные инструменты и не изменяет Git index. Действие `select` строит план по явному base commit и текущему T0 snapshot. Оно учитывает committed, staged, unstaged и nonignored untracked paths; `--no-renames` сохраняет обе стороны rename/delete. Текущий путь получает owner/origin из свежего T0, удалённый или возвращённый к upstream путь — owner из `module-map.yaml` базового commit. Неизвестный путь, недоступный base, неполная provenance или изменение дерева во время capture дают код 2.

Машинная политика `tools/quality/architecture-policy.json` определяет пять lanes:

| Lane | Выход | Значение успешного evidence |
| --- | --- | --- |
| `provenance` | встроенный свежий T0 | `OBSERVED`, не behavioral PASS |
| `python-architecture` | `architecture-python.json` | точный настроенный `PASS` |
| `runtime-graph` | `runtime-graph.json` | `CLASSIFIED`; policy остаётся `REPORT_ONLY` |
| `go-build-plan` | `go-build-plan.json` | `PLANNED`; native build остаётся `NOT_EVALUATED` |
| `policy-fixtures` | JUnit | все обязательные negative/positive fixtures реально выполнены без failure/error/skip |

Действие `aggregate` повторно строит selector plan и сравнивает его с сохранённым. Поэтому ручная смена `selected=true` на `false` не превращает применимую проверку в `NOT_APPLICABLE`. Все JSON reports обязаны иметь тот же HEAD, upstream base и T0 fingerprint. Evidence принимается только из свежего каталога рядом с plan; отсутствующий, stale, malformed или созданный другим tool отчёт даёт `INCOMPLETE`. Для JUnit проверяются и testcase nodes, и summary counts. При одновременном finding и неполноте итоговый код 2 сохраняет приоритет `INCOMPLETE`; чистый finding без неполноты даёт код 1.

Успешный текущий aggregate возвращает `REPORT_ONLY_COMPLETE` с `enforcement_status=NOT_ENABLED`. Статусы `CLASSIFIED` и `PLANNED` намеренно не называются `PASS`.

## Локальный запуск

Использовать новый пустой evidence directory; tool откажется перезаписывать старые результаты:

```powershell
$evidence = "output/quality/t3-architecture-policy-local"
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py select --base HEAD --output "$evidence/selection.json"

.\.venv\Scripts\python.exe -B tools/quality/check_architecture.py --base-ref HEAD --output "$evidence/architecture-python.json"
.\.venv\Scripts\python.exe -B tools/quality/inspect_typescript.py --output "$evidence/typescript-analysis.json"
.\.venv\Scripts\python.exe -B tools/quality/inspect_go.py --output "$evidence/go-analysis.json"
.\.venv\Scripts\python.exe -B tools/quality/check_runtime_graph_policy.py --typescript-report "$evidence/typescript-analysis.json" --go-report "$evidence/go-analysis.json" --output "$evidence/runtime-graph.json"
.\.venv\Scripts\python.exe -B tools/quality/check_go_build_profiles.py --output "$evidence/go-build-plan.json"
.\.venv\Scripts\python.exe -B -m pytest -p pytest_asyncio.plugin test/unit_test/tools/quality -q --junitxml="$evidence/policy-fixtures.xml"

.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py aggregate --plan "$evidence/selection.json" --report "python-architecture=$evidence/architecture-python.json" --report "runtime-graph=$evidence/runtime-graph.json" --report "go-build-plan=$evidence/go-build-plan.json" --report "policy-fixtures=$evidence/policy-fixtures.xml" --output "$evidence/aggregate.json"
```

`inspect_typescript.py` сейчас ожидаемо возвращает code 2 из-за одного уже классифицированного computed-import gap. Это не разрешение игнорировать команду: workflow допускает этот промежуточный code только при наличии отчёта, после чего `check_runtime_graph_policy.py` обязан вернуть полную exact-классификацию. Любая другая неполнота или отсутствие файла останавливает aggregate.

## Workflow

`.github/workflows/architecture.yml` запускается на каждом `pull_request`, push в `main` и вручную, без метки `ci` и без path filters. Единственный job и его display name — `architecture-policy`. Каждый run создаёт новый каталог под `RUNNER_TEMP`, готовит только выбранные environments, выполняет reports без `--write`/`--fix`/`git add`, всегда пытается собрать aggregate после успешного selector и сохраняет весь evidence artifact.

Workflow использует read-only permissions и не получает application secrets. Наличие YAML не доказывает, что job выполнялся или обязателен для merge.

## Что осталось до приёмки T3

1. После появления checker в base запускать trusted base implementation/policy против candidate и отдельные tests нового checker. Bootstrap/manual-review marker не должен исчезать простым изменением candidate tool.
2. Выполнить три изолированных PR-пробы: разрешённое изменение проходит; запрещённая зависимость блокируется; unclassified core change или отсутствующий analyzer завершается ошибкой.
3. Включить `architecture-policy` как required check в branch protection/ruleset и проверить, что удаление/переименование/skip job не разрешает merge.
4. Не создавать пустой `baseline.json`: текущий T2 не дал полного architecture/dead-code/build verdict. Существующие exact classifications остаются в своих policy files до появления доказанного более широкого анализа.
