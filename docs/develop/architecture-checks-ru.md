# Правила архитектуры: проверки и доказательства

Дата: 2026-09-10. Статус: нормативная спецификация для ревью и будущего автоматического контроля. Реестры происхождения T0 реализованы в [снимке инвентаризации](t0-provenance-report-ru.md); [Python-инструменты T2](t2-python-analysis-ru.md) включают наблюдательный import-граф, report-only ARC-01/02, два scoped ARC-03 cycle checks и DEAD-01 scan/plan для чистого домена и ASR. Девятнадцать runtime probes/contracts включают четыре изолированные import/HTTP process-пробы и пятнадцать exact pytest contracts: две agent registries, production adapters документов, frontend routes/navigation/bootstrap/provider/router/service factories, полный FastAPI route inventory и lifespan/worker cleanup ASR, точные inventories всех 15 локально изменённых/добавленных REST API loaders (248 routes), Python API и Flask admin bootstraps, task-executor registries, CLI handoff, execution modes, unacked replay, special fan-out hydration, operation-log, heartbeat report/stale-worker cleanup, poll/error/cancellation semantics, cleanup/ack, limiter release и shutdown cancellation, Business Documents worker lifecycle, sync worker process root и dispatch registry 37 connector sources. [TypeScript observer](t2-typescript-analysis-ru.md) отдельно строит широкий `web/src` import graph, reverse consumers, browser-entrypoint reachability, eager cycles и dynamic-loading gaps; его `OBSERVED` не означает PASS, а текущий computed import оставляет отчёт `INCOMPLETE`. [Go observer](t2-go-analysis-ru.md) строит полный source/package inventory, раздельные CGO/no-CGO graphs, entrypoint reachability, полную legacy build-constraint semantics и directive evidence без compiler; его `OBSERVED` также не является build/policy/dead-code PASS. [Runtime-graph classifier](t2-runtime-graph-classification-ru.md) на одном T0 snapshot классифицирует все текущие TypeScript/Go cycles, owned unreachable paths, production/auxiliary roots, dynamic gaps и Go directives; upstream cycle edge сравнивается по source/target/specifier и AST-семантике `kind/phase/type_only`, а отсутствие parser закрывает проверку как `INCOMPLETE`. Инструмент отклоняет новые неизвестные сигналы и оставляет policy `REPORT_ONLY`, dead code `REVIEW_REQUIRED`. [Go build-profile planner](t2-go-build-profiles-ru.md) проверяет полноту owner/target mapping всех изменённых `.go`, собирает entrypoints через `build.sh --go` и запускает package tests через `build.sh --test`; Linux/native lane выполнен для предыдущего согласованного snapshot, а текущий зафиксированный snapshot имеет только свежий `READY / NOT_EVALUATED` host-план. T2 принят как report-only инструментарий с классифицированными пределами, но не как архитектурный verdict или CI gate. Полные policy/dead-code verdicts, baseline новых нарушений и CI gates относятся к T3 и последующим этапам.

Связанные документы: [архитектура](architecture-and-code-quality-ru.md), [план перехода и инструкции](architecture-transition-ru.md), [существующие regression lanes](../../test/REGRESSION.md).

[Цикл очистки и упрощения](continuous-code-simplification-ru.md) определяет доказательства dead code, режимы автоприменения, сигналы сложности, проверку алгоритмов до/после и защиту текущих изменений рабочего дерева.

## 1. Каталог правил

Rule ID стабилен; изменение формулировки не должно молча ослаблять смысл. «Блокирует» означает требование ревью сейчас и условие машинного gate после его внедрения. Часть критериев остаётся предметным ревью: анализатор не доказывает отсутствие лишних абстракций или корректность бизнес-смысла.

