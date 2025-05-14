import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { DocumentParserType } from '@/constants/knowledge';
import { useMemo, useState } from 'react';
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
import { useFetchKnowledgeConfigurationOnMount } from './hooks';

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
  const [finalParserId, setFinalParserId] = useState<DocumentParserType>(
    DocumentParserType.Naive,
  );

  const knowledgeDetails = useFetchKnowledgeConfigurationOnMount(form);

  const parserId: DocumentParserType = useWatch({
    control: form.control,
    name: 'parser_id',
  });

  const ConfigurationComponent = useMemo(() => {
    return finalParserId
      ? ConfigurationComponentMap[finalParserId]
      : EmptyComponent;
  }, [finalParserId]);

  //   useEffect(() => {
  //     setFinalParserId(parserId);
  //   }, [parserId]);

  //   useEffect(() => {
  //     setFinalParserId(knowledgeDetails.parser_id as DocumentParserType);
  //   }, [knowledgeDetails.parser_id]);

  return (
    <section>
      <ConfigurationComponent></ConfigurationComponent>
    </section>
  );
}
