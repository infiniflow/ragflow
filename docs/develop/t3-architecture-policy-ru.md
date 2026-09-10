# T3: selector и aggregate архитектурной политики

Дата: 2026-09-10. Статус: `REPORT_ONLY_BASE_PROTOCOL_READY`. Реализованы локально проверенные selector, base/candidate policy comparison, aggregate и трёхjobовый workflow с единственным стабильным финальным context `architecture-policy`. Protocol v1 ещё отсутствует в comparison base текущего изменения, поэтому первый переход остаётся `candidate-bootstrap` и явно `NOT_ENABLED`. Опубликованный bootstrap создал push-run, но тот не стартовал: GitHub API показал ноль зарегистрированных self-hosted runners. Текущий кандидат переведён на GitHub-hosted Linux с явной установкой toolchain и разделением plan/analysis/final, но ещё не выполнялся в GitHub. Required Workflow ruleset не настроен и не проверен.

## Контракт

`tools/quality/check_architecture_policy.py` ничего не исправляет, не выполняет выбранные анализаторы и не изменяет Git index. Действие `select` строит план по явному base commit и текущему T0 snapshot. Оно учитывает committed, staged, unstaged и nonignored untracked paths; `--no-renames` сохраняет обе стороны rename/delete. Текущий путь получает owner/origin из свежего T0, удалённый или возвращённый к upstream путь — owner из `module-map.yaml` базового commit. Неизвестный путь, недоступный base, неполная provenance или изменение дерева во время capture дают код 2. План фиксирует hashes evaluator, base/candidate policy и candidate checker, exact source bundle каждого lane, ожидаемые producer/config digests отчётов и hashes fixture sources; внешний policy-файл принимается только при полном совпадении с содержимым candidate или comparison base.

Машинная политика `tools/quality/architecture-policy.json` определяет пять lanes:

| Lane | Выход | Значение успешного evidence |
| --- | --- | --- |
| `provenance` | встроенный свежий T0 | `OBSERVED`, не behavioral PASS |
| `python-architecture` | `architecture-python.json` | точный настроенный `PASS` |
| `runtime-graph` | `runtime-graph.json` | `CLASSIFIED`; policy остаётся `REPORT_ONLY` |
| `go-build-plan` | `go-build-plan.json` | `PLANNED`; native build остаётся `NOT_EVALUATED` |
| `policy-fixtures` | identity-bound JSON attestation со встроенным JUnit | authoritative checker выполнил точные trusted `path::nodeid` fixtures без failure/error/skip на том же T0 fingerprint |

Действие `compare` проверяет candidate policy относительно authoritative policy: запрещены удаление существующего lane/rule/required fixture, смена tool/contract, снятие `always` и сужение exact paths/prefixes/suffixes. Candidate plan обязан охватывать каждый lane, выбранный authoritative plan на том же HEAD/T0 fingerprint. При `authority_source=base` отчёт получает `compatibility_status=COMPATIBLE` и доказывает только несужение текущей base policy. При `authority_source=candidate-bootstrap` тот же механизм является исключительно self-check целостности candidate policy/plan и получает отдельный `compatibility_status=BOOTSTRAP_SELF_CHECK`; trusted base coverage в этом режиме не оценена. Расширение разрешено, но новое candidate-only правило не считается обязательным до появления в base.

Действие `materialize` извлекает каждый protected source выбранных lanes из comparison base, а в bootstrap — из совпадающего candidate bundle, перепроверяет digest и создаёт новый отдельный source tree рядом с plan. Analysis запускает Python producers из этого дерева через `run_isolated_python.py` с `python -I`: candidate working directory не участвует в import path, а локальные imports разрешаются только из materialized `tools/quality`. В режиме `authority_source=base` обычный изменённый analyzer, config, fixture, harness dependency или dependency lock закрывает lane как `INCOMPLETE`. Единственный base-governed control source с режимом `BASE_FIXTURE_REVIEW` — `tools/quality/architecture-policy.json`: authoritative plan и materialization используют его base-версию, candidate policy проходит монотонное base/candidate comparison, base fixtures проверяют её контракт, а plan требует ручного review. Candidate checker и workflow остаются `MUST_MATCH`; их обновление требует отдельного аудированного protocol/bootstrap flow. В bootstrap все sources принадлежат candidate self-check и сравнение их bytes с comparison base не выполняется.

