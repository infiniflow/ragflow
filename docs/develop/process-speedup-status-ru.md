# Ускорение review / build / tests / delivery: статус и остаточный DoD

Дата: 2026-09-06. Рабочий HEAD: `6a50434a41c96ec8b6d0d7aad3ffe489046e5e0d`.
Это отчёт об исполнении восьми задач, **не полный release PASS**.
По отдельной команде пользователя выполнен локальный source-overlay deploy
`v1.15.0` в `ragflow-local`; удалённый offline deploy и Actions publication не выполнялись.
Исторические разделы ниже описывают границы соответствующих предыдущих проверок.

## Локальный деплой v1.15.0

Версия проекта/uv.lock: 1.14.1 → 1.15.0 (minor: новые verify/candidate/packaging
возможности). Образ `infiniflow/ragflow:v0.26.4` сохранён: это версия базового runtime,
а не нового исходного инкремента. Deploy использует существующие source/dist mounts.
Четыре Compose слоя из labels действующего контейнера сохранены: основной,
local, linux.local и observability; env `.env` + `.env.local`.

Проверки перед переключением: 97 targeted tests PASS, uv lock --check --offline,
Ruff check/format, Actionlint и эквивалентные проверки hook-файлов. Смешанные EOL
исправлены штатным formatter только в затронутых файлах; Prettier упорядочил
package.json без изменения зависимостей. Предыдущие full Python/Go/frontend
результаты приведены ниже; пропуски не объявлены PASS.

Frontend собран `npm --prefix web run build -- --outDir ../output/local-deploy-v1.15.0/dist`
с NODE_OPTIONS=--max-old-space-size=8192, VITE_BUILD_SOURCEMAP=false,
VITE_MINIFY=esbuild: PASS, 2m03s. Старый dist сохранён, новый переключён только
после успешной сборки. PostgreSQL pg_dump -Fc сохранён в ignored локальном output;
dump не является доказательством проверенного restore.

Deploy из docker/: `docker compose -p ragflow-local --env-file .env --env-file .env.local
-f docker-compose.yml -f docker-compose.local.yml -f docker-compose.linux.local.yml
-f docker-compose.observability.yml up -d --force-recreate --no-deps --no-build
--pull never ragflow-cpu` — exit 0. Только приложение пересоздано; зависимости,
shared volumes и удалённая система не изменялись.

После запуска: healthy, HTTP healthz содержит status/db/doc_engine/redis/storage=ok;
страница HTTP 200. SHA-256 локального, mounted и HTTP-served index.html совпал:
`294038df37fc428a4857b52d2d01fe1dba8338d2fa46bf66fc77a5928cb8e29c`.
Логи, checks.xml, backup и предыдущий dist: `output/local-deploy-v1.15.0/`.
Этот локальный smoke не заменяет remote offline/recovery и реальный Actions CI.
Commit и tag пока не созданы: во время подготовки фиксации в общем дереве
появились четыре сторонних test edits (access groups, audit и external credentials).
Два новых пути ещё не классифицированы module-map, поэтому финальный T0 capture
остановился. Эти файлы не включены в наш reviewed increment; их содержимое сохранено.
Версия 1.15.0 в pyproject/uv.lock пока является рабочей версией, не созданным tag.

## Задачи, границы и приёмка

