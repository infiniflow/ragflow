# Устранение повторной подготовки: задача 2

Дата: 2026-09-05. Реализованы локальные изменения; это не подтверждение полного
release gate и не изменение состава обязательных тестов.

## Действующий порядок

1. После checkout или изменения frontend lockfile явно подготовить зависимости:
   `python3 tools/hooks/prepare_web.py` (Windows: `.venv/Scripts/python.exe tools/hooks/prepare_web.py`).
   Выполняется один `npm ci`, без установки Git hooks. Во время подготовки не
   запускать другие установки/сборки в том же `web/node_modules`.
2. Hooks используют только установленные Prettier/ESLint, не вызывают `npx`,
   не устанавливают зависимости и не ожидают глобальный `/tmp`-lock.
   При отсутствии инструментов требуется явная подготовка; ошибка не считается PASS.
3. CI перед Lefthook передаёт NUL-separated список путей в `prepare_web.py
   --files-from <file>`. Без frontend-входов установка не нужна; с ними она
   выполняется до параллельных hooks. Оба существующих workflow задают
   `LEFTHOOK_CHECK_ONLY=1`. Их объединение остаётся отдельной задачей 6.
4. `npm run test:focused -- <test>` использует штатный Jest cache без coverage;
   `npm run test -- --runInBand` сохраняет coverage и все действующие пороги.
   Для диагностики можно передать `--no-cache`; кешируются преобразования,
   а не успешные результаты тестов — тесты исполняются каждый раз.
5. Transformer hash включает свой исходник, оба lockfile, версии esbuild/
   esbuild-jest, исходник модуля и параметры Jest. Для изменения transformer
   нужен новый процесс Jest (изменения его кода внутри уже запущенного watch
   не обещаются). Тесты инвалидации воспроизводят отдельные процессы.

## Native-ресурсы

`ragflow_deps/native-deps.json` задаёт linux-amd64: pdfium 7809,
pdf_oxide 0.3.67, office_oxide 0.1.2. Версии последних двух проверяются против
`go.mod`; отдельный downloader больше не выбирает 0.3.73/0.1.3 поверх текущего ABI.
SHA-256 архивов сверены с GitHub release asset API; hashes распакованных файлов
получены только из архивов с совпавшими опубликованными digest.

Команды на подготовленном Python 3.13+:

```sh
python3 ragflow_deps/prepare_native.py --download
python3 ragflow_deps/prepare_native.py --check
# Изолированный эксперимент, без изменения пользовательского кеша:
python3 ragflow_deps/prepare_native.py --target-root output/native-experiment
```

Подготовка общая для обоих downloaders. Валидный кеш повторно не распаковывается;
контрольные суммы файлов проверяются при повторном использовании и в linux-amd64
ветке `build.sh`. Остальные платформенные пути не удалены и не объявлены проверенными.
Загрузка проверяет SHA-256 до публикации; распаковка запрещает ссылки/спецфайлы и
выход за целевой каталог. Неполный/чужой кеш отклоняется без перезаписи. Сначала
подготовить отдельный чистый target, затем предметно разобрать повреждённый кеш;
автоматического удаления пользовательских библиотек нет.

Источник pin: GitHub releases `kognitos/pdfium-static` tag `chromium/7809`,
`yfedoseev/pdf_oxide` tag `v0.3.67`, `yfedoseev/office_oxide` tag `v0.1.2`.
При обновлении версии повторно проверить release digest и содержимое; нельзя
просто пересчитать checksum неизвестного локального файла ради успешного gate.

## Docker и границы изменений

Из контекста исключены `output`, frontend coverage, release-каталоги/архивы
`deployment/linux-pg` и browser artifacts. Исходники installer и frontend сохранены.
После дополнительного ревью 2026-09-06 исключён и `.git`: Dockerfile получает
`RAGFLOW_BUILD_VERSION` и `RAGFLOW_SOURCE_REVISION` явно. Общий helper проверяет
формат и пишет VERSION/SOURCE_REVISION; без аргументов версия из pyproject получает
суффикс `-unverified`. Это metadata, не аттестация тестов или чистоты дерева.
Проверка COPY не равна полной сборке образа. Продолжение упаковки — в
[итоговом статусе](process-speedup-status-ru.md).

## Проверки и измерения

