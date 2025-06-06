import Editor, { loader } from '@monaco-editor/react';
import { INextOperatorForm } from '../../interface';

import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  DynamicInputVariable,
  TypeOptions,
  VariableTitle,
} from './next-variable';
import { useValues } from './use-values';
import {
  useHandleLanguageChange,
  useWatchFormChange,
} from './use-watch-change';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ node }: INextOperatorForm) => {
  const formData = node?.data.form as ICodeForm;
  const { t } = useTranslation();
  const values = useValues(node);

  const FormSchema = z.object({
    lang: z.string(),
    script: z.string(),
    arguments: z.array(
      z.object({ name: z.string(), component_id: z.string() }),
    ),
    return: z.union([
      z
        .array(z.object({ name: z.string(), component_id: z.string() }))
        .optional(),
      z.object({ name: z.string(), component_id: z.string() }),
    ]),
  });

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const handleLanguageChange = useHandleLanguageChange(node?.id, form);

  return (
    <Form {...form}>
      <form
        className="p-5 space-y-5"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable
          node={node}
          title={t('flow.input')}
        ></DynamicInputVariable>
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
                        <RAGFlowSelect
                          {...field}
                          onChange={(val) => {
                            field.onChange(val);
                            handleLanguageChange(val);
                          }}
                          options={options}
                        />
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

        {formData.lang === ProgrammingLanguage.Python ? (
          <DynamicInputVariable
            node={node}
            title={'Return Values'}
            name={'return'}
          ></DynamicInputVariable>
        ) : (
          <div>
            <VariableTitle title={'Return Values'}></VariableTitle>
            <FormContainer className="space-y-5">
              <FormField
                control={form.control}
                name={'return.name'}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('common.pleaseInput')}
                      ></Input>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`return.component_id`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormLabel>Type</FormLabel>
                    <FormControl>
                      <RAGFlowSelect
                        placeholder={t('common.pleaseSelect')}
                        options={TypeOptions}
                        {...field}
                      ></RAGFlowSelect>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </FormContainer>
          </div>
        )}
      </form>
    </Form>
  );
};

export default CodeForm;
