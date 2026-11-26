import { HomeCard } from '@/components/home-card';
import { Button } from '@/components/ui/button';
import { IFlowTemplate } from '@/interfaces/database/agent';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

interface IProps {
  data: IFlowTemplate;
  language: keyof IFlowTemplate['title'];
  showModal(record: IFlowTemplate): void;
}

export function TemplateCard({ data, language, showModal }: IProps) {
  const { t } = useTranslation();

  const handleClick = useCallback(() => {
    showModal(data);
  }, [data, showModal]);

  return (
    <HomeCard
      testId="template-card"
      data={{
        name: data.title[language],
        description: data.description[language],
        avatar: data.avatar,
      }}
      moreDropdown={
        <Button
          size="sm"
          className="pointer-events-none opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
          onClick={handleClick}
        >
          {t('flow.useTemplate')}
        </Button>
      }
    />
  );
}
