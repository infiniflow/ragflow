import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { LanguageAbbreviation } from '@/constants/common';
import { IFlowTemplate } from '@/interfaces/database/agent';
import i18n from '@/locales/config';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

interface IProps {
  data: IFlowTemplate;
  isCreate?: boolean;
  showModal(record: IFlowTemplate): void;
}

export function TemplateCard({ data, showModal }: IProps) {
  const { t } = useTranslation();

  const handleClick = useCallback(() => {
    showModal(data);
  }, [data, showModal]);

  const language = (
    i18n.language === LanguageAbbreviation.Zh
      ? 'zh'
      : i18n.language === LanguageAbbreviation.De
        ? 'de'
        : 'en'
  ) as 'en' | 'zh' | 'de';

  return (
    <Card className="border-colors-outline-neutral-standard group relative min-h-40">
      <CardContent className="p-4 ">
        <div className="flex justify-start items-center gap-4 mb-4">
          <RAGFlowAvatar
            className="w-7 h-7"
            avatar={data.avatar ? data.avatar : 'https://github.com/shadcn.png'}
            name={data?.title[language] || 'CN'}
          ></RAGFlowAvatar>
          <div
            className="text-[18px] font-bold break-words hyphens-auto overflow-hidden"
            lang={language}
          >
            {data?.title[language]}
          </div>
        </div>
        <p className="break-words hypens-auto" lang={language}>
          {data?.description[language]}
        </p>
        <div className="group-hover:bg-gradient-to-t from-black/70 from-10% via-black/0 via-50% to-black/0 w-full h-full group-hover:block absolute top-0 left-0 hidden rounded-xl">
          <Button
            variant="default"
            onClick={handleClick}
            className="absolute bottom-4 left-4 right-4 mx-auto px-4 py-2 max-w-[280px] whitespace-normal text-center"
          >
            <span className="inline-block">{t('flow.useTemplate')}</span>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
