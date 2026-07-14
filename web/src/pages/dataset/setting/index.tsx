import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { IConnector } from '@/interfaces/database/dataset';
import { useDataSourceInfo } from '@/pages/user-setting/data-source/constant';
import { IDataSourceBase } from '@/pages/user-setting/data-source/interface';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import LinkDataSource, {
  IDataSourceNodeProps,
} from './components/link-data-source';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import { useFetchDatasetSettingOnMount } from './hooks';

export default function DatasetSetting() {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: '',
      description: '',
      avatar: null,
      permission: '',
      embedding_model: '',
      pagerank: 0,
      connectors: [],
    },
  });

  const { dataSourceInfo } = useDataSourceInfo();
  const { knowledgeDetails, loading: datasetSettingLoading } =
    useFetchDatasetSettingOnMount(form);
  const [sourceData, setSourceData] = useState<IDataSourceNodeProps[]>();

  useEffect(() => {
    if (knowledgeDetails) {
      const source_data: IDataSourceNodeProps[] = (
        knowledgeDetails?.connectors ?? []
      ).map((connector: IConnector) => {
        return {
          ...connector,
          icon:
            dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
              ?.icon || '',
        };
      });

      setSourceData(source_data);
    }
  }, [knowledgeDetails, dataSourceInfo]);

  async function onSubmit(data: z.infer<typeof formSchema>) {
    console.log('Form validation passed, submit data', data);
  }

  const handleLinkOrEditSubmit = (data: IDataSourceBase[] | undefined) => {
    if (data) {
      const connectors = data.map((connector) => {
        return {
          ...(connector as IConnector),
          auto_parse: (connector as IConnector).auto_parse === '0' ? '0' : '1',
          icon:
            dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
              ?.icon || '',
        };
      });
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
    }
  };

  const unbindFunc = (data: IDataSourceNodeProps) => {
    if (data) {
      const connectors = sourceData?.filter((connector) => {
        return connector.id !== data.id;
      });
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
    }
  };

  const handleAutoParse = ({
    source_id,
    isAutoParse,
  }: {
    source_id: string;
    isAutoParse: boolean;
  }) => {
    if (source_id) {
      const connectors = sourceData?.map((connector) => {
        if (connector.id === source_id) {
          return {
            ...connector,
            auto_parse: isAutoParse ? '1' : '0',
          };
        }
        return connector;
      });
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
    }
  };

  return (
    <div className="pr-5 pb-5">
      <Card className="p-0 h-full flex flex-col bg-transparent shadow-none">
        <CardHeader className="p-5 border-b-0.5 border-border-button">
          <header>
            <CardTitle as="h1">
              {t('knowledgeDetails.nextConfiguration')}
            </CardTitle>
            <CardDescription>
              {t('knowledgeConfiguration.titleDescription')}
            </CardDescription>
          </header>
        </CardHeader>

        <CardContent className="p-0 flex-1 h-0 flex">
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              className="flex flex-col"
            >
              <div className="flex-1 h-0 w-[768px] px-5 pt-5 overflow-y-auto scrollbar-auto">
                <section className="space-y-5 text-text-secondary">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <GeneralForm></GeneralForm>

                  <Divider />
                  <LinkDataSource
                    data={sourceData}
                    handleLinkOrEditSubmit={handleLinkOrEditSubmit}
                    unbindFunc={unbindFunc}
                    handleAutoParse={handleAutoParse}
                  />
                </section>
              </div>

              <div className="p-5 text-right items-center flex justify-end gap-3 w-[768px]">
                <Button
                  type="reset"
                  variant="transparent"
                  onClick={() => {
                    form.reset();
                  }}
                >
                  {t('knowledgeConfiguration.cancel')}
                </Button>

                <Button type="submit" disabled={datasetSettingLoading}>
                  {t('knowledgeConfiguration.save')}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