| ID | Правило и область | Проверка и доказательство | Условие отказа |
| --- | --- | --- | --- |
| UPG-01 | База upstream известна и закреплена | Полный SHA, источник, история принятия; object существует | База не установлена, подставлен HEAD форка, ref плавает |
| UPG-02 | Собственное изменение стандартного файла учтено | Сравнение деревьев с принятой базой, core-changes, ревью причины и теста | Неучтённый add/modify/delete/rename или бессодержательное исключение |
| UPG-03 | Структура ядра сохраняется | Ревью переносов/форматирования, diffstat и назначения | Массовая перестройка ради локальной схемы без требования продукта |
| ARC-01 | Чистые собственные слои не зависят от I/O и bootstrap | Import graph плюс изолированный import test | Прямая/косвенная зависимость domain от ORM/HTTP/client или запуска приложения |
| ARC-02 | Обращения расширения к ядру локализованы | Граф, разрешённые target symbols и контрактные тесты адаптера | Обход адаптера, неизвестная точка обратного подключения ядра к расширению |
| ARC-03 | Публичная поверхность минимальна, собственных циклов нет | Import graph, анализ exports и ревью потребителей | Новый цикл, выход внутренней ORM-модели в публичный DTO, необоснованный export |
| ARC-04 | Один владелец актуального бизнес-правила | Поиск альтернатив и предметное ревью сценария | Две самостоятельно изменяемые реализации одного собственного правила |
| DATA-01 | Права и tenant scope сохраняются | Негативные API/application-тесты с другим tenant/ролью | Чужие данные доступны, адаптер обходит проверку ядра |
| DATA-02 | Повтор/конкуренция/отмена не нарушают состояние | Реальная БД, отдельные соединения, повторная доставка, stale worker | Двойное применение, потеря ревизии, запись отменённого или устаревшего результата |
| DATA-03 | Обновление сохраняет собственные данные | Предыдущий выпуск → миграция → сверка → восстановление | Потеря таблиц/объектов/связей, непроверенный порядок или восстановление |
| DEAD-01 | Подтверждённый собственный мёртвый код удаляется | Static report + поиск entrypoints/DSL/assets + тест загрузки | Подтверждённая находка без исправления или оформленного срока |
| DEAD-02 | Удаление не ломает активные контракты | Проверка регистрации, persisted DSL, SDK/API и профилей | Удаление только по отсутствию coverage/import либо работающего upstream-профиля |
| DEAD-03 | Автоматический patch ограничен доказательствами и текущим снимком | Origin/entrypoints/effects, whitelist, fingerprint до/после, фактические проверки | Неизвестный consumer, опасный side effect, конкурентное изменение файла, непроверенный patch |
| SIMP-01 | Значимый рост сложности собственного сценария разобран | Метрики одним инструментом/профилем, конкретное обоснование упрощения либо сохранения | Новый значимый сигнал без предметного разбора; числовой порог сам по себе не запрещает алгоритм |
| SIMP-02 | Упрощение сохраняет контракт и ресурсные ограничения | Before/after tests, differential cases, ошибки/effects; benchmark при влиянии на алгоритм/I/O | Нарушение поведения/памяти/времени или отсутствие применимого доказательства |
| SIMP-03 | Метрика не улучшается за счёт сокрытия сложности | Ревью всего сценария, helpers/exports/dependencies, baseline diff | Перенос ветвей/переименование/ignore выданы за упрощение без реального улучшения |
| BUILD-01 | Установка и сборка воспроизводимы | Чистый install по выбранному lockfile, build нужного профиля | Несогласованные install-пути, незакреплённый CI tool, native build без нужных flags |
| TEST-01 | Проверки соответствуют затронутому поведению | Область изменения → lanes → commands/results | Отсутствует обязательный lane; mocked UI выдан за доказательство backend |
| POL-01 | Исключения ограничены и не скрывают новые нарушения | Проверка схемы baseline, дат, точных symbols, базовой ветки | Wildcard-ignore, просроченная/неиспользуемая запись, автоматическое принятие новой ошибки |
| POL-02 | Сам контроль нельзя молча обойти | Негативные фикстуры checker, проверка selector и workflow | Анализатор не запущен, ошибка/skip выданы за PASS, правило ослаблено без ревью |

UPG-03, ARC-04, смысловая часть SIMP-01/03 и остальных правил требуют ревью; машинный отчёт помечает их `manual_required` до заполнения доказательства. Компилятор не заменяет эти проверки.

## 2. Область применения и происхождение кода

Три класса: `extension`, `core_change`, `upstream`. Generated/vendor files учитываются с явным источником и способом сборки. Неизвестное происхождение — `unclassified`, а не автоматическое исключение.

Для обычного PR изменение определяется относительно актуальной базовой ветки PR; незакоммиченный локальный запуск дополнительно включает staged, unstaged и untracked. Для происхождения файла используется отдельное сравнение с закреплённым upstream SHA. Эти два сравнения решают разные задачи и не подменяют друг друга.

В upstream-update PR сравнение с новым принятым SHA отделяет собственные изменения от пришедших файлов. Смена SHA сама требует проверки происхождения commit и истории интеграции: нельзя объявить произвольный commit форка «upstream», чтобы скрыть core-diff. Первичная фиксация базы — T0 с проверкой истории; последующие базы должны происходить из проверенного upstream и входить в историю кандидата.

