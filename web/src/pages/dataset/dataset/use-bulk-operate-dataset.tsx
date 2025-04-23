import { Ban, CircleCheck, CircleX, Play, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export function useBulkOperateDataset() {
  const { t } = useTranslation();

  const list = [
    {
      id: 'enabled',
      label: t('knowledgeDetails.enabled'),
      icon: <CircleCheck />,
      onClick: () => {},
    },
    {
      id: 'disabled',
      label: t('knowledgeDetails.disabled'),
      icon: <Ban />,
      onClick: () => {},
    },
    {
      id: 'run',
      label: t('knowledgeDetails.run'),
      icon: <Play />,
      onClick: () => {},
    },
    {
      id: 'cancel',
      label: t('knowledgeDetails.cancel'),
      icon: <CircleX />,
      onClick: () => {},
    },
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <Trash2 />,
      onClick: () => {},
    },
  ];

  return { list };
}
