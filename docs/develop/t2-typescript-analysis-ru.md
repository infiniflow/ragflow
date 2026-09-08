# T2: широкий наблюдательный анализ TypeScript

Дата обновления: 2026-09-08. Статус: report-only наблюдатель реализован для браузерного приложения `web/src`; это часть T2, а не архитектурный gate, проверка мёртвого кода или завершение T2.

## Запуск

Из корня репозитория в PowerShell:

```powershell
.venv\Scripts\python.exe -B tools/quality/inspect_typescript.py --output output/quality/typescript-analysis.json
.venv\Scripts\python.exe -m pytest test/unit_test/tools/quality/test_inspect_typescript.py -q
node --check tools/quality/inspect_typescript.cjs
```

Профиль `frontend-web` задаётся в `tools/quality/typescript-analysis.yaml`. Он использует установленный и закреплённый в `web/node_modules` TypeScript parser, читает `web/tsconfig.json` и начинает runtime-достижимость от фактического браузерного bootstrap `web/src/main.tsx`. Перед запуском новые локальные пути инструмента должны быть классифицированы свежим T0.

Код выхода 0 означает только `OBSERVED`: граф собран без известных пробелов. Код 2 и `INCOMPLETE` означает, что вычисляемая загрузка, ошибка разбора/разрешения, отсутствие Node/parser либо изменение входного снимка не позволяют считать граф полным. `policy_status` всегда `NOT_EVALUATED`, `dead_code_status` — `NOT_ANALYZED`. Статусы `OBSERVED` и `INCOMPLETE` не являются `PASS` ARC-01/02/03 или DEAD-01.

## Что анализируется

- Все `.ts` и `.tsx` под `web/src`, включая tests и declarations для индекса и обратных потребителей. Tests и `.d.ts` не становятся browser runtime roots.
- Статические imports и re-exports, `import =`, type-only imports, литеральные `import()` и `require()`, импортированные symbols, module/function phase и условность.
- Локальные относительные пути, aliases `@` и `@parent`, assets и query suffixes с разрешением через compiler options. Внешние пакеты остаются внешними и не обходятся транзитивно.
- Достижимость от `web/src/main.tsx`, прямые обратные потребители локально изменённых/добавленных файлов и strongly connected components только по eager module-initialization edges. Ленивые `import()` не создают cycle signal.
- `import.meta.glob`, `import.meta.globEager` и `require.context`: литеральные patterns записываются как registration evidence, вычисляемые аргументы делают анализ неполным.
- Вычисляемые `import()`/`require()`, `eval`, `Function`, parse errors и локальные цели вне профиля записываются как explicit incomplete reasons.

Наблюдатель не разворачивает glob patterns, не выполняет Vite transforms, decorators, browser code или package side effects и не строит call graph. Достижимый статический import показывает возможную загрузку модуля, а не выполнение каждого export. Отсутствие прямого потребителя или пути от одного browser entrypoint не доказывает dead code: Storybook, тесты, workers, persisted names, assets, внешние consumers и другие entrypoints требуют отдельной проверки.

Цикл в отчёте — сигнал для разбора ARC-03, не готовая находка: необходимо отделить upstream от собственного кода, проверить runtime и допустимый registration contract. Экспорты инвентаризируются, но их необходимость и публичность автоматически не оцениваются.

## Проверки реализации

Девять фикстур проверяют runtime/type-only/lazy imports, re-exports и assets; aliases и exports; литеральный glob; eager cycle; reverse consumers; отсутствие ложного dead-code verdict; вычисляемую загрузку и неразрешённую локальную цель; выход за source profile; отсутствие Node; защиту T0 provenance, output path, исходных байтов и конкурентных изменений. Worker дополнительно проходит `node --check`, Python wrapper — Ruff.

Инструмент проверяет SHA всех входных TypeScript-файлов, policy, Python wrapper и Node worker, а также HEAD, upstream base и T0 snapshot fingerprint. Отчёт разрешён только в игнорируемом пути репозитория и записывается лишь после повторной проверки исходных байтов; исходники он не меняет.

## Выполненный прогон

Текущий профиль проиндексировал **1 178** файлов: 1 152 runtime-файла, 8 592 import edges, из них 5 698 локальных runtime edges и 5 610 eager module-initialization edges. От `web/src/main.tsx` статически достижимы 1 067 runtime-файлов; 85 не достигнуты. В T0 к локальным `core_change`/`extension` относятся 242 файла, у 28 нет прямого import consumer в индексе, а 12 собственных runtime-путей не достигнуты от выбранного entrypoint. Все эти числа — inventory для ревью, не список на удаление.

Найдено 11 достижимых eager strongly connected components. Они сохранены с путями и составом владельцев, но не классифицированы как нарушения ARC-03. Литеральная регистрация `import.meta.glob("@/assets/svg/**/*.svg")` в `web/src/components/svg-icon.tsx` записана отдельно и не скрыта за обычным import edge.

Итог текущего запуска — `INCOMPLETE / NOT_EVALUATED / NOT_ANALYZED`, код выхода 2. Единственный известный runtime-пробел — намеренно вычисляемый `import(/* @vite-ignore */ name)` в `web/src/hooks/common-hooks.tsx:64`. Наблюдатель не подменяет его придуманным списком целей и не создаёт исключение. Актуальный локальный отчёт: `output/quality/typescript-analysis.json`.

[Широкий Go observer](t2-go-analysis-ru.md) и [объединяющий runtime-graph classifier](t2-runtime-graph-classification-ru.md) реализованы. Все 11 текущих циклов подтверждены как topology принятой upstream-базы по source/target/specifier и AST-семантике `kind/phase/type_only`: шесть полностью upstream, пять содержат локально изменённые файлы, но не новое ребро. Type-only или call-phase upstream import не считается доказательством текущего eager module edge. Все 12 owned unreachable runtime paths классифицированы exact-записями: три type-only contracts и девять static candidates без разрешения на удаление. Browser, Storybook/Jest и PDF worker roots также разведены. T2 report-only срез завершён; CI integration и policy enforcement относятся к T3.