Анализ импортов собственных слоёв учитывает транзитивные обходы. Разрешённый вход адаптера в upstream завершает проверяемую собственную границу; внутренний upstream-граф не создаёт автоматически долг расширения. Новые обратные импорты ядро → расширение допустимы только в конкретных точках подключения.

При сомнении в происхождении или неполном графе результат `INCOMPLETE`. Пользовательская задача может продолжаться в независимой части, но отчёт не называет проверку успешной.

## 3. Матрица запуска проверок

| Изменение | Минимальный набор | Когда расширять |
| --- | --- | --- |
| Только документация/инструкции | Ссылки, формат, согласованность правил и путей | Изменён смысл policy — ревью правил и спецификации checker |
| Собственная Python-логика | Ruff, ARC/DEAD, целевые unit/contracts | Изменение общей границы — тесты всех её потребителей |
| Автоматическое удаление собственного кода | DEAD-03, анализ корней/effects, fingerprint, affected tests/build/registry | Каскадное освобождение helpers — повторный анализ всего модуля |
| Упрощение собственного алгоритма | SIMP-01/02/03, контракт и before/after cases | Смена структуры данных/I/O/обхода — benchmark размеров входа и памяти |
| Собственный HTTP/адаптер ядра | Предыдущее + API envelopes, auth/tenant, integration | Новая регистрация — registry/route inventory; поток — завершение/ошибка/cancel |
| Worker/транзакция | Unit + реальная БД, повтор/конфликт/cancel | Смена task type/checkpoint — recovery и persisted data |
| Frontend | ESLint, type-check, Jest затронутой функции, build; для собственной TS-границы — report-only observer | Routes/auth/общий transport — isolated browser и применимые live journeys; новый dynamic root — явный entrypoint/contract |
| Go/native | Формат, report-only package observer, архитектура собственного пакета, package tests через build.sh | Изменение общего runtime/bindings — все потребители и нужные build profiles; новая directive/root — явный contract |
| Dependency/config/lockfile | Install consistency, сборка и affected lanes | Корневое изменение — полный профиль сборки и интеграций |
| Схема БД/DSL | Контракты, миграция с предыдущего выпуска, DATA-03 | Изменены ссылки/права/очередь — соответствующие негативные тесты |
| Upstream update | Полная применимая регрессия, UPG/DATA/BUILD, все адаптеры | Любая затронутая собственная функция входит независимо от diff её файла |
| Checker/baseline/workflow | Unit/negative fixtures самого checker и проверка required job | Смена selector/schema/tool — повторить положительные и отрицательные пробы |

Selector не опирается только на расширение файла. Изменение registry, global configuration или контракта расширяет набор по потребителям. T3 реализует exact path/prefix/suffix selector, использует свежий T0 и базовую карту для удалённых/переименованных путей и сохраняет причину `NOT_APPLICABLE`. Protocol v1 позволяет workflow запускать selector/aggregate из comparison base и той же реализацией доказывать, что candidate policy не удалила lane/rule/fixture и не сузила coverage. Plan, candidate analysis и final aggregate выполняются в трёх разных job; финальный job на свежем runner повторно извлекает base authority и не запускает candidate lifecycle. Protocol v1 уже находится в base, но до успешных base-mode PR-проб и проверки Required Workflow ruleset режим остаётся `REPORT_ONLY / NOT_ENABLED`.

## 4. Существующие команды

Это команды из текущего дерева, а не утверждение, что они уже выполнены. Использовать подготовленное окружение из инструкций репозитория. Локальные Windows-команды ниже выполняются из корня в PowerShell. Установка зависимостей может менять окружение; для сравнимых результатов CI использует отдельное окружение и закреплённые lockfiles.