Действие `fixtures` само запускает перечисленные policy `path::nodeid` между двумя T0 captures. При `source=base` fixture files извлекаются из comparison base и получают candidate root только через `ARCHITECTURE_CANDIDATE_ROOT`; candidate не может заменить их одноимёнными no-op tests. Pytest запускается с `--noconftest`, отключённой автоматической загрузкой plugins и очищенным от GitHub command-file/secret variables окружением. Прямые test-harness dependencies exact runtime contracts также входят в protected source closure. Attestation фиксирует source hashes, exact node IDs, HEAD, upstream base, T0 fingerprint, selection digest, SHA-256 и base64 исходного JUnit; aggregate повторно разбирает встроенный JUnit и отклоняет несовпадение names/counts/digest.

Действие `aggregate` повторно строит authoritative и candidate selector plans и сравнивает их с обоими сохранёнными файлами. Поэтому ручная смена `selected=true` на `false` не превращает применимую проверку в `NOT_APPLICABLE`, а произвольный синтаксически корректный candidate-plan digest не подменяет результат `compare`. Все передаваемые в aggregate JSON reports, включая fixture attestation, обязаны иметь тот же comparison base, HEAD, upstream base, T0 fingerprint, selection digest и ожидаемые producer/config hashes своего lane. Промежуточные TypeScript/Go observer reports не являются самостоятельным aggregate evidence: их hashes входят в итоговый runtime-graph report. При `authority_source=base` изменение ordinary protected analyzer/config/fixture/dependency source относительно base закрывает выбранный lane как `INCOMPLETE`, даже если отчёт сам объявляет правильное tool name и `PASS`; исключение `BASE_FIXTURE_REVIEW` ограничено candidate policy и не меняет base authority. Bootstrap проверяет только согласованность candidate bundle и не делает такой вывод относительно comparison base. Evidence принимается только из свежего каталога рядом с plan; отсутствующий, stale, malformed или созданный другим producer отчёт даёт `INCOMPLETE`. При одновременном finding и неполноте итоговый код 2 сохраняет приоритет `INCOMPLETE`; чистый finding без неполноты даёт код 1.

Успешный текущий aggregate возвращает `REPORT_ONLY_COMPLETE` с `enforcement_status=NOT_ENABLED`. Совместимость с реальной base policy отображается как `PASS`, а bootstrap self-check — только как `OBSERVED`; подмена пары authority/status даёт `INCOMPLETE`. Статусы `OBSERVED`, `CLASSIFIED` и `PLANNED` намеренно не называются `PASS`.

## Локальный запуск

Использовать новый пустой evidence directory; tool откажется перезаписывать старые результаты. Сначала явно разрешить comparison base текущего PR. В первом переходе candidate `HEAD` уже содержит protocol v1, но `origin/main` ещё не содержит его, поэтому запуск относительно этой базы остаётся bootstrap self-check: он проверяет механику candidate checker/policy, но не сравнивает candidate policy с доверенной base policy и не может считаться gate. Перед запуском убедиться, что локальный `origin/main` соответствует фактической PR base, либо подставить её точный commit SHA.