В `output/process-speedup-20260905` сохранены снимок с SHA-256, команды, UTC,
wall-clock и логи. Это ignored локальные свидетельства, не Git-артефакты выпуска.
Хост общий; сравнение не является лабораторным benchmark.

| Проверка | Результат |
| --- | --- |
| Focused Jest без cache, три запуска | 10,352 / 10,620 / 10,659 с |
| С cache, первый / два прогретых | 11,005 / 9,069 / 8,524 с |
| Полный Jest с coverage | 22 suites, 161 PASS, 59,098 с; пороги не менялись |
| Native, подготовка / повторное использование | 2,150 / 0,744 / 0,463 с |
| Реальный Lefthook 1.13.6 в отдельном Git repo | Неформатированный TS: FAIL; форматированный: PASS; файлы и staged entries неизменны |
| Реальный Jest, source/config/transformer mutations | Ожидаемые FAIL/PASS, stale cache не скрывает изменение |
| Unit tests подготовки | 8 PASS: hooks, lock, corruption, traversal, конфликт версии, download до публикации |
| Существующий native cache в Ubuntu WSL | Три библиотеки совпали с manifest, проверка read-only |
| Docker exclusion fixture | Все пять артефактов присутствуют до изменения и отсутствуют после; необходимые файлы сохранены |
| COPY всех входов текущего Dockerfile | PASS, 85,623 с; incremental transfer 2,44 MB, COPY `.git` 71,9 с |
| T0 capture tests | 8 PASS; реестр обновлён, unclassified = 0 |

Прогретый focused Jest приблизительно на 15–20% быстрее медианы серии без кеша;
первое заполнение кеша не быстрее. Полный Jest нельзя честно сравнивать с 97 с
первой задачи: там был другой снимок и параллельная нагрузка Docker.
Native verification добавляет чтение файлов относительно прежней проверки
существования каталога; это цена достоверного кеша, а не ускорение этой проверки.
2,44 MB — только повторная передача в существующий BuildKit cache, не новый
полный размер контекста. Поэтому это не доказательство сокращения 8,68 GB
из задачи 1 до мегабайт. Общий объём I/O процесса не профилировался.

Команды регрессии самого изменения:

```sh
python -m unittest test.unit_test.tools.hooks.test_process_preparation -v
node test/unit_test/tools/hooks/jest_cache_probe.cjs
ruff check tools/hooks/prepare_web.py tools/hooks/web_check.py ragflow_deps/prepare_native.py test/unit_test/tools/hooks/test_process_preparation.py
bash -n build.sh
python tools/quality/capture_inventory.py --write
```

Дополнительные проверки 2026-09-06: все 26 COPY-входов без `.git` прошли за
19,155 с; incremental transfer 264,17 kB не является полным размером контекста.
Реальный compile/link через `build.sh --test` в подготовленном Go 1.26.4 контейнере
прошёл три CGO PDF fixture теста; кеш native проверен новым helper. Контейнер
работал без сети, с read-only исходниками и кешем. Это не полный Go regression.
Два REST retry теста больше не спят по 26 секунд: fake clock проверяет те же
пять запросов и интервалы 1/3/7/15 с для HTTP 429/500. Production retry не менялся;
все 61 тест connector прошли за 1,84 с.

Непроверенное: полная чистая Docker-сборка и настоящий GitHub Actions run.
Поэтому полный DoD задачи 2 остаётся INCOMPLETE до этих проверок.

## Происхождение и cleanup

Принятая база: `cb93883f3f8c975eecb2fed81210effeb3bdb06f`.
`.dockerignore`, `build.sh`, `lefthook.yml`, оба workflow, downloaders и
`web/package.json` — core changes: эти стандартные входы нельзя заменить
неподключённым расширением. Transformer и новые helpers/tests/docs — extensions.
Точные пути и содержимое обновляются в T0 registries; это не архитектурный gate.
Применимы BUILD-01, TEST-01, POL-02, UPG-01.

Удалены два install/mutex блока hooks и две отдельные реализации native extraction.
Один владелец проверки/подготовки native, один frontend tool launcher.
App/domain code, runtime profiles, package manager и test selection не менялись.
Хеширование линейно по объёму проверяемых файлов; extraction выполняется лишь
для отсутствующего target и публикует только проверенный каталог.
Сложный общий scheduler и кеш результатов тестов не вводились.
