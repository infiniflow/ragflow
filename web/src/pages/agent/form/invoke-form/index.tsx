import { Collapse } from '@/components/collapse';
import { FormContainer } from '@/components/form-container';
import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-form-field';
import { useIsDarkTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { Form, FormField } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import Editor, { loader } from '@monaco-editor/react';
import { Plus } from 'lucide-react';
import { memo, useCallback } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { initialInvokeValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { FormSchema, FormSchemaType } from './schema';
import { useEditVariableRecord } from './use-edit-variable';
import { VariableDialog } from './variable-dialog';
import { VariableTable } from './variable-table';

loader.config({ paths: { vs: '/vs' } });

enum Method {
  GET = 'GET',
  POST = 'POST',
  PUT = 'PUT',
}

const MethodOptions = [Method.GET, Method.POST, Method.PUT].map((x) => ({
  label: x,
  value: x,
}));

interface TimeoutInputProps {
  value?: number;
  onChange?: (value: number | null) => void;
}

const TimeoutInput = ({ value, onChange }: TimeoutInputProps) => {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2 items-center">
      <NumberInput value={value} onChange={onChange} /> {t('flow.seconds')}
    </div>
  );
};

const outputList = buildOutputList(initialInvokeValues.outputs);

function InvokeForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialInvokeValues, node);

  const form = useForm<FormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  const {
    visible,
    hideModal,
    showModal,
    ok,
    currentRecord,
    otherThanCurrentQuery,
    handleDeleteRecord,
  } = useEditVariableRecord({
    form,
    node,
  });

  const handleAddVariable = useCallback(() => {
    showModal();
  }, [showModal]);

  const variables = useWatch({ control: form.control, name: 'variables' });

  const isDarkTheme = useIsDarkTheme();

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <RAGFlowFormItem name="url" label={t('flow.url')}>
            <PromptEditor
              placeholder="http://"
              showToolbar={false}
              multiLine={false}
            />
          </RAGFlowFormItem>
          <RAGFlowFormItem name="method" label={t('flow.method')}>
            <SelectWithSearch options={MethodOptions} />
          </RAGFlowFormItem>
          <RAGFlowFormItem name="timeout" label={t('flow.timeout')}>
            {(field) => (
              <TimeoutInput value={field.value} onChange={field.onChange} />
            )}
          </RAGFlowFormItem>
          <RAGFlowFormItem name="headers" label={t('flow.headers')}>
            {(field) => (
              <Editor
                height={200}
                defaultLanguage="json"
                theme={isDarkTheme ? 'vs-dark' : undefined}
                value={field.value}
                onChange={field.onChange}
              />
            )}
          </RAGFlowFormItem>
          <RAGFlowFormItem name="proxy" label={t('flow.proxy')}>
            <Input />
          </RAGFlowFormItem>
          <SwitchFormField
            name="clean_html"
            label={t('flow.cleanHtml')}
            tooltip={t('flow.cleanHtmlTip')}
          />
          {/* Create a hidden field to make Form instance record this */}
          <FormField
            control={form.control}
            name={'variables'}
            render={() => <div></div>}
          />
        </FormContainer>
        <Collapse
          title={<div>{t('flow.parameter')}</div>}
          rightContent={
            <Button variant={'ghost'} type="button" onClick={handleAddVariable}>
              <Plus />
            </Button>
          }
        >
          <VariableTable
            data={variables}
            showModal={showModal}
            deleteRecord={handleDeleteRecord}
            nodeId={node?.id}
          ></VariableTable>
        </Collapse>
        {visible && (
          <VariableDialog
            hideModal={hideModal}
            initialValue={currentRecord}
            otherThanCurrentQuery={otherThanCurrentQuery}
            submit={ok}
          ></VariableDialog>
        )}
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(InvokeForm);
