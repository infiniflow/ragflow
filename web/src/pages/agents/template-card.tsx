import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { IFlowTemplate } from '@/interfaces/database/agent';
import i18n from '@/locales/config';
import { useCallback, useMemo } from 'react';
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

  const language = useMemo(() => {
    return i18n.language || 'en';
  }, []) as 'en' | 'zh' | 'de';

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
            className="w-1/3 absolute bottom-4 right-4 left-4 justify-center text-center m-auto"
            onClick={handleClick}
          >
            {t('flow.useTemplate')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
