import Editor, { loader } from '@monaco-editor/react';
import { INextOperatorForm } from '../../interface';

import { FormContainer } from '@/components/form-container';
import { useIsDarkTheme } from '@/components/theme-provider';
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
import { ICodeForm } from '@/interfaces/database/agent';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import {
  DynamicInputVariable,
  TypeOptions,
  VariableTitle,
} from './next-variable';
import { FormSchema, FormSchemaType } from './schema';
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

const DynamicFieldName = 'outputs';

function CodeForm({ node }: INextOperatorForm) {
  const formData = node?.data.form as ICodeForm;
  const { t } = useTranslation();
  const values = useValues(node);
  const isDarkTheme = useIsDarkTheme();

  const form = useForm<FormSchemaType>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const handleLanguageChange = useHandleLanguageChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <DynamicInputVariable
          node={node}
          title={t('flow.input')}
          isOutputs={false}
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
                  theme={isDarkTheme ? 'vs-dark' : 'vs'}
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
            name={DynamicFieldName}
            isOutputs
          ></DynamicInputVariable>
        ) : (
          <div>
            <VariableTitle title={'Return Values'}></VariableTitle>
            <FormContainer className="space-y-5">
              <FormField
                control={form.control}
                name={`${DynamicFieldName}.name`}
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
                name={`${DynamicFieldName}.type`}
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
      </FormWrapper>
      <div className="p-5">
        <Output list={buildOutputList(formData.outputs)}></Output>
      </div>
    </Form>
  );
}

export default memo(CodeForm);
