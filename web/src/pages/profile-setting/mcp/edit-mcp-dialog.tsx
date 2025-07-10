import { Collapse } from '@/components/collapse';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useTestMcpServer } from '@/hooks/use-mcp-request';
import { IModalProps } from '@/interfaces/common';
import { IMCPTool, IMCPToolObject } from '@/interfaces/database/mcp';
import { omit } from 'lodash';
import { RefreshCw } from 'lucide-react';
import { MouseEventHandler, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { EditMcpForm, FormId, useBuildFormSchema } from './edit-mcp-form';
import { McpToolCard } from './tool-card';

function transferToolToObject(tools: IMCPTool[] = []) {
  return tools.reduce<IMCPToolObject>((pre, tool) => {
    pre[tool.name] = omit(tool, 'name');
    return pre;
  }, {});
}

export function EditMcpDialog({ hideModal, loading, onOk }: IModalProps<any>) {
  const { t } = useTranslation();
  const { testMcpServer, data: tools } = useTestMcpServer();
  const [isTriggeredBySaving, setIsTriggeredBySaving] = useState(false);
  const FormSchema = useBuildFormSchema();

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

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit profile</DialogTitle>
        </DialogHeader>
        <EditMcpForm onOk={handleOk}></EditMcpForm>
        <Collapse
          title={<div>{tools?.length || 0} tools available</div>}
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
          <div className="space-y-2.5">
            {tools?.map((x) => (
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
            disabled={!!!tools?.length}
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