| № / цель | Реализовано и проверено локально | Что ещё требуется для полного DoD |
| --- | --- | --- |
| 1. Измерить критический путь | Cold/warm frontend, hooks, native, COPY, source archive, isolated startup; baseline с командами/ресурсами | Полный образ, offline target и действительный downtime; startup/restart command-to-health не равен downtime |
| 2. Убрать повторную подготовку | Явная установка до hooks, check-only CI, корректный Jest transform cache, единый verified native cache, `.git` вне Docker context, fake retry clock | Чистая production-profile сборка и реальный CI; не менять менеджеры и пороги покрытия |
| 3. Неизменяемый кандидат | Exact source identity, проверка каждого файла, явные untracked, frontend build из отдельной копии, receipt артефактов, version/source conflict checks | Commit/tag publication binding и единая проверенная release attestation; текущие unsigned receipts подтверждают bytes, не тесты/доверие |
| 4. Единый запуск проверок | `verify.py plan/run/report`, quick/candidate/full, консервативный owner selection, deadlines, process-tree cleanup, strict JUnit/Jest | Shadow parity на реальном CI; structured Go report и только затем обязательный gate; результат пока всегда release INCOMPLETE |
| 5. Разделить test lanes | Cloud keys больше не ломают import/collection; cloud prerequisite error перед сетью; ограниченный reviewed local profile | Полная классификация live/provider suites, подготовленные disposable сервисы, live matrix и provider credentials; shared data не использовать |
| 6. Убрать CI дублирование | Один producer frontend artifact и три browser consumers; exact manifest/receipt checks, независимый inventory 51 сценария и always final gate | Реальные Actions runs, parity обоих engines/SDK/REST/Web/Admin/CLI; общий Go/image producer, затем удаление старых workflow и отдельное согласование required checks |
| 7. Повторно использовать упаковку | Canonical source archive, verified frontend, Docker image bundle cache, corruption/retag rejection; offline dist/ envelope сохранён | Полный набор 15 реальных images, настоящий offline install, cold/resume timing и отказ на целевой машине; ASR/outer archive ещё не имеют полного build-reuse контракта |
| 8. Сократить опасное окно upgrade | Pull/build до stop, immutable old image ID, target lock, recovery marker до new up, same-version identity checks; pre-boundary start только существующего контейнера | Schema compatibility matrix, actual backup/restore и failure injection на disposable target, capacity/permission checks, runtime identity proof и измеренный downtime |

Ни один незакрытый пункт не замещается пропуском теста или ослаблением gate.
Старые `tests.yml`/`sep-tests.yml` не удалены до доказанной parity. Release workflow
теперь вызывает их и browser-regression на том же commit до публикации; реальное
исполнение этой связи на Actions ещё не подтверждено. Не использовать наличие
workflow или новый shadow-инструментарий как разрешение на выпуск.

## Порядок использования

### Исправления release-блокеров, 2026-09-06

- `release.yml` вызывает существующие `tests.yml`, `sep-tests.yml` и
  `browser-regression.yml` через `workflow_call` без копирования тестовых команд.
  Release job зависит от всех трёх и CLI build, CLI assets — от release job.
  Отсутствие runners, failure, cancellation или skipped зависимости не разрешают
  публикацию через default success-condition. Это не проверка полноты каждого
  внутреннего pytest case и не замена offline upgrade/recovery acceptance.
- Удалено передвижение nightly из prepare. Теперь оно выполняется только после
  gates и успешной сборки Docker image. Все release checkouts закреплены на
  `github.sha`. Существующее передвижение mutable tag сохранено как контракт
  scheduled release; версионные теги workflow не перемещает.
- Общая release concurrency-group не допускает одновременной публикации разных
  refs; текущая публикация не отменяется новым run автоматически. Called workflows
  имеют разные группы, отдельно от самостоятельных проверок ветки.
- Nightly/prerelease не обновляет Docker latest; registry token передаётся через
  env/stdin, не вставляется в shell-код. Публикация Git/Docker/PyPI остаётся
  неатомарной: сетевой сбой после успешных gates может оставить частичный выпуск.
  Эта правка не обещает rollback опубликованных артефактов.
- Три реальных PostgreSQL transaction/race теста прошли на отдельном временном
  postgres:16-alpine с loopback ephemeral port и tmpfs. Контейнер удалён после
  проверки; рабочие БД не использовались. JUnit:
  `output/process-speedup-20260905/release-postgres-20260906.xml`.
