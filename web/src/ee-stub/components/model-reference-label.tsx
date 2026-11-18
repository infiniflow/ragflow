import LLMLabel from '@/components/llm-select/llm-label';

type ModelReferenceLabelProps = {
  value?: string;
  ownerTenantId?: string;
};

export function ModelReferenceLabel(props: ModelReferenceLabelProps) {
  return <LLMLabel {...props} />;
}