```powershell
# Состояние перед работой, включая неотслеживаемые файлы
git status --short
git diff --name-status
git diff --cached --name-status

# Документы: unit, API contract и текущее совмещённое покрытие
.\.venv\Scripts\python.exe -m pytest test/unit_test/api/apps/business_documents test/unit_test/api/apps/restful_apis/test_business_document_api_contract.py --cov=api/apps/business_documents --cov-branch --cov-fail-under=79

# Детерминированные требования документов
.\.venv\Scripts\python.exe -m pytest test/evals/business_documents/test_golden_dialogue_harness.py test/evals/business_documents/test_live_quality_scorer.py

# Примеры точечных проверок существующих интеграций
.\.venv\Scripts\python.exe -m pytest test/unit_test/data_source/test_eva_wiki_connector.py
.\.venv\Scripts\python.exe -m pytest test/unit_test/admin/test_audit_feed.py

# Полный Python unit lane
.\.venv\Scripts\python.exe run_tests.py -i

# Ограниченный DEAD-01 scan/plan; OBSERVED не означает, что удаление подтверждено
.\.venv\Scripts\python.exe tools/quality/check_python_dead_code.py --output output/quality/python-dead-code.json

# Широкий TypeScript observer; OBSERVED/INCOMPLETE не являются architecture/dead-code PASS
.\.venv\Scripts\python.exe -B tools/quality/inspect_typescript.py --output output/quality/typescript-analysis.json

# Широкий Go observer; не компилирует и не заменяет build.sh
.\.venv\Scripts\python.exe -B tools/quality/inspect_go.py --output output/quality/go-analysis.json

# Объединяющая классификация на том же T0 snapshot; COMPLETE не является policy PASS
.\.venv\Scripts\python.exe -B tools/quality/check_runtime_graph_policy.py --output output/quality/runtime-graph-classification.json
```

Ruff применяется к изменённым Python-файлам командами `ruff check <пути>` и `ruff format --check <пути>` в окружении с установленным Ruff. В примере `<пути>` — подстановка реальных файлов, не готовый аргумент. Перемещение модуля требует обновить pytest/coverage paths; не оставлять проверку только старого пустого каталога.

Frontend scripts существуют в `web/package.json`: `lint`, `type-check`, `test`, `build`. В подготовленном `web` их можно вызвать через `npm run lint`, `npm run type-check`, `npm run test -- --runInBand`, `npm run build`: запуск script не означает выбор npm для установки. Текущий isolated browser workflow устанавливает pnpm-зависимости; установка должна следовать конкретному проверяемому lane до завершения T6.

После frontend build: `.\.venv\Scripts\python.exe test/run_browser_regression.py --browser chromium`. Требуются установленный браузер и зависимости. Другие поддерживаемые браузеры — отдельные lanes, когда они применимы.

PostgreSQL lane: `.\.venv\Scripts\python.exe -m pytest test/integration/test_business_document_postgres.py`. Требуется `BUSINESS_DOCUMENT_TEST_POSTGRES_DSN` для одноразового окружения. Отсутствие DSN или skip обязательного теста не подтверждает DATA-02.

Go lane сначала планируется по свежему T0 командой `python tools/quality/check_go_build_profiles.py --output output/quality/go-build-profiles.json`, затем в подготовленном Linux/native-окружении исполняется с `--execute`. Entry points проверяются штатным `bash build.sh --go`, package owners — `bash build.sh --test`; для отдельного Go owner допустима сфокусированная команда вида `bash build.sh --test ./internal/ingestion/pipeline/...`. Полный T1 lane — `bash test/run_go_regression.sh` после подготовки нативной сборки. Не запускать сырые `go test`/`go build` на Windows как замену этому доказательству. `READY/NOT_EVALUATED` от planner означает полный план, а не успешную компиляцию.

Полные live API/UI lanes и условия их запуска описаны в [REGRESSION.md](../../test/REGRESSION.md). Они используют одноразовый стек: существующие fixtures могут удалять данные. Не направлять их на общую рабочую БД. Выбранные upgrade/recovery проверки реализованы и имеют ограничения в [T1](t1-regression-report-ru.md). Ограниченная команда `python tools/quality/inspect_python.py --module business-documents --output output/quality/python-analysis.json` формирует наблюдения, а не PASS архитектуры; точный профиль и коды выхода описаны в [Python T2](t2-python-analysis-ru.md). Аналогично `inspect_typescript.py` создаёт только широкий frontend inventory с ограничениями из [TypeScript T2](t2-typescript-analysis-ru.md), а classifier лишь проверяет полноту exact-классификаций из [объединяющего среза](t2-runtime-graph-classification-ru.md).

## 5. Контракт будущих реестров

Ниже минимальные поля для реализации на T0–T3. Файлы не создаются с пустыми или выдуманными значениями ради зелёного статуса.

В T0 происхождение каждого пути хранится в `file-inventory.json`, связанном с точными списками `module-map.yaml`. Локальная first-parent история и bounded ignored-artifact обзор обновляются явной комбинацией `capture_inventory.py --write --refresh-supporting`; она сохраняет ранее подтверждённые official commit records, не выполняет fetch и не подтверждает новую upstream-базу. `integration_dependencies` описывают взаимодействия областей, но не разрешают импорты. Разрешённые рёбра импортов, symbols и схемы автоматического gate уточняются на T2; инвентаризация их не сертифицирует.

