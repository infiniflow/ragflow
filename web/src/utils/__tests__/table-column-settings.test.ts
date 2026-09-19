import { resolveTableColumnSettings } from '../table-column-settings';

describe('resolveTableColumnSettings', () => {
  it('prefers root-level user settings', () => {
    expect(
      resolveTableColumnSettings({
        table_column_mode: 'manual',
        table_column_roles: { a: 'vectorize', b: 'nonsense' },
        'Parser:Table': {
          spreadsheet: { column_mode: 'auto', column_roles: { a: 'both' } },
        },
      }),
    ).toEqual({
      mode: 'manual',
      roles: { a: 'indexing', b: 'both' },
      names: [],
    });
  });

  it('falls back to the component-shaped entry', () => {
    expect(
      resolveTableColumnSettings({
        'Parser:Table': {
          spreadsheet: {
            column_mode: 'manual',
            column_roles: { a: 'metadata' },
          },
        },
      }),
    ).toEqual({ mode: 'manual', roles: { a: 'metadata' }, names: [] });
  });

  // The runtime compares both values verbatim: only the exact "manual" selects
  // manual (common.NormalizeTableColumnMode), and a role entry is keyed by the
  // column name as stored. So a padded name is still a column, and a role that
  // merely resembles one of the three is not one: it resolves to the default,
  // which is the only value a dialog can offer for it.
  it('reads a stored mode and stored roles exactly', () => {
    expect(
      resolveTableColumnSettings({ table_column_mode: ' Manual ' }),
    ).toEqual({ mode: 'auto', roles: {}, names: [] });

    expect(
      resolveTableColumnSettings({
        table_column_roles: { ' Indexing ': ' INDEXING ', '': 'metadata' },
      }),
    ).toEqual({
      mode: 'auto',
      roles: { ' Indexing ': 'both', '': 'metadata' },
      names: [],
    });
  });

  // Published column names are discovery output, not intent: every successful
  // table run writes them, so they must not shadow a canvas profile that the
  // user can still change after the first parse
  // (indexdoc.ResolveTableProfile).
  it('does not treat published column names as a setting', () => {
    expect(
      resolveTableColumnSettings({
        table_column_names: ['a'],
        'Parser:Table': {
          spreadsheet: {
            column_mode: 'manual',
            column_roles: { a: 'metadata' },
          },
        },
      }),
    ).toEqual({
      mode: 'manual',
      roles: { a: 'metadata' },
      names: ['a'],
    });
  });

  // A blank name is still a column of the schema, so the published list keeps
  // it verbatim (indexdoc.parseTableColumnNames).
  it('carries a blank published column name', () => {
    expect(
      resolveTableColumnSettings({
        table_column_names: [''],
        table_column_roles: { '': 'metadata' },
      }),
    ).toEqual({ mode: 'auto', roles: { '': 'metadata' }, names: [''] });
  });

  it('resolves names from the first component that states them', () => {
    expect(
      resolveTableColumnSettings({
        'Parser:A': { spreadsheet: { column_names: ['a'] } },
        'Parser:B': { spreadsheet: { column_names: ['b'] } },
      }).names,
    ).toEqual(['a']);
  });

  // A component entry that states nothing is not a profile either: a canvas
  // can carry an empty spreadsheet block, and the resolution has to reach the
  // component that does declare one.
  it('skips a component entry that states nothing', () => {
    expect(
      resolveTableColumnSettings({
        'Parser:A': { spreadsheet: {} },
        'Parser:B': {
          spreadsheet: {
            column_mode: 'manual',
            column_roles: { a: 'metadata' },
          },
        },
      }),
    ).toEqual({ mode: 'manual', roles: { a: 'metadata' }, names: [] });
  });

  // An unconfigured parser states no mode at all, so a dialog save does not
  // write an "auto" that would outrank a profile configured elsewhere.
  it('states no mode when nothing is configured', () => {
    expect(resolveTableColumnSettings(undefined)).toEqual({
      roles: {},
      names: [],
    });
    expect(resolveTableColumnSettings({})).toEqual({ roles: {}, names: [] });
    expect(
      resolveTableColumnSettings({
        'Parser:Table': { spreadsheet: { output_format: 'markdown' } },
      }),
    ).toEqual({ roles: {}, names: [] });
  });
});
