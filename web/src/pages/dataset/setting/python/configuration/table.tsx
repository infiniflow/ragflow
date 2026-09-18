import { TableColumnSettingsFields } from '@/components/table-column-settings-form-fields';
import { useFormContext, useWatch } from 'react-hook-form';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { GlobalIndexModelItem } from './common-item';

export function TableConfiguration() {
  const form = useFormContext();

  const tableColumnMode = useWatch({
    control: form.control,
    name: 'parser_config.table_column_mode',
    defaultValue: 'auto',
  });
  const tableColumnNames = useWatch({
    control: form.control,
    name: 'parser_config.table_column_names',
    defaultValue: [],
  });
  const tableColumnRoles = useWatch({
    control: form.control,
    name: 'parser_config.table_column_roles',
    defaultValue: {},
  });

  const handleModeChange = (value: 'auto' | 'manual') => {
    form.setValue('parser_config.table_column_mode', value);
  };

  const handleRoleChange = (columnName: string, role: string) => {
    const current =
      (form.getValues('parser_config.table_column_roles') as Record<
        string,
        string
      >) || {};
    form.setValue('parser_config.table_column_roles', {
      ...current,
      [columnName]: role,
    });
  };

  return (
    <ConfigurationFormContainer>
      <GlobalIndexModelItem />
      <TableColumnSettingsFields
        idPrefix="dataset"
        mode={tableColumnMode}
        columns={tableColumnNames}
        roles={tableColumnRoles}
        onModeChange={handleModeChange}
        onRoleChange={handleRoleChange}
      />
    </ConfigurationFormContainer>
  );
}
