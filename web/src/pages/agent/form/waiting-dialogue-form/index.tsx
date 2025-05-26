import Editor, { loader } from '@monaco-editor/react';
import { INextOperatorForm } from '../../interface';

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

  return (
    <Form {...form}>
      <form
        className="p-5 space-y-5"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormField
          control={form.control}
          name="script"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="flex items-center justify-between">
                Code
                <FormField
                  control={form.control}
                  name="lang"
                  render={({ field }) => (
                    <FormItem>
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
                  height={300}
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
      </form>
    </Form>
  );
};

export default CodeForm;
