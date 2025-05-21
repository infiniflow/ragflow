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
import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/flow';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DynamicInputVariable,
  TypeOptions,
  VariableTitle,
} from './next-variable';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

const CodeForm = ({ form, node }: INextOperatorForm) => {
  const formData = node?.data.form as ICodeForm;
  const { t } = useTranslation();

  useEffect(() => {
    // TODO: Direct operation zustand is more elegant
    form?.setValue(
      'script',
      CodeTemplateStrMap[formData.lang as ProgrammingLanguage],
    );
  }, [form, formData.lang]);

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
