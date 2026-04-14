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
import { AlertTriangle } from 'lucide-react';
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
import {
  CodeExecPanelSystemOutputs,
  getBusinessOutputs,
  serializeCodeOutputContract,
} from './utils';

loader.config({ paths: { vs: '/vs' } });

const options = [
  ProgrammingLanguage.Python,
  ProgrammingLanguage.Javascript,
].map((x) => ({ value: x, label: x }));

function CodeForm({ node }: INextOperatorForm) {
  const formData = node?.data.form as ICodeForm;
  const { t } = useTranslation();
  const { values, legacyOutputs } = useValues(node);
  const isDarkTheme = useIsDarkTheme();

  const form = useForm<FormSchemaType>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const handleLanguageChange = useHandleLanguageChange(node?.id, form);
  const lang = form.watch('lang');
  const currentOutput = form.watch('output');
  const outputFieldDirty = !!form.formState.dirtyFields?.output;
  const displayedBusinessOutputs =
    legacyOutputs.length > 0 && !outputFieldDirty
      ? getBusinessOutputs(formData?.outputs)
      : serializeCodeOutputContract(currentOutput);

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
                  language={lang}
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

        <div className="space-y-3">
          <VariableTitle title={'Return Value'}></VariableTitle>
          {legacyOutputs.length > 0 && (
            <div className="flex items-start gap-2 rounded-md border border-state-error/40 bg-state-error/10 px-3 py-2 text-sm text-text-primary">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-state-error" />
              <p>
                This CodeExec node uses the deprecated multi-output schema:{' '}
                {legacyOutputs.join(', ')}. Keep one business output here and
                move field extraction to downstream nodes.
              </p>
            </div>
          )}
          <FormContainer className="space-y-5">
            <FormField
              control={form.control}
              name="output.name"
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
              name="output.type"
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
      </FormWrapper>
      <div className="space-y-4 p-5">
        <Output list={buildOutputList(displayedBusinessOutputs)}>
          Business
        </Output>
        <Output list={buildOutputList(CodeExecPanelSystemOutputs)}>
          System
        </Output>
      </div>
    </Form>
  );
}

export default memo(CodeForm);
