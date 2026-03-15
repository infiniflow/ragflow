import { FormControl, FormItem, FormLabel } from '@/components/ui/form';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext, useWatch } from 'react-hook-form';
import { ConfigurationFormContainer } from '../configuration-form-container';

const ROLE_OPTIONS = [
  { value: 'both', labelKey: 'tableColumnRoleBoth' },
  { value: 'vectorize', labelKey: 'tableColumnRoleVectorize' },
  { value: 'metadata', labelKey: 'tableColumnRoleMetadata' },
] as const;

export function TableConfiguration() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeConfiguration');

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

  const columns: string[] = Array.isArray(tableColumnNames)
    ? tableColumnNames
    : [];

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

  if (columns.length === 0) {
    return (
      <ConfigurationFormContainer>
        <p className="text-sm text-muted-foreground">
          {t('tableColumnRolesEmpty')}
        </p>
      </ConfigurationFormContainer>
    );
  }

  return (
    <ConfigurationFormContainer>
      <p className="text-sm text-muted-foreground mb-3">
        {t('tableColumnRolesTip')}
      </p>
      <div className="space-y-3">
        {columns.map((col) => (
          <FormItem key={col} className="flex flex-row items-center gap-4">
            <FormLabel className="min-w-[120px] shrink-0 text-sm font-normal">
              {col}
            </FormLabel>
            <FormControl>
              <Select
                value={(tableColumnRoles && tableColumnRoles[col]) || 'both'}
                onValueChange={(value) => handleRoleChange(col, value)}
              >
                <SelectTrigger className="w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ROLE_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {t(opt.labelKey)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormControl>
          </FormItem>
        ))}
      </div>
      <p className="text-xs text-muted-foreground mt-3">
        {t('tableColumnRolesReparseTip')}
      </p>
    </ConfigurationFormContainer>
  );
}
