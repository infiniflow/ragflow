import {
  DataFlowSelect,
  IDataPipelineSelectNode,
} from '@/components/data-pipeline-select';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { pipelineFormSchema } from '../form-schema';
import { IDataPipelineNodeProps } from './link-data-pipeline';

const LinkDataPipelineModal = ({
  data,
  open,
  setOpen,
  onSubmit,
}: {
  data: IDataPipelineNodeProps | undefined;
  open: boolean;
  setOpen: (open: boolean) => void;
  onSubmit?: (pipeline: IDataPipelineSelectNode | undefined) => void;
}) => {
  const isEdit = !!data;
  const [list, setList] = useState<IDataPipelineSelectNode[]>();
  const form = useForm<z.infer<typeof pipelineFormSchema>>({
    resolver: zodResolver(pipelineFormSchema),
    defaultValues: {
      pipeline_id: '',
      set_default: false,
      file_filter: '',
    },
  });
  //   const [open, setOpen] = useState(false);
  const { navigateToAgents } = useNavigatePage();
  const handleFormSubmit = (values: any) => {
    console.log(values, data);
    // const param = {
    //   ...data,
    //   ...values,
    // };
    const pipeline = list?.find((item) => item.id === values.pipeline_id);
    onSubmit?.(pipeline);
  };
  return (
    <Modal
      className="!w-[560px]"
      title={
        !isEdit
          ? t('knowledgeConfiguration.linkDataPipeline')
          : t('knowledgeConfiguration.editLinkDataPipeline')
      }
      open={open}
      onOpenChange={setOpen}
      showfooter={false}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(handleFormSubmit)}>
          <div className="flex flex-col gap-4 ">
            {!isEdit && (
              <DataFlowSelect
                toDataPipeline={navigateToAgents}
                formFieldName="pipeline_id"
                setDataList={setList}
              />
            )}
            {/* <FormField
              control={form.control}
              name={'file_filter'}
              render={({ field }) => (
                <FormItem className=" items-center space-y-0 ">
                  <div className="flex flex-col gap-1">
                    <div className="flex gap-2 justify-between ">
                      <FormLabel
                        tooltip={t('knowledgeConfiguration.fileFilterTip')}
                        className="text-sm text-text-primary whitespace-wrap "
                      >
                        {t('knowledgeConfiguration.fileFilter')}
                      </FormLabel>
                    </div>

                    <div className="text-muted-foreground">
                      <FormControl>
                        <Input
                          placeholder={t(
                            'knowledgeConfiguration.filterPlaceholder',
                          )}
                          {...field}
                        />
                      </FormControl>
                    </div>
                  </div>
                  <div className="flex pt-1">
                    <div className="w-full"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
            {isEdit && (
              <FormField
                control={form.control}
                name={'set_default'}
                render={({ field }) => (
                  <FormItem className=" items-center space-y-0 ">
                    <div className="flex flex-col gap-1">
                      <div className="flex gap-2 justify-between ">
                        <FormLabel
                          tooltip={t('knowledgeConfiguration.setDefaultTip')}
                          className="text-sm text-text-primary whitespace-wrap "
                        >
                          {t('knowledgeConfiguration.setDefault')}
                        </FormLabel>
                      </div>

                      <div className="text-muted-foreground">
                        <FormControl>
                          <Switch
                            value={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                      </div>
                    </div>
                    <div className="flex pt-1">
                      <div className="w-full"></div>
                      <FormMessage />
                    </div>
                  </FormItem>
                )}
              />
            )} */}
            <div className="flex justify-end gap-1">
              <Button
                type="button"
                variant={'outline'}
                className="btn-primary"
                onClick={() => {
                  setOpen(false);
                }}
              >
                {t('modal.cancelText')}
              </Button>
              <Button
                type="button"
                variant={'default'}
                className="btn-primary"
                onClick={form.handleSubmit(handleFormSubmit)}
              >
                {t('modal.okText')}
              </Button>
            </div>
          </div>
        </form>
      </Form>
    </Modal>
  );
};
export default LinkDataPipelineModal;
