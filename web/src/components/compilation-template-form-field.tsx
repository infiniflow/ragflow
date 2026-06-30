import { RAGFlowFormItem } from '@/components/ragflow-form';
import {
  MultiSelect,
  MultiSelectOptionType,
} from '@/components/ui/multi-select';
import { useFetchAllCompilationTemplateGroups } from '@/hooks/use-compilation-template-group-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type CompilationTemplateFormFieldProps = {
  horizontal?: boolean;
};

const ScopeTranslationKeyMap: Record<string, string> = {
  file: 'scopeFile',
  dataset: 'scopeDataset',
};

type CompilationTemplateMultiSelectProps = {
  value?: string[];
  onChange(value: string[]): void;
};

/**
 * Renders a multi-select for compilation template groups.
 *
 * Selection rule:
 * - Each group has a scope: "file" or "dataset".
 * - At most one group per scope can be selected.
 * - The two scopes can be combined (one file group + one dataset group).
 * - Once a scope is selected, remaining options of the same scope are disabled.
 * - The scope is displayed as a suffix after each option label.
 */
function CompilationTemplateMultiSelect({
  value: rawValue = [],
  onChange,
}: CompilationTemplateMultiSelectProps) {
  const { t } = useTranslation();
  const { groups } = useFetchAllCompilationTemplateGroups();

  const value = useMemo(() => {
    // Normalize legacy single-string values into an array during the migration.
    if (Array.isArray(rawValue)) return rawValue;
    if (typeof rawValue === 'string' && rawValue) return [rawValue];
    return [];
  }, [rawValue]);

  // Collect scopes that are already selected by the current value.
  const selectedScopes = useMemo(() => {
    const scopes = new Set<string>();
    value.forEach((id) => {
      const group = groups?.find((g) => g.id === id);
      if (group?.scope) {
        scopes.add(group.scope);
      }
    });
    return scopes;
  }, [value, groups]);

  const options = useMemo<MultiSelectOptionType[]>(() => {
    return (groups ?? []).map((group) => {
      const scopeTranslationKey = group.scope
        ? ScopeTranslationKeyMap[group.scope]
        : undefined;
      // Disable other options that share an already-selected scope.
      const isSameScopeSelected =
        !!group.scope &&
        selectedScopes.has(group.scope) &&
        !value.includes(group.id);

      return {
        label: group.name,
        value: group.id,
        disabled: isSameScopeSelected,
        suffix: scopeTranslationKey ? (
          <span className="text-text-secondary ml-1">
            ({t(`knowledgeConfiguration.${scopeTranslationKey}`)})
          </span>
        ) : undefined,
      };
    });
  }, [groups, selectedScopes, value, t]);

  return (
    <MultiSelect
      options={options}
      onValueChange={onChange}
      defaultValue={value}
      value={value}
      showSelectAll={false}
    />
  );
}

export function CompilationTemplateFormField({
  horizontal,
}: CompilationTemplateFormFieldProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem
      name="parser_config.compilation_template_group_id"
      label={t('knowledgeConfiguration.compilationTemplate')}
      className="pb-4"
      horizontal={horizontal}
    >
      {(field) => (
        <CompilationTemplateMultiSelect
          value={field.value ?? []}
          onChange={field.onChange}
        />
      )}
    </RAGFlowFormItem>
  );
}
