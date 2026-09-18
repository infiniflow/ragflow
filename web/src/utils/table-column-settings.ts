/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

export type TableColumnSettings = {
  /** The mode a level states; absent means nothing chose one yet. */
  mode?: 'auto' | 'manual';
  roles: Record<string, 'indexing' | 'metadata' | 'both'>;
  names: string[];
};

// normalizeRole canonicalises a stored role for display only. The comparison is
// exact because the runtime compares exactly: the chunk body and the dataset
// field_map test the stored string with `role in (...)`, without trimming or
// case-folding (rag/app/table.py:701-706, and Go common.NormalizeColumnRole
// ports the same rule), so a value like " Indexing " excludes the column rather
// than honouring it. The dialog must not show a match ingestion will not make.
function normalizeRole(raw: unknown): 'indexing' | 'metadata' | 'both' {
  const role = String(raw ?? '');
  if (role === 'indexing' || role === 'vectorize') {
    return 'indexing';
  }
  if (role === 'metadata') {
    return 'metadata';
  }
  return 'both';
}

function collectRoles(raw: unknown): TableColumnSettings['roles'] {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return {};
  }
  const roles: TableColumnSettings['roles'] = {};
  // Keep every stored key: the lookup is by exact column name
  // (`column_roles.get(col, "both")`, rag/app/table.py:701), and a delimited
  // header is the first record as read (rag/app/table.py:582), which can
  // legitimately name a column "" or " ". Dropping a blank key would hide a
  // role ingestion honours.
  for (const [column, role] of Object.entries(raw as Record<string, unknown>)) {
    roles[column] = normalizeRole(role);
  }
  return roles;
}

// storedMode reads a persisted column mode the way the runtime does: an absent
// or non-string value is unset, and every other value is taken verbatim, because
// only the exact "manual" selects manual
// (internal/common/table_column.go, NormalizeTableColumnMode).
function storedMode(raw: unknown): string {
  return typeof raw === 'string' ? raw : '';
}

// storedNames returns the published schema the runtime keeps: every string
// entry, blank included, because a delimited header can name a column "" and
// that is a real column of the published schema
// (internal/ingestion/task/indexdoc, parseTableColumnNames).
function storedNames(raw: unknown): string[] {
  return Array.isArray(raw)
    ? raw.filter((name): name is string => typeof name === 'string')
    : [];
}

// spreadsheetSetups returns, in ascending component-id order, the spreadsheet
// parameter blocks a canvas or parser dialog saved under a `Parser:<id>` key —
// the shape the backend reads at internal/ingestion/task/indexdoc,
// ResolveTableProfile.
function spreadsheetSetups(config: Record<string, any>): Record<string, any>[] {
  return Object.keys(config)
    .filter((key) => key.startsWith('Parser:'))
    .sort()
    .map((key) => (config[key] as Record<string, any>)?.spreadsheet)
    .filter(
      (spreadsheet): spreadsheet is Record<string, any> =>
        !!spreadsheet && typeof spreadsheet === 'object',
    );
}

/**
 * The table column settings a parse will actually use, resolved out of a
 * dataset's or a document's parser_config.
 *
 * Root-level `table_column_*` keys hold the column intent someone set — at
 * upload or on a parser dialog — and answer as soon as they state a mode or a
 * role. A component's spreadsheet block holds that run's canvas parameters and
 * is consulted only while the root states no intent, which is how a canvas
 * author's column settings reach a document configured nowhere else.
 *
 * Column names are discovery output rather than intent: every successful table
 * run publishes them, so they never decide which level answers — they are
 * resolved on their own, root first. Mirrors ResolveTableProfile and
 * ResolveTableColumnNames in internal/ingestion/task/indexdoc/process.go.
 */
export function resolveTableColumnSettings(
  parserConfig?: Record<string, any> | null,
): TableColumnSettings {
  const config = parserConfig ?? {};
  const components = spreadsheetSetups(config);
  const rootNames = storedNames(config.table_column_names);
  const names =
    rootNames.length > 0
      ? rootNames
      : (components
          .map((spreadsheet) => storedNames(spreadsheet.column_names))
          .find((listed) => listed.length > 0) ?? []);

  const rootMode = storedMode(config.table_column_mode);
  const rootRoles = collectRoles(config.table_column_roles);
  if (rootMode || Object.keys(rootRoles).length > 0) {
    return {
      mode: rootMode === 'manual' ? 'manual' : 'auto',
      roles: rootRoles,
      names,
    };
  }

  for (const spreadsheet of components) {
    const mode = storedMode(spreadsheet.column_mode);
    const roles = collectRoles(spreadsheet.column_roles);
    // A block that states nothing is not a profile: a canvas can carry a
    // spreadsheet block for a component nobody configured.
    if (mode || Object.keys(roles).length > 0) {
      return {
        mode: mode === 'manual' ? 'manual' : 'auto',
        roles,
        names,
      };
    }
  }

  return { roles: {}, names };
}
