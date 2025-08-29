import { Button } from '@/components/ui/button';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { DocumentParserType } from '@/constants/knowledge';
import { useMemo } from 'react';
import { AudioConfiguration } from './configuration/audio';
import { BookConfiguration } from './configuration/book';
import { EmailConfiguration } from './configuration/email';
import { KnowledgeGraphConfiguration } from './configuration/knowledge-graph';
import { LawsConfiguration } from './configuration/laws';
import { ManualConfiguration } from './configuration/manual';
import { NaiveConfiguration } from './configuration/naive';
import { OneConfiguration } from './configuration/one';
import { PaperConfiguration } from './configuration/paper';
import { PictureConfiguration } from './configuration/picture';
import { PresentationConfiguration } from './configuration/presentation';
import { QAConfiguration } from './configuration/qa';
import { ResumeConfiguration } from './configuration/resume';
import { TableConfiguration } from './configuration/table';
import { TagConfiguration } from './configuration/tag';
import { SavingButton } from './saving-button';

const ConfigurationComponentMap = {
  [DocumentParserType.Naive]: NaiveConfiguration,
  [DocumentParserType.Qa]: QAConfiguration,
  [DocumentParserType.Resume]: ResumeConfiguration,
  [DocumentParserType.Manual]: ManualConfiguration,
  [DocumentParserType.Table]: TableConfiguration,
  [DocumentParserType.Paper]: PaperConfiguration,
  [DocumentParserType.Book]: BookConfiguration,
  [DocumentParserType.Laws]: LawsConfiguration,
  [DocumentParserType.Presentation]: PresentationConfiguration,
  [DocumentParserType.Picture]: PictureConfiguration,
  [DocumentParserType.One]: OneConfiguration,
  [DocumentParserType.Audio]: AudioConfiguration,
  [DocumentParserType.Email]: EmailConfiguration,
  [DocumentParserType.Tag]: TagConfiguration,
  [DocumentParserType.KnowledgeGraph]: KnowledgeGraphConfiguration,
};

function EmptyComponent() {
  return <div></div>;
}

export function ChunkMethodForm() {
  const form = useFormContext();
  const { t } = useTranslation();

  const finalParserId: DocumentParserType = useWatch({
    control: form.control,
    name: 'parser_id',
  });

  const ConfigurationComponent = useMemo(() => {
    return finalParserId
      ? ConfigurationComponentMap[finalParserId]
      : EmptyComponent;
  }, [finalParserId]);

  return (
    <section className="h-full flex flex-col">
      <div className="overflow-auto flex-1 min-h-0">
        <ConfigurationComponent></ConfigurationComponent>
      </div>
      <div className="text-right pt-4 flex justify-end gap-3">
        <Button
          type="reset"
          className="bg-transparent text-color-white hover:bg-transparent border-gray-500 border-[1px]"
          onClick={() => {
            form.reset();
          }}
        >
          {t('knowledgeConfiguration.cancel')}
        </Button>
        <SavingButton></SavingButton>
      </div>
    </section>
  );
}
