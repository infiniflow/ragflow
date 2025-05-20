import Editor, { loader } from '@monaco-editor/react';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from './dynamic-input-variable';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { useTranslation } from 'react-i18next';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ form, node }: INextOperatorForm) => {
  const formData = node?.data.form as ICodeForm;
  const { t } = useTranslation();

  // useEffect(() => {
  //   setTimeout(() => {
  //     // TODO: Direct operation zustand is more elegant
  //     form?.setFieldValue(
  //       'script',
  //       CodeTemplateStrMap[formData.lang as ProgrammingLanguage],
  //     );
  //   }, 0);
  // }, [form, formData.lang]);

  return (
    <Form {...form}>
      <DynamicInputVariable node={node}></DynamicInputVariable>
      <FormField
        control={form.control}
        name="script"
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              <FormField
                control={form.control}
                name="channel"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel tooltip={t('channelTip')}>
                      {t('channel')}
                    </FormLabel>
                    <FormControl>
                      <RAGFlowSelect {...field} options={options} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </FormLabel>
            <FormControl>
              <Editor
                height={600}
                theme="vs-dark"
                language={formData.lang}
                options={{
                  minimap: { enabled: false },
                  automaticLayout: true,
                }}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </Form>
  );
};

export default CodeForm;
