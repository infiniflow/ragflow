import { Button } from '@/components/ui/button';
import { Loader2Icon } from 'lucide-react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { DocumentParserType } from '@/constants/knowledge';
import { useUpdateKnowledge } from '@/hooks/knowledge-hooks';
import { useMemo } from 'react';
import { useParams } from 'umi';
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
  // const [submitLoading, setSubmitLoading] = useState(false); // submit button loading
  const { id: kb_id } = useParams();

  const { saveKnowledgeConfiguration, loading: submitLoading } =
    useUpdateKnowledge();

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
        <Button
          disabled={submitLoading}
          onClick={() => {
            (async () => {
              try {
                let beValid = await form.formControl.trigger();
                if (beValid) {
                  // setSubmitLoading(true);
                  // let postData = form.formState.values;
                  // console.log('submit form -->', form);
                  // delete postData['avatar']; // has submitted in first form general
                  form.handleSubmit(async (values) => {
                    console.log('saveKnowledgeConfiguration: ', values);
                    delete values['avatar'];
                    await saveKnowledgeConfiguration({
                      kb_id,
                      ...values,
                    });
                  })();
                }
              } catch (e) {
                console.log(e);
              } finally {
                // setSubmitLoading(false);
              }
            })();
          }}
        >
          {submitLoading && <Loader2Icon className="animate-spin" />}
          {t('knowledgeConfiguration.save')}
        </Button>
      </div>
    </section>
  );
}
