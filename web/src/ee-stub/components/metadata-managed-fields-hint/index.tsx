import type { MetadataType } from '@/pages/dataset/components/metedata/constant';

/**
 * The metadata modal mounts this slot with the same props in every build;
 * nothing is rendered here.
 */
export const MetadataManagedFieldsHint = (_props: {
  metadataType?: MetadataType;
  doc?: Record<string, any>;
}) => {
  return null;
};