```powershell
$evidence = "output/quality/t3-architecture-policy-$(Get-Date -Format yyyyMMdd-HHmmss)"
$base = (git rev-parse "origin/main^{commit}").Trim()
uv venv --clear services/asr-online-service/.architecture-venv --python 3.13
uv pip sync --python services/asr-online-service/.architecture-venv/Scripts/python.exe --require-hashes --strict services/asr-online-service/architecture-contract-requirements.txt
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py select --base $base --output "$evidence/selection.json"
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py select --base $base --output "$evidence/candidate-policy-selection.json"
$selectionSha256 = (Get-Content -Raw "$evidence/selection.json" | ConvertFrom-Json).selection_sha256
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py compare --base-plan "$evidence/selection.json" --candidate-plan "$evidence/candidate-policy-selection.json" --candidate-policy tools/quality/architecture-policy.json --output "$evidence/policy-compatibility.json"
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py materialize --plan "$evidence/selection.json" --output-directory "$evidence/protected-sources"

$trustedSources = (Resolve-Path "$evidence/protected-sources").Path
$isolatedRunner = Join-Path $trustedSources "tools/quality/run_isolated_python.py"
.\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py fixtures --plan "$evidence/selection.json" --python .venv/Scripts/python.exe --junit-output "$evidence/policy-fixtures.xml" --output "$evidence/policy-fixtures.json"
& .\.venv\Scripts\python.exe -I -B $isolatedRunner $trustedSources tools/quality/inspect_typescript.py --root (Get-Location) --output "$evidence/typescript-analysis.json"
& .\.venv\Scripts\python.exe -I -B $isolatedRunner $trustedSources tools/quality/inspect_go.py --root (Get-Location) --output "$evidence/go-analysis.json"
& .\.venv\Scripts\python.exe -I -B $isolatedRunner $trustedSources tools/quality/check_runtime_graph_policy.py --root (Get-Location) --typescript-report "$evidence/typescript-analysis.json" --go-report "$evidence/go-analysis.json" --selection-sha256 $selectionSha256 --output "$evidence/runtime-graph.json"
& .\.venv\Scripts\python.exe -I -B $isolatedRunner $trustedSources tools/quality/check_go_build_profiles.py --root (Get-Location) --selection-sha256 $selectionSha256 --output "$evidence/go-build-plan.json"
& .\.venv\Scripts\python.exe -I -B $isolatedRunner $trustedSources tools/quality/check_architecture.py --root (Get-Location) --base-ref $base --selection-sha256 $selectionSha256 --output "$evidence/architecture-python.json"

$reportFiles = @{
    "python-architecture" = "$evidence/architecture-python.json"
    "runtime-graph" = "$evidence/runtime-graph.json"
    "go-build-plan" = "$evidence/go-build-plan.json"
    "policy-fixtures" = "$evidence/policy-fixtures.json"
}
$aggregateReports = @()
$requiredReports = (Get-Content -Raw "$evidence/selection.json" | ConvertFrom-Json).required_reports
foreach ($lane in $requiredReports) {
    if (-not (Test-Path -LiteralPath $reportFiles[$lane])) { throw "Missing selected report: $lane" }
    $aggregateReports += "--report"
    $aggregateReports += "$lane=$($reportFiles[$lane])"
}
& .\.venv\Scripts\python.exe -B tools/quality/check_architecture_policy.py aggregate --plan "$evidence/selection.json" --candidate-plan "$evidence/candidate-policy-selection.json" --compatibility "$evidence/policy-compatibility.json" @aggregateReports --output "$evidence/aggregate.json"
```

Команда регенерации ASR lock при осознанном изменении direct pins приведена в [инструкции T2](t2-python-analysis-ru.md). Workflow не обновляет lock автоматически: он проверяет tracked input/lock обязательной fixture и устанавливает только зафиксированный closure с `--require-hashes --strict`.

В bootstrap-блоке обе selection построены candidate checker/policy: `policy-compatibility.json` обязателен для fail-closed aggregate, но он обязан сообщать `authority_source=candidate-bootstrap` и `compatibility_status=BOOTSTRAP_SELF_CHECK`. Aggregate принимает эту точную пару только как `OBSERVED`; `COMPATIBLE`/`PASS` зарезервированы для сравнения с policy из реальной protocol-v1 base.

После появления protocol v1 в comparison base локальная проверка base/candidate обязана использовать именно base checker и policy. Для `HEAD` как доверенной базы точные bytes извлекаются так:

```powershell
$trustedEvidence = "output/quality/t3-architecture-policy-trusted-$(Get-Date -Format yyyyMMdd-HHmmss)"
$base = (git rev-parse "HEAD^{commit}").Trim()
$trustedRoot = Join-Path $trustedEvidence "trusted-base"
$trustedQuality = Join-Path $trustedRoot "tools/quality"
New-Item -ItemType Directory -Force -Path $trustedQuality | Out-Null
git cat-file blob "${base}:tools/quality/check_architecture_policy.py" > "$trustedQuality/check_architecture_policy.py"
git cat-file blob "${base}:tools/quality/capture_inventory.py" > "$trustedQuality/capture_inventory.py"
git cat-file blob "${base}:tools/quality/architecture-policy.json" > "$trustedQuality/architecture-policy.json"

$baseRunner = Join-Path $trustedQuality "check_architecture_policy.py"
$basePolicy = Join-Path $trustedQuality "architecture-policy.json"
& .\.venv\Scripts\python.exe -B $baseRunner --root (Get-Location) --policy $basePolicy select --base $base --output "$trustedEvidence/base-selection.json"
& .\.venv\Scripts\python.exe -B $baseRunner --root (Get-Location) --policy tools/quality/architecture-policy.json select --base $base --output "$trustedEvidence/candidate-policy-selection.json"
& .\.venv\Scripts\python.exe -B $baseRunner --root (Get-Location) --policy $basePolicy compare --base-plan "$trustedEvidence/base-selection.json" --candidate-plan "$trustedEvidence/candidate-policy-selection.json" --candidate-policy tools/quality/architecture-policy.json --output "$trustedEvidence/policy-compatibility.json"
```

