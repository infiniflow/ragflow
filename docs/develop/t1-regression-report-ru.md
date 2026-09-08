# T1: проверка сохраняемых функций

Дата базового кандидата C: 2026-09-06; актуальное продолжение: 2026-09-08. **Приёмка T1: BASELINE_COMPLETE_WITH_KNOWN_FAILURES.** Основные прогоны выполнены на неизменяемом кандидате C. Его два сбоя сохранены; последующие исправления тестового стенда проверены отдельно. На локальном Docker-контуре затем закрыты ранее ошибочно названные внешними prerequisites: выполнены live-проверки EVA/OpenMetadata, согласованное PostgreSQL+MinIO восстановление, реальный ASR и воспроизводимые MRZ/saved-DSL lanes. MRZ и часть уже сохранённых Canvas DSL имеют локализованные baseline failures; эти функции нельзя рефакторить до отдельного исправления. Статус T1 фиксирует наблюдаемый baseline, а не объявляет всё приложение исправным или готовым к выпуску.

- HEAD базового кандидата C: `6a50434a41c96ec8b6d0d7aad3ffe489046e5e0d`; HEAD текущего продолжения: `f019c812ded5b8e9c127dfe824276d1dbf5179ad`.
- Проверенная upstream-база: `cb93883f3f8c975eecb2fed81210effeb3bdb06f`.
- Candidate C: `505e53dcddb9a6bf9536d915075e52a9e0b2dd3d3d325316c65d15cb9c84b668`, **4 750 файлов**, `S:/ragflow-t1-candidate-20260906-c/source`.
- Снимок содержит незакоммиченные изменения и служит входом тестов. Он не является готовым выпуском.

[Машинные результаты](t1-regression-results.json) содержат test IDs, команды, результаты по возможностям и хеши доказательств. Финальные логи находятся в `.codex_tmp/t1-20260906/*/candidate-c/`, live — в `output/t1-continuation-20260906/live/candidate-c/`. Числа разных наборов пересекаются: их нельзя складывать в количество уникальных тестов.

## Проверки кандидата C

| Проверка | Результат | Что доказывает |
| --- | --- | --- |
| Полный Python unit, `run_tests.py -i` | 2 735 passed, 25 skipped; отдельно 146 subtests passed | Полный unit-набор C; 25 существующих upstream skips сохранены |
| Business documents и API contracts | 136 passed; coverage 79,79%, порог 79% | Реальные service bodies и изолированные зависимости; внешние AI/EVA вызовы подменены |
| Требования и scorer | 5 passed | Все 24 scripted golden-сценария и детерминированный scorer |
| PostgreSQL races | 3 passed | Отдельные соединения: повтор команды, конфликт версии и захват job |
| User/tenant/connector route units | 33 passed | API-контракты с подменёнными зависимостями; включая шесть новых случаев выбора модели |
| Предыдущий выпуск → текущая схема → восстановление | 1 passed | Настоящие initializers, dump/restore, данные 19 старых / 23 текущих таблиц, grants и отказ DDL |
| MinIO raw backup/restore | 1 passed | Байты и metadata трёх объектов, независимость восстановленного тома, очистка |
| ASR | 46 unit + 1 real passed | Контракты сервиса; настоящий endpoint/worker/FFmpeg/T-One распознал заранее заданную фразу |
| Upgrade identity с настоящим jq | 11 passed, 0 skipped | Синтетические каталоги, реальные Bash/jq; это не запуск обновления рабочей системы |
| Go: четыре пакета через `build.sh` | 1327 passed, 3 skipped | DAO, model adapters, agent components и handlers; тесты выполнены заново |
| Frontend Jest | 22 suites, 161 passed, 0 skipped | Все текущие suites; область coverage и пороги сохранены |
| Frontend type-check / lint / production build | PASS / PASS / PASS | Lint: 0 ошибок, 138 предупреждений; clean install по pnpm lock, свежая сборка |
| Mocked Chromium / Firefox / WebKit | 50 passed, 1 failed / 51 passed / 51 passed | Построенная SPA с подменёнными API; не доказательство backend |
| Live Python stack | 5 passed, 1 failed | Пять успешных сценариев; полный roundtrip остановился на устаревшем locator поля поиска |

Глобальное frontend coverage: statements/lines **8,98%**, functions **15,34%**, branches **49,33%**, 1 152 файла. Прохождение существующих порогов не означает полного покрытия интерфейса. Это измерение не сравнивается напрямую с выделенным backend-модулем документов.

