import { Collapse } from '@/components/collapse';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useGetMcpServer, useTestMcpServer } from '@/hooks/use-mcp-request';
import { IModalProps } from '@/interfaces/common';
import { IMCPTool, IMCPToolObject } from '@/interfaces/database/mcp';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, pick } from 'lodash';
import { RefreshCw } from 'lucide-react';
import {
  MouseEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  EditMcpForm,
  FormId,
  ServerType,
  useBuildFormSchema,
} from './edit-mcp-form';
import { McpToolCard } from './tool-card';

function transferToolToArray(tools: IMCPToolObject) {
  return Object.entries(tools).reduce<IMCPTool[]>((pre, [name, tool]) => {
    pre.push({ ...tool, name });
    return pre;
  }, []);
}

const DefaultValues = {
  name: '',
  server_type: ServerType.SSE,
  url: '',
};

export function EditMcpDialog({
  hideModal,
  loading,
  onOk,
  id,
}: IModalProps<any> & { id: string }) {
  const { t } = useTranslation();
  const {
    testMcpServer,
    data: testData,
    loading: testLoading,
  } = useTestMcpServer();
  const [isTriggeredBySaving, setIsTriggeredBySaving] = useState(false);
  const FormSchema = useBuildFormSchema();
  const [collapseOpen, setCollapseOpen] = useState(true);
  const { data } = useGetMcpServer(id);
  const [fieldChanged, setFieldChanged] = useState(false);

  const tools = useMemo(() => {
    return testData?.data || [];
  }, [testData?.data]);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: DefaultValues,
  });

  const handleTest: MouseEventHandler<HTMLButtonElement> = useCallback((e) => {
    e.stopPropagation();
    setIsTriggeredBySaving(false);
  }, []);

  const handleSave: MouseEventHandler<HTMLButtonElement> = useCallback(() => {
    setIsTriggeredBySaving(true);
  }, []);

  const handleOk = async (values: z.infer<typeof FormSchema>) => {
    if (isTriggeredBySaving) {
      onOk?.(values);
    } else {
      const ret = await testMcpServer(values);
      if (ret.code === 0) {
        setFieldChanged(false);
      }
    }
  };

  useEffect(() => {
    if (!isEmpty(data)) {
      form.reset(pick(data, ['name', 'server_type', 'url']));
    }
  }, [data, form]);

  const nextTools = useMemo(() => {
    return isEmpty(tools)
      ? transferToolToArray(data.variables?.tools || {})
      : tools;
  }, [data.variables?.tools, tools]);

  const disabled = !!!tools?.length || testLoading || fieldChanged;

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('common.edit')}</DialogTitle>
        </DialogHeader>
        <EditMcpForm
          onOk={handleOk}
          form={form}
          setFieldChanged={setFieldChanged}
        ></EditMcpForm>
        <Collapse
          title={<div>{nextTools?.length || 0} tools available</div>}
          open={collapseOpen}
          onOpenChange={setCollapseOpen}
          rightContent={
            <Button
              variant={'ghost'}
              form={FormId}
              type="submit"
              onClick={handleTest}
            >
              <RefreshCw
                className={cn('text-background-checked', {
                  'animate-spin': testLoading,
                })}
              />
            </Button>
          }
        >
          <div className="space-y-2.5 overflow-auto max-h-80">
            {nextTools?.map((x) => (
              <McpToolCard key={x.name} data={x}></McpToolCard>
            ))}
          </div>
        </Collapse>
        <DialogFooter>
          <ButtonLoading
            type="submit"
            form={FormId}
            loading={loading}
            onClick={handleSave}
            disabled={disabled}
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
