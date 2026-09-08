# T2: классификация TypeScript/Go runtime-графов

Дата обновления: 2026-09-08. Статус: объединяющий report-only срез T2 реализован. Он классифицирует сигналы двух широких наблюдателей на одном T0 snapshot, но не объявляет архитектуру, dead code, сборку или CI пройденными.

## Запуск

Сначала обновить T0 и сформировать оба исходных отчёта:

```powershell
python tools/quality/capture_inventory.py --write
python tools/quality/inspect_typescript.py --output output/quality/typescript-analysis.json
python tools/quality/inspect_go.py --output output/quality/go-analysis.json
python tools/quality/check_runtime_graph_policy.py --output output/quality/runtime-graph-classification.json
```

TypeScript observer сейчас ожидаемо возвращает код 2 из-за одного вычисляемого импорта. Это не отменяет отчёт: классификатор принимает только точный `INCOMPLETE`-сигнал из неизменённого upstream-файла и сохраняет исходный статус, а неизвестный или локально изменённый пробел отклоняет.

Правила классификации находятся в `tools/quality/runtime-graph-policy.yaml`. Выход разрешён только в игнорируемом пути. Инструмент сверяет HEAD, принятую upstream-базу и T0 fingerprint обоих отчётов, SHA входов и неизменность рабочего снимка до записи результата.

## Что считается корнем

- Production frontend root — точный bootstrap `web/src/main.tsx`, подтверждённый `web/index.html`.
- Storybook и Jest записаны как development/test loaders, а не production reachability roots.
- `/pdfjs-dist/pdf.worker.min.js` записан как внешний runtime asset с существующей целью в `web/public`; это не TypeScript entrypoint.
- Production Go roots — `cmd/ragflow_server.go` и `cmd/ragflow-cli.go`, подтверждённые точными командами `build.sh`. Один server binary выбирает API/admin/ingestor режимы флагами.
- `tools/gen-component-parity/main.go` и `tools/migrate-canvas/main.go` классифицированы как самостоятельные developer tools. Любой новый неклассифицированный `package main` делает результат неполным.

## Классификация текущих сигналов

Одиннадцать достижимых eager TypeScript SCC проверяются относительно принятого upstream commit по source/target/specifier и семантике AST-ребра: `kind`, `phase`, `type_only`. Upstream-файл разбирается тем же зафиксированным TypeScript parser, поэтому type-only, lazy/call-phase и eager module imports не взаимозаменяемы; отсутствие Node/parser или синтаксическая ошибка закрывают проверку как `INCOMPLETE`. Все текущие рёбра уже существовали в upstream-базе с той же семантикой: шесть циклов полностью upstream, пять имеют локально изменённые файлы, но не новое локальное ребро. Это предметный долг на затронутых границах, не разрешение добавлять новые циклы. Любое ребро цикла, отсутствующее в принятой базе или отличающееся по семантике, остаётся `unclassified_local_cycle_candidate`.

Все 12 недостижимых собственных runtime-путей перечислены точно, без wildcard:

- три файла (`business-documents/types.ts`, `openmetadata/types.ts`, `skills/types.ts`) имеют только type-only consumers и классифицированы как compile-time contracts;
- девять путей получили `static_candidate`: у шести нет ни runtime-, ни type-only consumer, chunk-card достижим только из недостижимого ChunkerContainer, а два link-data-pipeline файла образуют изолированную пару.

`static_candidate` означает только очередь на отдельную проверку. Удаление требует поиска Storybook/генераторов/внешних imports, браузерного поведения, saved Canvas/pipeline data и соответствующих contracts. Автоматических удалений и apply-режима нет. Ставшая устаревшей или отсутствующая запись классификации также считается ошибкой, поэтому файл нельзя молча спрятать в бессрочном baseline.

Литеральный `import.meta.glob` SVG закреплён как Vite asset registration. Единственный вычисляемый import находится в неизменённом upstream helper `useDynamicSVGImport`, у которого нет найденных consumers; он остаётся `inherited_upstream_dynamic_gap` с обязательным manual review при изменении helper или потребителей. Это не превращает исходный TypeScript `INCOMPLETE` в `PASS`.

Go-графы не содержат package cycles и недостижимых собственных packages. Семь `go:embed` targets проверяются на существование и классифицируются как runtime assets; `go:generate` — developer directive, `go:linkname` в `_test.go` — test-only directive. Компилятор, типы и linkname target этим не проверяются.

## Результат и коды выхода

Текущий результат — `classification_status=COMPLETE`, `policy_status=REPORT_ONLY`, `architecture_status=NOT_EVALUATED`, `dead_code_status=REVIEW_REQUIRED`: 11 inherited TypeScript cycles, 9 static candidates, 1 классифицированный source gap, 0 Go package cycles и 0 неклассифицированных findings.

Код 0 означает только полноту классификации текущего набора сигналов. Код 2 означает stale/mismatched observer, новый или устаревший собственный недостижимый path, локальный cycle candidate, неизвестный dynamic/directive/main root, изменившийся upstream gap, отсутствующее root evidence/embed asset либо конкурентное изменение снимка.

Четырнадцать unit/negative fixtures проверяют допустимый набор; новое локальное ребро цикла; несовпадение type-only и call-phase upstream edge с eager module edge; fail-closed при отсутствии TypeScript parser; пропущенную и устаревшую запись пути; ложный type-only verdict; неизвестную регистрацию и source gap; изменение upstream gap на owned; новый Go main; отсутствующий embed asset; изменение root evidence; report-only схему и устойчивую сигнатуру цикла.

## Граница T3

Файл policy задаёт три будущих selector scope: owned TypeScript runtime graph, owned Go package graph и целостность runtime roots. Они остаются `report_only`: T2 не создаёт baseline/ignore и не меняет CI. На T3 нужно подключить неблокирующий aggregate job, проверить selector отрицательными PR-фикстурами, затем отдельно принять блокирующий режим и branch protection.