| Реестр | Обязательные сведения |
| --- | --- |
| `upstream-base.json` | schema_version, проверенный repository URL, полный commit SHA, release tag при наличии, evidence принятия базы |
| `module-map.yaml` | module ID, существующие paths, origin class, owner, public surfaces, разрешённые зависимости, runtime profiles, tests |
| `core-changes.yaml` | change ID, стандартные paths/symbols, тип изменения включая delete/rename, причина, owner, контракт/тест, upstream status |
| `baseline.json` | schema_version, записи rule ID + source/target symbol + fingerprint, owner, причина, issue/evidence, срок устранения |

Пути живого модуля должны существовать. Удалённый стандартный путь в core-changes проверяется в upstream-дереве; нельзя требовать его наличия в кандидате. Rename анализируется по обоим путям, а не теряется из-за фильтра `ACM`. Сгенерированный файл имеет связь с генератором и профилем; его ручная правка не маскируется классификацией.

Fingerprint строится по смысловому месту: правило, модуль, символ или ребро. Номер строки используется для отображения. Уничтожение символа удаляет соответствующее исключение; повторное добавление в другом файле не должно автоматически наследовать разрешение.

### Исключения

- Только конкретная находка, конечный срок и владелец. Предлагаемый обычный максимум — 30 дней; продление требует новой причины, результата предыдущей попытки и проверки влияния.
- Исключение не разрешает новую потерю данных, обход прав или ложный PASS. Невыполненная проверка остаётся `INCOMPLETE`, даже если её временно приняли как ограничение задачи.
- Динамически используемый symbol учитывается как entrypoint с тестом, а не как бессрочное исключение dead code.
- Baseline из базовой ветки сопоставляется с результатом кандидата. Удалённые находки убираются, новые блокируются. Изменение baseline требует предметного ревью; автоматическое «обновить ожидаемое» запрещено.
- Пустой baseline валиден только после полноценного анализа, не при отсутствии инструмента или входных файлов.

## 6. Контракт результата checker и CI

Частичный runner `tools/quality/check_architecture.py` реализован на T2 для Python-проверок ARC-01, exact-symbol import allowlist ARC-02, scoped explicit-runtime cycles ARC-03 и явно настроенных runtime contracts. Статическая часть использует AST-граф `inspect_python.py` и явные import roots для вложенных `src` layouts; runtime worker в свежих процессах проверяет import чистого домена и ASR contracts. Профиль может перечислить платформенные кандидаты собственного Python-интерпретатора; если ни один не существует, обязательная проба получает `INCOMPLETE`. Для ASR это выделенное `.architecture-venv`, создаваемое из tracked hash-locked requirements, а не игнорируемый service `uv.lock`. Точные HTTP inventories охватывают Business Documents/OpenMetadata со stub-границей сервисов, полный FastAPI surface ASR в его рабочем каталоге и все 15 REST API loaders, которые свежий T0 относит к локальным core changes или extensions: connector; user/system/dataset/file-commit; search/bot/chat/chat-channel/document; agent; models/provider. Всего закреплено 248 REST routes; для генерируемых endpoint-имён сохраняется точное сравнение. Тип `pytest_contract` запускает только перечисленные точные pytest nodes, отключает автоматическую загрузку сторонних плагинов, явно подключает `pytest_asyncio.plugin` и разбирает JUnit: assertion failure означает `FAIL`, а error, skip, timeout, несовпадение количества или отсутствие отчёта — `INCOMPLETE`. Остальные contracts закрепляют реальные динамические регистрации OpenMetadata component и изменённого Retrieval tool, generated saved Canvas DSL round-trip, передачу tenant/prompt/retry/token-limit и actor/request/result двумя Business Documents production adapters без внешних вызовов, локально затронутые frontend routes/navigation, application bootstrap, provider/router composition и фактическое поведение обеих HTTP service factories через parser из установленного `web/node_modules/typescript`, а также ASR lifespan cleanup/worker thread-pool start/join, порядок и параметры Python API/Flask admin bootstraps, task-executor exact registries, CLI identity, `TE_RUN_MODE`, unacked-before-live dispatch, special fan-out hydration, acknowledgement of non-runnable messages, empty/error poll behavior, cancellation/failure accounting, trace/progress reporting, special-task operation log with referent before ack, one heartbeat report/own-history/stale-worker cleanup cycle, limiter release and shutdown cancellation, singleton/wake lifecycle Business Documents worker, synchronization worker consumer identity/logger/async handoff/settings/signal/dispatch-loop wiring и точное соответствие 37 `FileSource` внешним connector handlers. Harnesses заменяют DB, Redis, HTTP, фоновые сервисы и async scheduler test doubles; API progress loop, реальные queue/DB/Redis operations и ASR inference не запускаются, поэтому проверяется wiring и dispatch, а не доступность инфраструктуры или полная обработка задач. Отсутствие Node/TypeScript считается `INCOMPLETE`, изменение ожидаемого контракта — `FAIL`.