- GitHub API на момент проверки вернул 0 self-hosted runners и 0 Actions runs.
  Нужны подготовленные runners `ragflow-test`/`ragflow-release`, secrets и выбранный
  deployment target. Старые 6 Go failures относятся к снимку до `5100b7e3b`;
  актуальный повтор описан ниже, эти failures больше не являются текущим блокером.

Целевой повтор: **57 PASS, 0 skipped, 21,85 с** (release/browser/build identity/hooks),
`output/release-gate-fix-20260906/workflow-tests.xml`. Ruff check и format --check
нового теста прошли. Actionlint 1.7.12 прошёл для всех четырёх workflow; существующая точная метка
`ragflow-release` добавлена в его конфигурацию (не wildcard-ignore и не утверждение
наличия runner). SHA-256 скачанного Windows archive сверена с GitHub release asset.
Shellcheck/Pyflakes в этом запуске actionlint не выполняются; shell-поведение
публикации проверяется отдельными изолированными тестами без registry/Git writes.
Основание same-commit вызова и разделения concurrency —
[GitHub reusable workflows](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)
и [ограничения concurrency](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations).
Происхождение release/tests/sep-tests — core_change относительно принятой базы;
browser workflow, actionlint config и новый contract test — extensions. Правила:
UPG-02, BUILD-01, TEST-01, POL-02. Старый early-tag путь удалён, новых runtime
или бизнес-зависимостей нет. Commit/tag/push и deploy в этой правке не выполнялись.

### Актуализация Go и recovery, 2026-09-06

Старый Go-отчёт от 2026-09-05 предшествовал commit
`5100b7e3b2720f968b071b28d336e4dcae2e940d`, уже исправившему шесть несоответствий
в handlers и тестах. Повторно менять Go-код или ослаблять assertions не потребовалось.
На текущем HEAD `6a50434a4` весь `internal/handler` прошёл: **330 PASS**.

Полный повтор через `bash build.sh --test -count=1 -json -p 4 -timeout 10m ./...`:
**4973 PASS, 0 FAIL, 40 SKIP** верхнеуровневых тестов; отдельно **1395 PASS,
4 SKIP** подтестов. 84 пакета прошли, 21 пакет без test files; process exit 0.
Лог: `output/release-readiness-20260906/full-go.jsonl`; stderr пуст.
Это успешное исполнение доступного Go-набора, **не полный release PASS**:
существующие graph/checkpoint gaps, внешние provider lanes и пропущенный DNS
подтест в изолированной сети остаются явно непроверенными.

Окружение: сохранённый `ragflow-go-regression:go1.26.4-native-20260905`, native cache
`ragflow-go-qa-cache`, source и resource mounts read-only; отдельная internal
Docker network без внешнего доступа, временный MinIO с tmpfs и синтетическими
QA credentials. Shared runtime/БД не использовались. Перед cleanup в MinIO
осталась только `.minio.sys`; контейнеры и сеть удалены, reusable toolchain/cache
сохранены. Новый прогон не использовал test-result cache (`-count=1`).

В `deployment/linux-pg/upgrade.sh` исправлен partial-stop recovery: флаг
`APP_STOPPED=1` выставляется перед `compose stop`, поскольку неуспешная команда
может уже остановить приложение. До data-change boundary ERR handler пробует
запустить существующее старое приложение и сохраняет исходный код ошибки.
После возможной миграции автоматический rollback остаётся запрещён.

Новый failure-injection тест до правки: **1 FAIL / 8 PASS**, exit17 без start.
После правки recovery/identity/delivery/offline-source: **40 PASS, 0 SKIP,
22,08 с** с подготовленным jq. Добавлены partial-stop и missing-app cases;
сохранены no-start при неудачном возврате дерева и после data-change boundary.
Ruff check/format, Bash syntax и diff-check прошли. Это shell control-flow
fixtures, не доказательство полноценного offline install/upgrade/restore.
Три затронутых deployment/test файла — extensions по accepted upstream tree;
правила DATA-03, TEST-01, SIMP-02. Новых runtime paths, API или зависимостей нет.
Для завершения release DoD ещё нужны реальные CI runners/secrets, выбранный
target и проверка обновления/восстановления предыдущей поставки на disposable
системе. Commit/tag/push/deploy в этой правке не выполнялись.