## Исправления и очистка

**Выбор Ollama в chat API.** До правки явный `llm_id` отклонялся при пустом API-ключе, хотя локальная модель была настроена. Теперь API проверяет конфигурацию модели в tenant scope. Тип выбирается как в `dialog_service`: chat имеет приоритет, vision-only сохраняет image2text. Generation parameters сохранены; недоступная конфигурация возвращает существующий контролируемый ответ `Cannot use specified model` с code 102 до генерации. Новые route-тесты проверяют keyless chat/vision, keyed chat, отсутствующую/отключённую модель и ранний отказ резолвера чужого provider. Это unit-доказательство границы резолвера, не полная live ACL или vision-inference проверка.

Изменение `api/apps/restful_apis/chat_api.py` классифицировано как необходимая локальная правка стандартного ядра: файл присутствует в принятой upstream-базе. Удалён неиспользуемый импорт `get_api_key` и его ненужная тестовая подмена. Логика проверки остаётся у существующего владельца, нового адаптерного framework или второго business path не создано.

**Тестовые данные и UI.** Go-фикстура сортировки версий теперь задаёт `CreateTime`, по которому DAO действительно сортирует; assertions не ослаблены. Live chat/search используют собственный загруженный и реально проиндексированный документ. Исправлены устаревшие selectors, публичные состояния `DONE`/`FAIL` и DTO `search_id`. Режим сравнения проверяет два разных `llm_id`, два завершённых успешных потока и состояние UI. Чтение тела SSE выполняется после ограниченного ожидания завершения запросов. Переключатели thinking/internet проверяются включением и возвратом в исходное состояние перед обычным текстовым запросом.

**Удалён подтверждённый лишний код.** Удалены 43 строки устаревшего opt-in golden-intake теста с эксклюзивными fixture/import и три недостижимых selector-helper: `_mm_click_generic_model_option`, `_mm_click_model_option_by_testid`, `_mm_open_model_options`. Все 24 scripted golden-сценария и AST сохранённых golden-функций остаются прежними. Отдельный `test_live_model_quality.py` и используемый им scorer сохранены: это действующий потребитель. Его последующий реальный запуск и сбой описаны ниже. Настоящий intake проверяет непустые вопросы и 2–4 варианта **на вопрос**, согласно опубликованному контракту.

Также в полном C unit-наборе находятся добавленные audit persistence/retention, group rollback/grants, credential scope, отрицательная admin-матрица, настоящая загрузка текущего OpenMetadata DSL и защита от необработанных page errors. ASR readiness использует реальный lifespan; FFmpeg-error и installer-contract проверки согласованы с исполняемым путём. Bash-синтаксис проверяется для каждого из пяти scripts отдельно; отсутствие инструмента не выдаётся за PASS.

Параллельные изменения инструментов сборки и поставки вошли в снимок и сохранены, но не приписываются работе T1. Точные владельцы и provenance находятся в `tools/quality/module-map.yaml`, `core-changes.yaml` и `file-inventory.json`. Применены UPG-02/03, DATA-01/02/03, DEAD-01/02/03, TEST-01 и POL-02. Полные architecture/dead-code/complexity runners и их блокирующая политика остаются работой T2/T3; их прохождение не заявляется. Первый наблюдательный Python-инструмент появился отдельно после C.

## Границы реальных доказательств

- **PostgreSQL.** Временный кластер, старый выпуск `v1.12.0` (`3cf71a547e2a7610da908163a19a0e2d595e01c8`), синтетические документы/ревизии/evidence/credentials/audit/grants. Сверены выбранные строки и расшифровка контрольного токена. Event trigger PostgreSQL действительно отклоняет создание `public.access_group`; sequence доказывает один вызов вопреки rollback. Это серверная инъекция ошибки SQLSTATE 53100, а не физическое заполнение диска. Для обеих версий использован установленный dependency set; старый installer и все прежние зависимости не воспроизводились.
- **MinIO.** Настоящие S3-запросы, raw tar остановленного `/data`, отдельный restore volume, сравнение байтов/metadata. Экспортный объект содержит синтетические байты: качество DOCX не проверяется. Тест не доказывает атомарный backup PostgreSQL вместе с объектами.
- **ASR.** Реальное распознавание одной синтетической русской записи, существующий T-One model/runtime с записанными хешами. Не общий quality benchmark, не микрофон и не end-to-end отмена.
- **Live UI и LLM.** Новый одноразовый PostgreSQL/Redis/MinIO/Elasticsearch/app stack, реальный синтетический администратор, установленные Qwen2.5, Llama3.1 и bge-m3. На C прошли upload/parse/delete трёх простых PDF, chat, search, сравнение моделей и intake. Отдельный roundtrip подтвердил upload → parse → retrieval и очистку, но остановился на locator; повтор после исправления описан ниже. Intake документов исполняет API → worker → настоящую LLM после browser login; это не полный UI-путь редактора и не оценка качества итогового документа.
- **Среда live.** Зависимости взяты из записанного существующего образа, каталог моделей инициализирован из канонических данных в пустой QA-БД. Это не доказательство новой release-сборки, чистого production bootstrap или сложного PDF/OCR. Shared application data не использовались.
- **Пропуски.** 25 существующих upstream Python skips и три Go-сценария (реальный Gitee, два Stagehand/browser с внешними credentials) остаются непроверенными. Отсутствие QA-доступа не преобразовано в PASS.

