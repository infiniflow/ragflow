import { IProviderInstance } from '@/interfaces/database/llm';
import { ProviderInstanceCard } from '@/pages/user-setting/setting-model/instance-card/provider-instance-card';

type ProviderInstanceCardWithPrivilegeProps = {
  providerName: string;
  instance: IProviderInstance;
  defaultOpen: boolean;
};

/**
 * Enterprise seam around a provider instance card. Open source has no
 * collaborator permissions, so the card renders unchanged.
 */
export function ProviderInstanceCardWithPrivilege({
  providerName,
  instance,
  defaultOpen,
}: ProviderInstanceCardWithPrivilegeProps) {
  return (
    <ProviderInstanceCard
      providerName={providerName}
      instance={instance}
      defaultOpen={defaultOpen}
    />
  );
}