### Локальные команды

1. Для локальных hooks выполнить `python tools/hooks/prepare_web.py` после изменения
   lockfile; hooks сами зависимости не ставят. Native: `python ragflow_deps/prepare_native.py --download`,
   затем `--check`. Повреждённый кеш не перезаписывается автоматически.
2. `python tools/quality/verify.py plan --mode quick --output output/verify-plan-NEW`;
   `run --mode full --output output/verify-run-NEW` исполняет проверки. Каталог должен
   быть новым. Неподготовленный live/Go lane возвращает INCOMPLETE; `--isolated-stack`
   допустим только для действительно выделенных тестовых ресурсов.
   `execution` перечисляет реальные запуски, `covered_by` — вложенные проверки:
   при совместном выборе plain process-contracts входит в полный Python unit
   без повторного исполнения. Статус вложенного набора консервативно наследует
   статус общего (включая FAIL/INCOMPLETE/CANCELLED); прошлые результаты не используются.
3. `python tools/quality/candidate.py --help` — snapshot/verify/record-artifact/verify-artifact.
   Назначить версию до snapshot. Dirty snapshot требует явного разрешения и точного
   списка untracked; он пригоден для эксперимента, не для публикации.
4. `python -m tools.quality.frontend_artifact --help`: build в отдельном work directory,
   pinned Node/pnpm и production profile; verify перед передачей упаковщику.
   На Windows допустим явный `--pnpm-cli` к установленному `pnpm.cjs`: пользовательский
   wrapper, зависящий от APPDATA, несовместим с изолированным HOME.
5. Source/offline PowerShell упаковщики поддерживают `CandidateDirectory` и
   `FrontendArtifact`; offline дополнительно `BundleCacheDirectory`. Canonical
   source archive нельзя пересобрать/перезаписать даже с Overwrite: вернуть
   сохранённые bytes или создать новый кандидат. Legacy режим остаётся явно unverified.
6. Upgrade при recovery marker прекращается: автоматического возврата старого
   кода после возможной миграции нет. Сначала оператор проверяет совместимость
   данных/восстановление. Production запуск этим отчётом не авторизован.
   Offline wrapper требует пустой `SOURCE_DIR` для каждого вызова, в том числе
   после `--check`. Непустой каталог не удаляется и не переиспользуется по версии.

## Закрытие пяти замечаний ревью

- Offline source A больше нельзя принять вместо payload B той же версии:
  непустой SOURCE_DIR отклоняется до preflight/load; отдельный исполняемый тест
  проверяет сохранение A, другой — распаковку B в пустой каталог.
- Для всех запусков очищается унаследованный PYTEST_ADDOPTS. Только явно заданные
  параметры текущей политики могут попасть в него. Регрессия с `-k test_keep`
  подтверждает исполнение и отказ скрытого падающего теста.
- Browser receipt и gate сверяют точные nodeids независимого
  `test/playwright/mandatory-browser-cases.json` (51 сценарий), его digest,
  дубли/пропуски/ошибки и счётчики. Ожидаемый список не генерируется из текущего
  runner selection в CI; изменение inventory требует ревью тестового контракта.
- `frontend-foundation` связан с frontend lanes: обычный banner выбирает types
  и Jest; shared services/transport/routes по-прежнему требуют полного плана.
- Совпадающий plain process subset выполняется внутри полного unit запуска один
  раз. Оптимизация отключается при изменении команд, флагов или prerequisites;
  отсутствующее/неуспешное общее доказательство не превращается в отдельный PASS.