Запускать анализаторы нужно по `required_reports` из `selection.json`. Текущий переход меняет policy, workflow и producer-контракты, поэтому выбирает все четыре report lanes: `python-architecture`, `runtime-graph`, `go-build-plan` и `policy-fixtures`. Для последующих узких изменений полный список команд выше остаётся справочником: lane с `NOT_APPLICABLE` запускать и передавать в aggregate нельзя. Aggregate отклоняет лишний отчёт так же, как отсутствующий выбранный. `inspect_typescript.py` сейчас ожидаемо возвращает code 2 из-за одного уже классифицированного computed-import gap. Это не разрешение игнорировать команду: workflow допускает этот промежуточный code только при наличии отчёта, после чего `check_runtime_graph_policy.py` обязан вернуть полную exact-классификацию. Любая другая неполнота или отсутствие файла останавливает aggregate.

## Workflow

`.github/workflows/architecture.yml` запускается для `opened`, `synchronize`, `reopened`, `ready_for_review` и `edited` событий `pull_request`, при push в `main` и вручную, без метки `ci` и без path filters. `edited` закрывает смену base-ветки PR; лишний повтор при правке title/body безопасно сворачивается concurrency group. В workflow три job: `architecture-policy-plan` без установки candidate dependencies и запуска candidate analyzers/tests; `architecture-policy-analysis` для выбранных environments и отчётов; финальный `architecture-policy` на новом runner. Только финальный job имеет стабильный display name `architecture-policy`.

Plan job извлекает `check_architecture_policy.py`, `capture_inventory.py` и `architecture-policy.json` из comparison base, строит authoritative base plan, независимо оценивает candidate policy и выпускает обязательный `policy-compatibility.json`. Только изменение candidate policy получает `BASE_FIXTURE_REVIEW`: base evaluator и source bundle остаются неизменными, а candidate policy обязана сохранить coverage. Изменённые checker или workflow закрывают затронутые lanes как обычные protected sources. Analysis job получает plan artifact и сначала отклоняет любой выбранный, но неисполняемый из-за source integrity lane; поэтому оставшийся runnable lane не запускает candidate lifecycle после неполного соседнего lane. Затем job материализует проверенные protected producer sources, запускает их через isolated safe-path runner, передаёт exact selection digest каждому итоговому producer, выполняет fixtures до установки TypeScript parser и всех producer steps и публикует отдельный report artifact. Все producer steps после fixtures имеют `success()` gate. Все три artifact upload явно включают hidden files, поэтому защищённые `.github/workflows/architecture.yml` и `web/.npmrc` не исчезают из evidence bundle. Дочерние Python/pytest и Node subprocesses, запускаемые protected producers, не наследуют GitHub command files, evidence paths, tokens, `NODE_OPTIONS`/`NODE_PATH` или ambient pytest/Python injection variables; conftest discovery отключён. Отдельно запускаемый package manager этой Python-функцией очистки не охвачен. Финальный job не выполняет `uv sync`, pnpm, pytest или candidate analyzers: он заново checkout-ит candidate tree, повторно извлекает authority из base, проверяет неизменность base/source и selection, повторяет candidate-policy comparison, требует точного совпадения compatibility с plan artifact, загружает отчёты и передаёт compatibility в aggregate. Отсутствующий, stale или не соответствующий plan compatibility закрывает final fail-closed. Поэтому candidate lifecycle из analysis job не может изменить checker/policy, реально исполняемые финальным aggregate.

Если comparison base содержит `TRUSTED_BASE_PROTOCOL = 1`, authority в plan и final берётся из base, а проверенная совместимость может получить `PASS`. Если protocol отсутствует или bundle неполон, оба job явно используют `candidate-bootstrap`; compatibility остаётся `OBSERVED`, весь результат сохраняет `enforcement_status=NOT_ENABLED` и не заявляет non-bypassable gate.

