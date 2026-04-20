import { FormControl, FormItem, FormLabel } from '@/components/ui/form';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
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

  const mode = tableColumnMode === 'manual' ? 'manual' : 'auto';
  const columns: string[] = Array.isArray(tableColumnNames)
    ? tableColumnNames
    : [];

  const handleModeChange = (value: string) => {
    form.setValue(
      'parser_config.table_column_mode',
      value as 'auto' | 'manual',
    );
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
      <FormItem className="space-y-2">
        <FormLabel className="text-sm font-medium">
          {t('tableColumnMode')}
        </FormLabel>
        <FormControl>
          <RadioGroup
            value={mode}
            onValueChange={handleModeChange}
            className="flex gap-4"
          >
            <div className="flex items-center space-x-2">
              <RadioGroupItem value="auto" id="table-mode-auto" />
              <label
                htmlFor="table-mode-auto"
                className="text-sm font-normal cursor-pointer"
              >
                {t('tableColumnModeAuto')}
              </label>
            </div>
            <div className="flex items-center space-x-2">
              <RadioGroupItem value="manual" id="table-mode-manual" />
              <label
                htmlFor="table-mode-manual"
                className="text-sm font-normal cursor-pointer"
              >
                {t('tableColumnModeManual')}
              </label>
            </div>
          </RadioGroup>
        </FormControl>
      </FormItem>

      {mode === 'auto' && (
        <p className="text-sm text-muted-foreground">
          {t('tableColumnModeAutoDescription')}
        </p>
      )}

      {mode === 'manual' && columns.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t('tableColumnRolesEmpty')}
        </p>
      )}

      {mode === 'manual' && columns.length > 0 && (
        <>
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
                    value={
                      (tableColumnRoles && tableColumnRoles[col]) || 'both'
                    }
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
        </>
      )}
    </ConfigurationFormContainer>
  );
}