Изменены принадлежащие форку quality/deployment owners; принятая upstream-база
не менялась. BUILD-01, TEST-01, POL-02, SIMP-02. Удалены version-only ветка
повторной распаковки и повторный вызов вложенного unit-набора. Общий бизнес-код,
порог coverage и состав обязательных browser-сценариев не сокращались.

Проверка исправлений: 23 verifier/offline теста и 20 browser-gate тестов прошли;
после форматирования повторно прошли оба исполняемых offline теста. Ruff check,
Ruff format --check, Bash syntax и git diff --check прошли. Новый inventory также
принял три ранее сохранённых browser JUnit (по 51 сценарию): это повторная
проверка свидетельств, не новый browser/live прогон. Независимый повторный обзор
verifier не обнаружил новых конкретных дефектов.

Итоговый локальный full shadow run после исправлений:
`output/process-speedup-20260905/review-fixes-full/report.json`.
Python: **2700 PASS, 25 существующих skipped, 146 subtests PASS**, 257,97 с
в pytest / 271,01 с всей команды. Process-contracts включён в этот запуск без
повторного исполнения; его статус консервативно INCOMPLETE вместе с общим набором.
Document coverage: **136 PASS, 79,79%**; requirements: **5 PASS, 1 live SKIP**.
TypeScript PASS; Jest **22 suites / 161 PASS** с coverage (56,23 с всей команды).
Неуспешных тестов в этом прогоне нет. Postgres races и полный Go не запускались:
нет подтверждённого disposable-stack/DSN и требуемого Linux toolchain.
Итог остаётся **release INCOMPLETE**: skips и внешние CI/live/release evidence
не заменены локальными результатами. Коммиты, публикация и деплой не выполнялись.

Frontend receipt относится к исходному flat tar; offline package переупаковывает
его в прежний `dist/` layout, защищённый SHA256SUMS. Receipt не выдаётся за прямой
проверочный документ переупакованного tar. Docker bundle receipt связывает tags,
image IDs/platform и bytes архива; он не доказывает сборку этих images из кандидата.

## Измерения и свидетельства

Все локальные журналы лежат в ignored `output/process-speedup-20260905`, baseline —
`output/process-baseline-20260905/REPORT.md`. Общий хост делает сравнения ориентировочными.

Финальные process checks воспроизводятся из корня:

```powershell
$env:JQ_TEST_BINARY='S:/ragflow/output/process-speedup-20260905/identity-tools/jq-linux-amd64'
$env:JQ_TEST_IMAGE='infiniflow/ragflow:v0.26.4'
.venv/Scripts/python.exe -m pytest test/unit_test/tools test/unit_test/deployment test/unit_test/test_live_model_profile.py test/unit_test/data_source/test_rest_api_connector.py -q --junitxml=output/process-speedup-20260905/replay-process-contracts.xml
```

Для jq используется проверенный SHA-256 official 1.7.1 binary и networkless
disposable container; это не запуск приложения. Без native jq или указанных
prerequisites identity tests дают SKIP/INCOMPLETE, не PASS. Имена replay output
нужно выбирать новыми, сохраняя историю предыдущих результатов.

- Warm focused Jest: 9,069 / 8,524 с против медианы 10,620 с без cache (около 15–20%).
- Candidate verification, те же 4724 файла: до 9,544/16,453/15,438 с, после общего
  линейного обхода 2,703/2,283/1,694 с. Проверяются точный file set, hashes и links;
  результат тестов не кешируется. Это не защита от враждебной конкурентной FS mutation.
- Docker `.git` занимал 7,86 GB; он исключён. Последний COPY probe: 26 входов,
  19,155 с. 264,17 kB — incremental transfer, не размер чистого полного контекста.
- Реальный Docker bundle alpine: первое сохранение 2,307 с, verified reuse 0,493 с,
  те же 3 895 808 bytes. Не экстраполировать на 15 production images.
