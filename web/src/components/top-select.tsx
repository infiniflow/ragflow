import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { SelectWithSearch } from './originui/select-with-search';
import { RAGFlowFormItem } from './ragflow-form';

type TopSelectProps = {
  value?: number;
  onChange?(value: number): void;
};

export function TopSelect({ value = 10, onChange }: TopSelectProps) {
  const { t } = useTranslation();

  const sizeChangerOptions = useMemo(() => {
    return [10, 20, 50, 100].map((x) => ({
      label: <span>{t('common.top', { top: x })}</span>,
      value: x.toString(),
    }));
  }, [t]);

  return (
    <SelectWithSearch
      options={sizeChangerOptions}
      value={value.toString()}
      onChange={(val) => onChange?.(Number(val))}
    ></SelectWithSearch>
  );
}

export function TopSelectFormItem() {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem label={t('knowledgeConfiguration.top')} name="size">
      <TopSelect></TopSelect>
    </RAGFlowFormItem>
  );
}
