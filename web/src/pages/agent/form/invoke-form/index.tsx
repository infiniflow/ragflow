import { Collapse } from '@/components/collapse';
import { FormContainer } from '@/components/form-container';
import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { zodResolver } from '@hookform/resolvers/zod';
import Editor, { loader } from '@monaco-editor/react';
import { Plus } from 'lucide-react';
import { memo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { initialInvokeValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
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

  const variables = useWatch({ control: form.control, name: 'variables' });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <FormField
            control={form.control}
            name="url"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.url')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="http://" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="method"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.method')}</FormLabel>
                <FormControl>
                  <SelectWithSearch {...field} options={MethodOptions} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="timeout"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.timeout')}</FormLabel>
                <FormControl>
                  <TimeoutInput {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="headers"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.headers')}</FormLabel>
                <FormControl>
                  <Editor
                    height={200}
                    defaultLanguage="json"
                    theme="vs-dark"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="proxy"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.proxy')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="clean_html"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('flow.cleanHtmlTip')}>
                  {t('flow.cleanHtml')}
                </FormLabel>
                <FormControl>
                  <Switch
                    onCheckedChange={field.onChange}
                    checked={field.value}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
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
            <Button
              variant={'ghost'}
              onClick={(e) => {
                e.preventDefault();
                showModal();
              }}
            >
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
