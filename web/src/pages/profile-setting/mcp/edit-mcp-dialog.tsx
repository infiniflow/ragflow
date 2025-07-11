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
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, omit, pick } from 'lodash';
import { RefreshCw } from 'lucide-react';
import { MouseEventHandler, useCallback, useMemo, useState } from 'react';
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

function transferToolToObject(tools: IMCPTool[] = []) {
  return tools.reduce<IMCPToolObject>((pre, tool) => {
    pre[tool.name] = omit(tool, 'name');
    return pre;
  }, {});
}

function transferToolToArray(tools: IMCPToolObject) {
  return Object.entries(tools).reduce<IMCPTool[]>((pre, [name, tool]) => {
    pre.push({ ...tool, name });
    return pre;
  }, []);
}

export function EditMcpDialog({
  hideModal,
  loading,
  onOk,
  id,
}: IModalProps<any> & { id: string }) {
  const { t } = useTranslation();
  const {
    testMcpServer,
    data: tools,
    loading: testLoading,
  } = useTestMcpServer();
  const [isTriggeredBySaving, setIsTriggeredBySaving] = useState(false);
  const FormSchema = useBuildFormSchema();
  const [collapseOpen, setCollapseOpen] = useState(true);
  const { data } = useGetMcpServer(id);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    values: isEmpty(data)
      ? { name: '', server_type: ServerType.SSE, url: '' }
      : pick(data, ['name', 'server_type', 'url']),
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
      onOk?.({
        ...values,
        variables: {
          ...(values?.variables || {}),
          tools: transferToolToObject(tools),
        },
      });
    } else {
      testMcpServer(values);
    }
  };

  const nextTools = useMemo(() => {
    return tools || transferToolToArray(data.variables?.tools || {});
  }, [data.variables?.tools, tools]);

  const dirtyFields = form.formState.dirtyFields;
  const fieldChanged = 'server_type' in dirtyFields || 'url' in dirtyFields;
  const disabled = !!!tools?.length || testLoading || fieldChanged;

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit profile</DialogTitle>
        </DialogHeader>
        <EditMcpForm onOk={handleOk} form={form}></EditMcpForm>
        <Collapse
          title={<div>{tools?.length || 0} tools available</div>}
          open={collapseOpen}
          onOpenChange={setCollapseOpen}
          rightContent={
            <Button
              variant={'ghost'}
              form={FormId}
              type="submit"
              onClick={handleTest}
            >
              <RefreshCw className="text-background-checked" />
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