## Повторяемость, сбои и очистка

Кандидат C и его source manifest не менялись. Python/Go/frontend выполнялись в отдельных копиях либо с read-only source mounts; caches и evidence вынесены наружу. Перед/после сверены входные файлы и байты dist. `docker/.env` исключён штатным механизмом snapshot; live использует отдельную одноразовую конфигурацию. Версии runtime, модельные/DeepDoc assets и native library записаны отдельно. Существующая C++ библиотека повторно использована только после равенства 132 native/build входов B и C; результаты Go-тестов не брались из cache.

Начальные прогоны и candidate B сохранены как история. Они не подменяют результат C: B обнаружил Go-фикстуру и ошибки live-сценариев. Development attempt07 прерван после ошибки памяти хоста; финального результата нет, root удалил оставшийся собственный Compose project и проверил отсутствие ресурсов. Он обозначен `INTERRUPTED_NOT_PASS`.

Первый Chromium на C дал **50 passed / 1 failed** из-за оборванной загрузки двух существующих JS chunks; React Router перехватил import errors, поэтому они не стали `pageerror`. Сохранены первый XML/log, хеши файлов и причина повторного запуска. Повтор после снятия нагрузки также дал **50 passed / 1 failed**, но на другом файле и другом сценарии. Нативный Chromium gate кандидата C остаётся **FAIL**; диагностические прогоны не подменяют его результат.

Документы, временные схемы/БД, контейнеры, тома и сети проверок очищены; receipts очистки находятся рядом с lane results. Общие compiler caches сохранены. Commit, merge, deployment и изменение рабочей поставки не выполнялись.

## Отдельная проверка исправлений тестового стенда после C

В новой копии `S:/ragflow-t1-scoped-patch-20260906/source` изменены ровно три файла относительно C:

1. `test/run_browser_regression.py`: HTTP/1.1 для повторного использования соединений при загрузке chunks.
2. `test/unit_test/playwright/test_browser_server.py`: настоящие HTTP-запросы проверяют один TCP для нескольких assets/SPA-перехода и отказ 503 для неподменённых `/api/` и `/v1/`.
3. `test/playwright/e2e/test_live_document_roundtrip.py`: существующий `_search_query_input` вместо несуществующего test ID; assertions полного сценария сохранены.

Все три файла — локальные расширения относительно принятой upstream-базы. Хеши исходных и исправленных файлов, всей копии и неизменённого dist находятся в `frontend/candidate-c/scoped-patch/overlay-manifest.json`. Продуктовые исходники остаются байт-в-байт C.

| Проверка после исправления | Результат | Область |
| --- | --- | --- |
| Server/page-error/workflow units | 28 passed | Отдельный root-прогон; изменённые server/test хеши совпадают с копией, поддерживающие тесты совпадают с C |
| Нативный Chromium | 51 passed | Настоящий исправленный runner, прежние 51 scenario и тот же dist |
| Live roundtrip | 1 passed | Новый одноразовый stack на product source C и явно указанный исправленный тест |

Исправленный roundtrip подтвердил весь путь: загрузка через UI, настоящий worker, retrieval из загруженного документа, тот же результат в browser search, ответ чата с контрольным фактом «seven copper telescopes» и удаление dataset. Это один синтетический документ и один вопрос, не общий RAG-quality benchmark.