Workflow готовит только выбранные environments, выполняет reports без `--write`/`--fix`/`git add` и сохраняет раздельные plan, analysis и final artifacts. Финальный job имеет `if: always()` и остаётся видимым required context даже при failure/skip предыдущего job; он отдельно требует `result == success` и для plan, и для analysis, поэтому поздний setup/post-action failure нельзя скрыть уже созданным report artifact. Отсутствие plan или выбранного report также закрывает проверку ошибкой.

Все job используют `ubuntu-latest`; plan/final имеют 15-минутный, analysis — 45-минутный timeout. Node 22, pnpm 10 и dependency synchronization существуют только в analysis, Python 3.13 для policy runner устанавливается через `astral-sh/setup-uv`. Корневое Python-окружение синхронизируется frozen-командой `uv sync --no-install-project`; выделенное ASR contract-окружение — из tracked lock командой `uv pip sync --require-hashes --strict`. TypeScript parser устанавливается не напрямую из candidate `web`: authoritative `package.json`, `pnpm-lock.yaml` и `.npmrc` копируются из materialized bundle в новый каталог `RUNNER_TEMP`, где pnpm запускается с `--ignore-scripts --ignore-pnpmfile --ignore-workspace`. В реальном `authority_source=base` эти inputs закреплены base; изменённый manifest/lock/config делает каждый Node-consuming lane неисполняемым и analysis не запускает candidate package-manager inputs. В `candidate-bootstrap` materialized inputs принадлежат candidate self-check и не являются доверенной supply-chain границей. Свежий checkout обязан не содержать `web/node_modules`; в него через dereference копируется только пакет `typescript`, после чего проверяется его реальный `lib/typescript.js`. В analysis artifact временный полный `node_modules` не попадает. Fixture-only lane не поднимает Node. Это не является сетевой или OS sandbox для самого package manager. Такой запуск устраняет бесконечную очередь при отсутствии `self-hosted/ragflow-test`, но успешность hosted lane должна быть подтверждена реальным run после публикации текущего кандидата.

Workflow использует read-only permissions и не получает application secrets. Изолированный import path, trusted package-manager inputs и очищенное child environment не являются OS sandbox: exact runtime probes всё ещё исполняют candidate application code с правами пользователя runner. `BASE_FIXTURE_REVIEW` и запись в `manual_review_required` являются только evidence для ревью, а не технически обеспеченным approval. Candidate checker/workflow поэтому не входят в исключение и должны совпадать с base; их изменение, изменение protected fixtures либо сужение устаревших controls требует отдельного аудированного обновления протокола/bootstrap. Текущий report-only evidence не объявляется non-bypassable до отдельной process/filesystem isolation, проверенного Required Workflow и обязательного control-owner approval. Наличие YAML не доказывает, что job выполнялся или обязателен для merge.

## Что осталось до приёмки T3

1. Зафиксировать protocol v1 в base и подтвердить следующим реальным PR четыре base-mode сценария: безвредное монотонное расширение policy получает `BASE_FIXTURE_REVIEW`, сохраняет base materialization и `COMPATIBLE`; сужение policy отклоняется; изменение checker/workflow или другого ordinary protected source даёт `INCOMPLETE`; провал trusted fixture не может дать успешный aggregate. Evidence должен сообщать `evaluator_source=base` и `evaluated_policy_source=base`.
2. Повторить в GitHub три уже проходящие локальные изолированные PR-пробы: разрешённое изменение проходит; запрещённая зависимость блокируется; unclassified core change или отсутствующий analyzer завершается ошибкой.
3. Выполнять candidate runtime probes в отдельной OS-level process/filesystem boundary без доступа к runner command files, credentials и supervisor-owned evidence; отрицательная fixture должна попытаться изменить эти поверхности и получить отказ.
4. Закрепить workflow из доверенной default branch как Required Workflow в repository ruleset (либо использовать эквивалентный внешний неизменяемый check) и проверить, что удаление/переименование/skip job в candidate не разрешает merge. Обычный required status context в branch protection проверяет имя результата, но сам по себе не фиксирует содержимое PR workflow.
5. Не создавать пустой `baseline.json`: текущий T2 не дал полного architecture/dead-code/build verdict. Существующие exact classifications остаются в своих policy files до появления доказанного более широкого анализа.