Отдельный `tools/quality/check_python_dead_code.py` реализует только DEAD-01 static `scan/plan` для профилей из `python-dead-code.yaml`, включая чистый Business Documents domain и owned ASR source package с явным import root. Он привязывает scope к свежему T0 owner, учитывает точные статические imports, module-level references, declaration effects и локальную достижимость, а также отклоняет небезопасные пути, перезапись исходника и изменение snapshot во время анализа. Отчёт содержит symbol fingerprints и классы `PRESERVE`, `REVIEW_REQUIRED`, `INCOMPLETE`, `STATIC_CANDIDATE`, но всегда оставляет `dead_code_status=NOT_CONFIRMED` и `NO_AUTOMATIC_PATCH`; exit 0 означает `OBSERVED`, не PASS DEAD-01. Persisted DSL, reflection, внешние contracts, runtime roots и apply/verify пока не покрыты.

Отдельный `tools/quality/inspect_typescript.py` использует locked TypeScript parser для всего `web/src`, проверяет T0 provenance и неизменность исходных байтов, строит import/re-export graph, reverse consumers, browser-entrypoint reachability и eager module cycles, а также записывает glob/dynamic registrations. Tests и declarations индексируются, но не становятся runtime roots. Вычисляемые loaders, `eval`/`Function`, parse/resolution gaps и выход локальной цели за profile дают `INCOMPLETE`; литеральный glob записывается, но не разворачивается. Отсутствие reachability/consumer не означает DEAD-01, cycle не является ARC-03 finding без предметной классификации. Поэтому observer всегда оставляет `policy_status=NOT_EVALUATED`, `dead_code_status=NOT_ANALYZED` и не входит в Python architecture PASS.

Отдельный `tools/quality/inspect_go.py` без выполнения Go-кода читает package/import headers, `go:build`/legacy/filename constraints и directives всех файлов текущего T0. Legacy `+build` учитывает отрицание, comma-AND, space-OR и многострочный AND; некорректный терм делает профиль неполным. Для настроенных `linux-cgo` и `linux-no-cgo` наблюдатель строит разные активные package graphs, standalone server/CLI roots, reverse consumers и cycle signals. Полный синтаксис/types, external modules, embed/linkname targets, compiler/init behavior и native linkage не проверяются; `OBSERVED` означает только завершённое статическое наблюдение с `policy_status=NOT_EVALUATED`, `dead_code_status=NOT_ANALYZED`. Отсутствие package cycle или недостижимого собственного package не является ARC-03/DEAD-01 PASS.

`tools/quality/check_runtime_graph_policy.py` требует совпадающие T0 fingerprint/HEAD/upstream base двух широких отчётов. Он проверяет exact production/development/test roots и evidence, доказывает inherited TypeScript cycle topology по каждому source/target/specifier в принятом upstream commit, требует явную запись каждого owned unreachable runtime path, literal dynamic registration, classified source gap, Go `package main` и directive. TypeScript parser разрешается от явного candidate root, поэтому materialized base producer не ищет `node_modules` внутри evidence bundle. Новое локальное ребро цикла, отсутствующая/stale классификация, изменившийся upstream gap, неизвестный root/directive либо пропавший embed asset дают `INCOMPLETE`. Текущий `COMPLETE` означает только полную классификацию: `policy_status=REPORT_ONLY`, `architecture_status=NOT_EVALUATED`, `dead_code_status=REVIEW_REQUIRED`, автоматических удалений нет.