Контролируемое сравнение 60 GET: HTTP/1.0 — 60 принятых TCP, HTTP/1.1 — один. Отдельная диагностика браузера дала 51 passed, 9 798 GET через 310 соединений без прежних chunk import errors. Это подтверждает сокращение числа соединений; системное исчерпание портов напрямую не измерялось. Диагностический PASS не подменяет исходный FAIL C.

После C не повторялись неизменённые backend/Go/ASR suites: исправления затрагивают только тестовый сервер и locator. Полный единый прогон всего текущего дерева после этих правок не заявляется. Дополнительно обновлены инструкции, отчёт и точные записи реестров; fingerprint текущего дерева находится в `tools/quality/file-inventory.json`.

## Исторические незакрытые условия после кандидата C

### Продолжение: реальный quality lane

На отдельной копии C с тремя точными eval-правками запущен существующий `test_live_model_quality.py`: настоящий Qwen2.5 7B, AI/worker, одноразовый PostgreSQL tenant/model catalog и отдельный SQLite-файл агрегатов. Поиск/evidence и ACL этого теста остаются контролируемыми фикстурами. Общая рабочая БД не использовалась.

Исправлены устаревшее требование BPMN вместо опубликованного PlantUML activity-контракта, ошибка monkeypatch-target и SQLite `:memory:`, непригодная для отдельного heartbeat-соединения. В scorer устранён пропуск выдуманных процентов: `42%` и `42% uptime` теперь снижают precision, известные `99,9%` не считаются выдумкой. Проба до исправления: **2 failed / 1 passed**; scorer после: **6 passed**. Пороги 95% и 3,2 не снижены; все прочие live assertions сохранены. Удалена только устаревшая BPMN-фикстура этого теста, поддержка форматов продукта не менялась.

**Реальный результат: 1 failed, 1 teardown error, 0 skipped; 45,47 с.** Первый intake завершился, второй после трёх попыток получил `QUESTION_STAGE_CONFLICT` и `DEAD`: schema прошла, но вопрос имел неподходящий этап. Черновик не создан, rubric/grounding score — **NOT_EVALUATED**, а не ноль или PASS. Отдельно teardown выявил необработанную coroutine `LiteLLM Logging.async_success_handler`. Предшествующий запуск, остановившийся на ошибке fixture до model call, сохранён как отдельный prerequisite FAIL.

Доказательства: `.codex_tmp/t1-20260906/contracts/live-quality/attempt02/` (`lane-summary.json`, `identity.json`, `origin.json`, XML, jobs/projection и cleanup). C и execution copy проверены по хешам; собственный PostgreSQL-контейнер удалён, томов не создавалось, оставшихся ресурсов — 0. AI, промпты, retry policy и warning policy не менялись. Следующий шаг этой границы — исправить причину несогласованного этапа и lifecycle callback LiteLLM с отдельными регрессиями, затем повторить quality lane до черновика.

Этот запуск не заменяет результаты C. Параллельно добавлен [наблюдательный инструмент T2](t2-python-analysis-ru.md); его отчёт не закрывает указанные сбои поведения.

### Условия приёмки

1. **EVA / OpenMetadata:** отдельные QA endpoints и локальная конфигурация credentials, реальные чтение/запись, повторы после неопределённого ответа и восстановление. Текущие mocks/DSL checks этих условий не закрывают.
2. **Полнота конкретных функций:** успешный document-quality прогон после исправления выявленных сбоев, реальные MRZ-изображения и исторические DSL, browser journeys OpenMetadata/audit, микрофон/отмена ASR и серверная матрица всех затронутых ролей. Перед рефакторингом каждой непроверенной границы закрыть её условия, а не полагаться на общее количество PASS.
3. **Данные и поставка:** согласованный PostgreSQL+objects snapshot и восстановление всей поставки с её реальными зависимостями. Текущие независимые проверки подтверждают выбранные данные и storage-формат. Пробное обновление upstream и полноценный installer/recovery rehearsal относятся к T6 и ещё не выполнялись.

Следующий шаг T1 — получить QA-конфигурации внешних интеграций и выполнить соответствующие live lanes. После защиты нужных границ перейти к T2: точным правилам зависимостей и проверенным анализаторам. Команды и prerequisites приведены в [test/REGRESSION.md](../../test/REGRESSION.md); порядок этапов — в [плане перехода](architecture-transition-ru.md).

## Продолжение 7–8 сентября 2026: закрытие локальной части T1