- Изолированный frontend: snapshot 46,148 с + build/verification 246,302 с;
  внутри install 63,6 с, Vite 94 с. Candidate source ID
  `3b987d2f0ea28d2b79cf001fb2a5c4c2a9acf470d0754dcc076eb60f6291b185`.
  Этот snapshot предшествует финальным tooling/test правкам и не является
  same-source release proof для окончательного рабочего дерева.
- На одном построенном SPA: Chromium 51 PASS / 73,87 с, WebKit 51 PASS / 165,39 с.
  Firefox сначала 50 PASS / 1 FAIL: request event опережал route handler теста.
  Исправлено ожидание завершённого PUT response вместо request без sleep/retry;
  полный повтор Firefox 51 PASS / 111,06 с. Test file повторного запуска изменён,
  UI artifact тот же; это mocked UI, не live backend regression.
- Shadow run 1: Python 2612 PASS + 25 существующих skipped / 222,66 с;
  document coverage 136 PASS, 79,79% при пороге 79%; Jest 22 suites / 161 PASS,
  TypeScript PASS. Deterministic evals 5 PASS + 1 optional live SKIP. Skips и
  отсутствующие live/Go prerequisites означают INCOMPLETE. Первый process-contracts
  run поймал конкурентное изменение fixture; окончательный результат хранится
  отдельно в `final-process-contracts.xml`, история не перезаписана.
- Финальный process/deployment/profile/REST набор: **292 PASS, 0 skipped,
  224,68 с** (`final-process-contracts.xml`). Проверки Ruff, PowerShell parser,
  Bash syntax и `git diff --check` прошли. T0 capture: 625 records,
  337 core changes / 288 extensions, unclassified = 0.
- Финальный Python unit повтор: **2680 PASS, 35 skipped, 122 subtests PASS,
  256,96 с** (`final-python-unit.xml`). 25 skips — существующий
  `TestRemoveRedundantSpaces`; 10 — `test_upgrade_identity`, поскольку общий
  запуск не получил JQ_TEST_BINARY/JQ_TEST_IMAGE. Эти 10 отдельно прошли в
  целевом наборе 292/292 выше. Сообщение старого `run_tests.py` «All tests passed»
  не отменяет skipped: полный результат остаётся INCOMPLETE, не release PASS.
- Настоящий CGO compile/link через `build.sh --test`: три PDF fixture tests PASS
  в Go 1.26.4 container, network none, read-only source/cache. Это не полный Go.
  Исторический Go regression до `5100b7e3b` имел 6 failures в internal/handler;
  он заменён актуальным повтором выше, а не считается текущим блокером.

## Provenance, review и cleanup

Accepted upstream: `cb93883f3f8c975eecb2fed81210effeb3bdb06f`. Exact paths и hashes
содержатся в обновлённых `tools/quality/module-map.yaml`, `file-inventory.json`
и `core-changes.yaml`; классификация основана на Git tree, не на каталогах.
Dockerfile/workflows/build/hooks/downloaders, testcases configs/conftest и REST
connector test — изменения стандартных входов (core changes). Новые quality
helpers и их тесты — extensions. Deployment и navigation test относятся к
существующим локальным расширениям; чужие изменения этих файлов сохранены.

Применимы BUILD-01, TEST-01, POL-02, UPG-01. Протокол cleanup выполнен вручную
по изменённым владельцам, вызовам и CLI/CI entrypoints: общий native downloader,
общий source walker, удалены install/mutex дубли hooks и реальные sleeps retry tests.
Нет новых ORM/runtime зависимостей доменного кода, mass moves, смены package manager,
удаления поддерживаемых профилей или послаблений coverage. Будущие архитектурные
runners не объявляются существующими. До переключения CI потребуется отдельная
приёмка на доступных runners; на origin в момент проверки runners = 0, runs = [].