Отдельный `check_go_build_profiles.py` применяет BUILD-01 к Go-дельтам: unmapped/wrong-owner/overlap — `FAIL`, недоступный driver/toolchain, skip или отсутствие структурированного package PASS — `INCOMPLETE`. Исторические сохранённые данные этими tools не проверены. Неохваченные adapters/surfaces и итоговые TypeScript/Go policy/dead-code verdicts остаются `manual_required`/неоценёнными. T3 `check_architecture_policy.py` выбирает применимые Python architecture, runtime-graph, Go-plan и policy-fixture lanes, повторно вычисляет план при aggregation, проверяет единую identity/fingerprint, source bundle и producer/config digests и трактует отсутствие evidence как `INCOMPLETE`. Его protocol v1 сверяет hashes evaluator/base/candidate policy, `compare` допускает только монотонное расширение существующих selectors/rules/fixtures/protected sources, `materialize` извлекает intact protected producers из authority bundle, а `fixtures` запускает exact base `path::nodeid`, передаёт им candidate root явно, отключает ambient conftest/plugin injection и встраивает проверяемый JUnit в identity-bound JSON-attestation. При `authority_source=base` любое отличие обычного protected analyzer/config/fixture/dependency source закрывает lane; только candidate policy может отличаться как `BASE_FIXTURE_REVIEW`, при этом base evaluator/materialization остаются authoritative, candidate policy проходит монотонное comparison и требуется ручной review. Checker и workflow остаются `MUST_MATCH`. Bootstrap использует candidate source bundle и проверяет только self-consistency, не совпадение с comparison base. Analysis отклоняет весь запуск до lifecycle setup, если хотя бы один выбранный lane не имеет intact source bundle. Aggregate требует отдельные authoritative/candidate plan files и `policy-compatibility.json` с совпадающими identity, точными plan hashes и checked lanes; отсутствующий или stale plan/compatibility означает `INCOMPLETE`. Только точная пара `authority_source=base / compatibility_status=COMPATIBLE` даёт compatibility `PASS`; bootstrap обязан сообщать `candidate-bootstrap / BOOTSTRAP_SELF_CHECK` и агрегируется как `OBSERVED`, поскольку trusted base coverage не оценена. `.github/workflows/architecture.yml` без label/path filters отделяет статическое планирование от candidate analysis; analysis запускает materialized producers через isolated safe-path runner, выполняет fixtures как `success()`-предусловие остальных producers, очищает control/secret environment дочерних Python и Node subprocesses protected producers и устанавливает TypeScript parser во временной копии materialized authority web manifest/lock/config без pnpm hooks/scripts/workspace. Только при `authority_source=base` эти package-manager inputs доверенно закреплены base и их изменение блокирует Node-consuming lanes; bootstrap использует candidate self-check inputs. Единственный стабильный финальный job `architecture-policy` на свежем runner заново извлекает base checker/capture/policy, повторяет selection/compare, сверяет compatibility из plan artifact, загружает reports и выполняет aggregate без candidate dependency install, analyzers или pytest. Эти меры не являются OS sandbox для намеренно враждебного candidate runtime code или package manager. Protocol v1 уже находится в base; base-mode PR-сценарии, runtime isolation и Required Workflow ruleset ещё не проверены полностью, поэтому это подготовленный report-only gate, а не действующий required gate.

Машинный отчёт должен содержать schema/tool versions, candidate SHA, PR base SHA, upstream SHA, профиль/ОС, dirty-state или fingerprint локального снимка, scope/entrypoints, команды и коды выхода, применимые правила, findings, использованные исключения, manual review requirements, результаты lanes и ссылки на доказательства. Секреты и полные документы в отчёт не включаются.

| Статус | Значение | Итог обязательного gate |
| --- | --- | --- |
| `PASS` | Применимая проверка выполнена, нарушений нет | Успех |
| `FAIL` | Доказано нарушение/регресс | Отказ |
| `INCOMPLETE` | Проверка применима, но данные/инструмент/окружение отсутствуют или результат недостоверен | Отказ |
| `NOT_APPLICABLE` | Проверенный selector определил отсутствие применимости и записал причину | Допустимо; не заменяет PASS для применимого lane |

Предлагаемые коды выхода агрегатора: 0 — все применимые автоматические проверки выполнены успешно; 1 — есть нарушения; 2 — анализ неполон или ошибка конфигурации/инструмента. При сочетании нарушений и неполноты использовать 2, сохранив все findings. Ручное ревью явно отражается отдельно и закрывается процессом PR; автоматический PASS не означает автоматическое одобрение смысловых правил.

Unit-тест с `skip` внутри lane требует разбора: обязательный случай, который не исполнился, делает его `INCOMPLETE`. Допустимые optional cases перечисляются заранее. Проверка не может самостоятельно объявить свой провал неприменимостью.

