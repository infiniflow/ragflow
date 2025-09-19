import { DataFlowSelect } from '@/components/data-pipeline-select';
import Input from '@/components/originui/input';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { linkPiplineFormSchema } from '../form-schema';

const LinkDataPipelineModal = ({
  open,
  setOpen,
}: {
  open: boolean;
  setOpen: (open: boolean) => void;
}) => {
  const form = useForm<z.infer<typeof linkPiplineFormSchema>>({
    resolver: zodResolver(linkPiplineFormSchema),
    defaultValues: { data_flow: ['888'], file_filter: '' },
  });
  //   const [open, setOpen] = useState(false);
  const { navigateToAgents } = useNavigatePage();
  const handleFormSubmit = (values: any) => {
    console.log(values);
  };
  return (
    <Modal
      title={t('knowledgeConfiguration.linkDataPipeline')}
      open={open}
      onOpenChange={setOpen}
      showfooter={false}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(handleFormSubmit)}>
          <div className="flex flex-col gap-4 ">
            <DataFlowSelect
              toDataPipeline={navigateToAgents}
              formFieldName="data_flow"
            />
            <FormField
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
                          placeholder={t('dataFlowPlaceholder')}
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
            <div className="flex justify-end gap-1">
              <Button type="reset" variant={'outline'} className="btn-primary">
                {t('modal.cancelText')}
              </Button>
              <Button type="submit" variant={'default'} className="btn-primary">
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
