import { DelimiterFormField } from '@/components/delimiter-form-field';
import { EntityTypesFormField } from '@/components/entity-types-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';

export function KnowledgeGraphConfiguration() {
  return (
    <>
      <>
        <EntityTypesFormField></EntityTypesFormField>
        <MaxTokenNumberFormField max={8192 * 2}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
      </>
    </>
  );
}