Предыдущий сбой real-model quality воспроизведён и устранён в текущем рабочем снимке. Все затронутые пути относятся к собственному расширению `business-documents` по T0; стандартное upstream-ядро в этой правке не менялось. Владелец поведения остался прежним, второго production path или совместимого legacy API не добавлено.

Исправлены четыре причины нестабильности model boundary: этап вопроса теперь привязывается к авторитетному типу job, поля соседних question-схем удаляются до строгой валидации, неподдерживаемое форматирование блока сохраняется как текст в той же секции, а точная ссылка на Evidence добавляется только при дословном совпадении характерного числового или технического идентификатора. Промпты явно фиксируют `INTAKE`, допустимые типы блоков и запрет `source_event_ids` вне объявленной JSON Schema. Завершение краткоживущего event loop теперь ограниченно дожидается callback worker LiteLLM 1.82.5 и гарантированно очищает его очередь; прежняя необработанная coroutine не повторилась.

Реальный изолированный quality lane с Qwen2.5 14B прошёл полный путь intake → answers → draft: **1 passed за 102,95 с**, все четыре jobs завершены. Итог scorer: **3,6**, grounded-reference precision **1,0**, две grounded claims, нет unsupported measurable claims и hard failures; protocol separation и границы вариантов вопросов соблюдены. PostgreSQL работал в отдельном tmpfs-контейнере, общая БД не использовалась; после запуска осталось 0 контейнеров и 0 именованных томов. Точная модель: `qwen2.5:14b-instruct`, digest `7cdf5a0187d5c58cc5d369b255592f7841d1c4696d45a8c8a9489440385b22f6`. Доказательства находятся в `.codex_tmp/t1-20260907/live-quality/`. Предшествующие попытки с Qwen2.5 7B, завершившиеся неполным или невалидным draft, сохранены как отрицательный результат и не выданы за PASS.

Текущий локальный регресс:

| Проверка | Результат |
| --- | --- |
| Полный Python unit | 2814 passed, 35 skipped, 167 subtests passed; 352,41 с |
| Business documents с покрытием | 171 passed; 79,94% при пороге 79% |
| Golden + quality scorer | 8 passed |
| Route units | 33 passed |
| Browser result assertions | 53 passed |
| Frontend Jest | 23 suites, 170 tests passed |
| TypeScript / ESLint | PASS; lint 0 errors и 138 существующих warnings |
| Production frontend build | PASS; 13 226 modules transformed |
| Chromium / Firefox / WebKit | 52 / 52 / 52 passed на одном production dist |
| T0 capture | PASS; после Docker follow-up 701 запись, 0 неклассифицированных; актуальный fingerprint в `tools/quality/file-inventory.json` |
| Точный ARC-01/ARC-02 runner | PASS; 1 boundary, 1 connection, 4 runtime probes, 0 findings |
| Python observation владельца | INCOMPLETE / NOT_EVALUATED; 15 известных динамических или неразрешимых транзитивных imports, без заявления policy PASS |

Первый параллельный запуск Firefox дал 51 passed / 1 failed: один сценарий не увидел `files-list` за 5 секунд. Тот же сценарий отдельно прошёл 1/1, а полный последовательный повтор Firefox — 52/52. WebKit в том же параллельном запуске прошёл 52/52. Это классифицировано как ресурсная нестабильность параллельного тестового стенда; исходный FAIL сохранён и не используется как итог матрицы. Go-набор не повторялся, потому что текущая правка не затрагивает Go/native/build inputs; ранее выполненный native Go результат остаётся историческим доказательством, а не результатом нового единого снимка.

### Вердикт до проверки Docker-контура

Локальная часть T1, включая ранее падавший document-quality lane, завершена. На этом промежуточном шаге общий этап T1 считался **INCOMPLETE / BLOCKED_EXTERNAL_PREREQUISITES**, потому что Docker-контур ещё не был обследован. Этот вердикт сохранён как история и заменён результатом следующего раздела: необходимые сервисы, credentials и одноразовые targets оказались доступны локально.

## Продолжение 8 сентября 2026: реальные проверки локального Docker-контура

Контур обследован без изменения модельных настроек и промптов. В нём найдены работающие RAGFlow, PostgreSQL, MinIO, T-One ASR, EVA Wiki и OpenMetadata, а также уже заведённые непроизводственные connector records. Секреты читались только процессом теста внутри application container, не печатались и не сохранялись в evidence.

Добавлены воспроизводимые opt-in lanes и генератор синтетических fixtures:

