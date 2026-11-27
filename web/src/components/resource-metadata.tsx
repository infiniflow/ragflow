import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

type ResourceMetadataItem = {
  key: string;
  content: ReactNode;
};

type ResourceMetadataProps = {
  createTime?: string;
  items?: ResourceMetadataItem[];
};

export function ResourceMetadata({
  createTime,
  items = [],
}: ResourceMetadataProps) {
  const { t } = useTranslation();

  if (!createTime && items.length === 0) {
    return null;
  }

  return (
    <div className="min-w-0 truncate text-xs text-text-secondary">
      {createTime && (
        <span className="mr-3">
          {t('common.createdAt')} {createTime}
        </span>
      )}
      {items.map((item) => (
        <span key={item.key} className="mr-3 last:mr-0">
          {item.content}
        </span>
      ))}
    </div>
  );
}
