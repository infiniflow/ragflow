# T2: профили сборки изменённого Go-кода

Дата: 2026-09-10. Реализованы report-only политика `tools/quality/go-build-profiles.yaml` и планировщик `tools/quality/check_go_build_profiles.py` версии 0.3.0. Они не заменяют T1 Go regression и будущий T3 CI gate. Цель этого среза — не позволить объявить Go-изменение проверенным, пока каждый затронутый путь из свежего T0 не сопоставлен владельцу и штатной команде `build.sh` с проверяемым результатом. [Широкий Go observer](t2-go-analysis-ru.md) отдельно строит package/import/build-constraint graphs без компиляции; его `OBSERVED` не входит в BUILD-01 PASS. [Runtime-graph classifier](t2-runtime-graph-classification-ru.md) дополнительно проверяет production/developer `package main` и directives, но также не выполняет сборку.

## Запуск

Проверить полноту профилей и получить точную команду без компиляции:

```powershell
.venv/Scripts/python.exe tools/quality/check_go_build_profiles.py --output output/quality/go-build-profiles.json
```

В подготовленном Linux/native-окружении выполнить ту же зафиксированную команду:

```bash
python3 tools/quality/check_go_build_profiles.py --execute --output output/quality/go-build-profiles-executed.json
```

Обычный запуск возвращает `READY/NOT_EVALUATED`, если все изменённые `.go` сопоставлены, но сборка ещё не выполнялась. `--execute` даёт `PASS` только если entrypoints собраны штатным `build.sh --go`, оба объявленных бинарника существуют и все package targets завершились структурированными package `pass` events. Дополнительно обязательны отдельные test `pass` events для всех функций `Test*` из изменённых `_test.go`. Assertion/package failure становится `FAIL`; отсутствующий Linux, `go`, C compiler (`gcc` или `clang`), native toolchain, timeout, ошибка сборки либо отсутствие обязательного evidence становятся `INCOMPLETE`. Skips фиксируются; skip обязательного изменённого теста означает `INCOMPLETE`, а skip постороннего upstream integration test не подменяет доказательство затронутых тестов. Сырые `go test`/`go build` не используются.

## Текущий профиль

Профиль `owned-go-delta` использует один native driver и шесть точных целей:

| Цель | Владелец T0 | Пути | Driver/evidence |
| --- | --- | --- | --- |
| `server-entrypoints` | `deployment` | `cmd/**` | `build.sh --go`; `bin/ragflow_server`, `bin/ragflow-cli` |
| `agent-runtime` | `agent-runtime` | `internal/agent/component/**`, `internal/handler/**` | два соответствующих пакета |
| `canvas-template-seed` | `mrz-assets` | `internal/dao/**` | `./internal/dao` |
| `model-runtime` | `models-runtime` | `internal/entity/models/**` | `./internal/entity/models` |
| `telemetry-provider` | `observability` | `internal/observability/otel/**` | `./internal/observability/otel` |
| `storage-regression` | `quality-governance` | `internal/storage/**` | `./internal/storage` |

Для `cmd/**` используется отдельное штатное действие `bash build.sh --go`: каталог содержит самостоятельные `main`-файлы и не является обычным `./cmd/...` test package. Для остальных целей команда формируется из неизменяемого префикса `bash build.sh --test`, аргументов `-json -p 4 -timeout 5m` и package targets. Политика требует Linux, `bash`, `go`, один из поддерживаемых `build.sh` C compilers (`gcc`/`clang`), а также `office_oxide`, `pdfium-static` и `pdf_oxide`, которые настраивает `build.sh`.

## Границы доказательства

Планировщик анализирует все `.go` среди локальных дельт T0, включая core changes, extensions и удалённые пути. Непокрытый путь, несовпадение владельца или пересекающиеся prefixes — нарушение `BUILD-01`. Отсутствующий каталог/driver либо изменение provenance во время анализа — неполное доказательство. Сам `READY` подтверждает только полноту плана, не компиляцию, архитектуру пакета, runtime-поведение или отсутствие dead code.

Текущий Windows host имеет `bash` и Docker, но не имеет `go` и C compiler; поэтому прямой локальный `--execute` остаётся `INCOMPLETE`. Контейнерный Linux/native lane обязан сам переснять T0 и подтвердить тот же candidate SHA и fingerprint; принятый ранее T1 test result не переиспользуется как результат нового снимка.

## Выполненный прогон

План на текущем T0 успешно сопоставил **16 из 16** изменённых Go-файлов шести целям, не обнаружил findings или missing target directories и сформировал две команды:

```text
bash build.sh --go
bash build.sh --test -json -p 4 -timeout 5m ./internal/agent/component ./internal/handler ./internal/dao ./internal/entity/models ./internal/observability/otel ./internal/storage
```

Статус host-плана: **READY / NOT_EVALUATED**. Прямой Windows-запуск с `--execute`: **INCOMPLETE**, причины `platform:linux`, `tool:go`, `tool:any-of:clang|gcc`; это классификация среды, а не регресс исходников.

Linux/native выполнение проведено для кандидата `f019c812d` и его T0 fingerprint `dfc9949b…` в одноразовом окружении через image digest `sha256:5ef754a957a48530fdb0108239697e7136c34a3f8bdbd9cc42de66540d539470` и изолированный MinIO. Первый прогон сохранил инфраструктурный разрыв: в чистой копии отсутствовал игнорируемый C++ archive. Повтор использовал только архив, ранее зафиксированный T1 и повторно сверенный по SHA-256 `55a9726facc7eaf7f335086fdeda75de0e4c00d9b8ad073a37ea55bc09496bbc` (2 147 922 bytes); результаты тестов из T1 не переиспользовались. Итоговый отчёт `output/quality/go-build-profiles-linux-executed.json` содержит хэши двух вновь собранных бинарников и структурированные package/test evidence; все 93 функции `Test*` из изменённых test files имеют `pass`, посторонние upstream skips лишь учтены. Текущий зафиксированный snapshot повторно спланирован как `READY / NOT_EVALUATED`, но новый Linux `--execute` для его fingerprint не выполнялся и прежний `PASS` ему не приписывается.

Тринадцать тестов планировщика, включая negative fixtures для unmapped path, wrong owner, prefix overlap, bypass/narrowing, entrypoint build action, artifacts, skip и missing structured evidence: **13 passed**. Актуальные локальные отчёты: `output/quality/go-build-profiles.json`, `output/quality/go-build-profiles-executed.json` и `output/quality/go-build-profiles-linux-executed.json`.