| Граница | Реальный результат |
| --- | --- |
| OpenMetadata | **PASS:** validation, чтение 908 таблиц, инъекция одного `503` с успешным повтором, изменение description выбранной тестовой таблицы, readback и точное восстановление исходного значения |
| EVA Wiki | **PASS:** инъекция одного `503` с успешным повтором, создание отдельной синтетической страницы, publish, чтение, update, повторный publish, удаление и проверка отсутствия |
| PostgreSQL + MinIO | **PASS:** 1 test за 21,18 с; единый `snapshot_id` и digest, freeze/backup, восстановление в новые контейнеры и тома, проверка связности; отрицательный control с удалённым object обнаруживает частичное восстановление; после cleanup ресурсов не осталось |
| T-One ASR | **PASS после исправления:** короткая запись распознана точной ожидаемой русской фразой; отмена длинной записи завершилась `canceled`, `result=null`, artifacts пусты; live integration 2/2 и полный service suite 47 passed, 2 skipped |
| Synthetic saved DSL | **PASS:** current и две historical revisions записаны, загружены настоящим `Canvas` и удалены; после проверки строк не осталось |
| MRZ template | **FAIL, воспроизводится:** визуально читаемый TD3 PNG проходит до реального Image2Text boundary без transport/component error, но возвращаемый текст не содержит fixture MRZ; детерминированный validator правильно отвечает `mrz_not_found` |
| Существующие saved DSL | **KNOWN BASELINE FAILURES:** current — 16/18 load, 2 fail; attached history — 158/195 load, 37 fail. Классы: отсутствующая runtime dependency 1 current/30 history, обязательная component configuration 1/6, ещё один historical `AssertionError`; 9 orphan versions исключены из проверки как не привязанные к текущему Canvas |

Fixture generator создал `synthetic-russian.wav` (281 924 байта, SHA-256 `70396b95e9f9fd02906ae25e8059be3b51d29b434d61ff3a197ea4ead839bfca`), `synthetic-russian-long.wav` (3 382 614 байт, `de15164bc911058b9a5ea8d574fcdf69079002f4176cf80ad4556d4ef9a3d381`) и `synthetic-td3-closeup.png` (23 867 байт, `3d2852e423d7716530bf3174dd5291f46cf337019c261b3ba70a8bba7fad4c8d`). Аудио синтезируется установленным русским Windows SAPI voice и нормализуется FFmpeg; MRZ содержит валидный TD3 example и визуально проверен.

Первый live cancel выявил дефект owned-расширения `asr`: после принятой отмены синхронный inference всё равно записывал terminal state `done`. В `jobs/worker.py` отмена теперь выигрывает на следующей безопасной границе стадии после acquire, preprocess, inference или enrich и очищает result/artifacts/error. Движок остаётся синхронным и не прерывается посреди вызова, поэтому это cooperative cancellation, а не мгновенная остановка вычисления. Unit regression блокирует обратное появление `done` после mid-inference cancel.

MRZ-сбой локализован на выходе уже настроенной Image2Text boundary: доставка data URI, запуск Canvas и deterministic validator завершились штатно; валидатор отклоняет неверный OCR output. Перебирать модели, менять tenant defaults или редактировать prompt для получения зелёного результата не стали. Аналогично существующие saved DSL не мигрировались автоматически и отсутствующие providers не восстанавливались вслепую. Их IDs и классы ошибки доступны в opt-in read-only audit; значения model/provider и credentials в отчёт не выгружаются.

### Итоговый вердикт T1

Критерий T1 выполнен как **BASELINE_COMPLETE_WITH_KNOWN_FAILURES**: у обязательных локальных возможностей есть реальные повторяемые проверки; прежние пропуски окружения закрыты; найденный ASR-дефект исправлен; оставшиеся MRZ и persisted-DSL сбои воспроизведены и локализованы. Это разрешает продолжить T2/T3 для остальных владельцев, но ставит явный запрет на рефакторинг MRZ Image2Text boundary и несовместимых persisted DSL до исправления или предметной миграции данных.

Не проверены физическое аудиоустройство браузера и полный release installer/recovery rehearsal. Первое не подменяет доказанный API/worker cancel path и требует hardware/browser lane только перед изменением voice UI. Второе относится к T6 по плану перехода, а не к prerequisites T1. Команды повторения находятся в [test/REGRESSION.md](../../test/REGRESSION.md), структурированные результаты — в `t1-regression-results.json`.