В CI проверки выполняются без `--fix` и `git add`. Кандидат с изменением checker/workflow/baseline проверяется относительно политики и fixture contracts базовой ветки; нельзя доверять изменённому скрипту, самодекларированному имени analyzer или одноимённому candidate test. T3 protocol извлекает checker/capture/policy из comparison base, запрещает сужение base coverage и запускает materialized protected producers с isolated import path, а fresh final job не наследует изменяемый trusted bundle из candidate-analysis job. Метка `BASE_FIXTURE_REVIEW` и manual-review запись сами по себе не обеспечивают approval; поэтому исключение распространяется только на declarative candidate policy, тогда как checker/workflow обязаны совпасть с base. Намеренно враждебный runtime/control code требует отдельной OS/process/filesystem isolation от command files и supervisor-owned evidence, а обновление checker/workflow/protected fixtures или сужение controls — отдельного аудированного protocol/bootstrap flow. Неизменяемость определения workflow и обязательный control-owner approval должны отдельно обеспечиваться Required Workflow ruleset либо эквивалентным внешним pinned check: обычный required status context в branch protection фиксирует имя результата, но не содержимое PR workflow.

## 7. Приёмочные пробы самой системы контроля

Создать маленькие изолированные fixtures при реализации checker; не вносить намеренные дефекты в рабочий код.

| Проба | Ожидаемый результат |
| --- | --- |
| Domain импортирует ORM напрямую | ARC-01 / FAIL |
| Domain обращается к ORM через собственный helper | ARC-01 / FAIL |
| Domain импортирует чистый собственный тип, adapter — разрешённый upstream service | PASS |
| Импорт чистого модуля запускает родительский bootstrap | ARC-01 / FAIL; проверять в отдельном процессе без настоящих внешних подключений |
| Новый импорт ядро → расширение вне registry | ARC-02 / FAIL |
| Функция вызывается только по имени из зарегистрированного DSL | Не удалять автоматически; PASS после entrypoint/load test |
| Собственный export действительно не имеет потребителей/контрактов | DEAD-01 finding с доказательствами |
| Стандартный backend не включён локально | Не считать собственным мёртвым кодом |
| Изменён/удалён/переименован стандартный файл без записи | UPG-02 / FAIL во всех трёх случаях |
| Upstream update добавляет стандартный файл без локального изменения | Не требовать записи собственного core-change |
| Подмена upstream SHA на commit форка | UPG-01 / FAIL либо INCOMPLETE при невозможности подтвердить источник |
| Baseline расширен новым нарушением, истёк или содержит исчезнувший symbol | POL-01 / FAIL |
| Анализатор отсутствует, завершился ошибкой, либо scope пуст по ошибке | POL-02 / INCOMPLETE, ненулевой exit |
| Изменён только README | Кодовые lanes обоснованно NOT_APPLICABLE; doc/policy checks выполнены |
| Изменён общий transport/lockfile | Selector включает потребителей/сборку, а не только тесты одного файла |
| Required workflow не запускается без метки ci | T3 не принят до исправления и проверки настроек |
| Checker из PR всегда возвращает 0 | Проверка относительно base policy/negative fixtures должна заблокировать такой PR |
| Автоочистка удаляет регистрационный import или имя, используемое через locals | DEAD-03 / FAIL; механизм не должен предлагать AUTO_LOCAL |
| Файл изменился после подготовки автоматического patch | DEAD-03: применение остановлено, требуется новый анализ |
| Упрощение меняет порядок/дубликаты или приоритет ошибок | SIMP-02 / FAIL |
| Новое превышение сложности получило только пустой комментарий «так надо» | SIMP-01: требуется предметный разбор, предупреждение не закрыто |

Проверять также нормализацию путей Windows/Linux, relative/alias imports, NUL-safe имена Git и build-tag профили. Исключённый native-профиль должен быть виден в отчёте; его отсутствие не является доказательством неиспользования кода.

## 8. Доказательства завершения задачи

Минимальная запись: изменение поведения, затронутый модуль/ядро, применимые rule IDs, команды и фактические результаты, удалённый собственный код, ограничения. Для миграции дополнительно — исходный выпуск, схема/объекты до и после, результат восстановления. Для upstream update — обе версии и изменения адаптеров.

Нельзя писать «вся регрессия пройдена» по isolated UI или unit subset, «архитектура соблюдается» без проверки границ, «мёртвого кода нет» по одному Ruff, «merge защищён» по существованию workflow YAML. Отчёт ограничивается тем, что реально доказано.
