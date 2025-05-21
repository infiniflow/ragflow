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
import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { useEffect } from 'react';
import { DynamicInputVariable } from './next-variable';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ form, node }: INextOperatorForm) => {
  const formData = node?.data.form as ICodeForm;

  useEffect(() => {
    // TODO: Direct operation zustand is more elegant
    form?.setValue(
      'script',
      CodeTemplateStrMap[formData.lang as ProgrammingLanguage],
    );
  }, [form, formData.lang]);

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
