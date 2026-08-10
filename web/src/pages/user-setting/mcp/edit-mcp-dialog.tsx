import { Collapse } from '@/components/collapse';
import { Button, ButtonLoading } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { DialogFooter } from '@/components/ui/dialog';
import { Modal } from '@/components/ui/modal/modal';
import { useGetMcpServer, useTestMcpServer } from '@/hooks/use-mcp-request';
import { IModalProps } from '@/interfaces/common';
import { IMCPTool, IMCPToolObject } from '@/interfaces/database/mcp';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, omit, pick } from 'lodash';
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
  authorization_token: '',
};

// Mask shown in the token field when editing an existing server that already
// has a token, so the user knows a secret is stored without exposing it.
const TokenMask = '********';

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

  const tools = useMemo<IMCPTool[]>(() => {
    const tested = testData?.data;
    if (tested?.length) {
      return tested;
    }
    return transferToolToArray(data.variables?.tools || {});
  }, [testData?.data, data.variables?.tools]);

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
    // When the field still holds the mask, the user did not change the token;
    // send the previously stored token so it is not overwritten with empty.
    const token =
      values.authorization_token === TokenMask
        ? data.variables?.authorization_token
        : values.authorization_token;
    const nextValues = {
      ...omit(values, 'authorization_token'),
      variables: { authorization_token: token },
      headers: { Authorization: 'Bearer ${authorization_token}' },
    };
    if (isTriggeredBySaving) {
      onOk?.(nextValues);
    } else {
      const ret = await testMcpServer(nextValues);
      if (ret.code === 0) {
        setFieldChanged(false);
      }
    }
  };

  useEffect(() => {
    if (!isEmpty(data)) {
      form.reset({
        ...pick(data, ['name', 'server_type', 'url']),
        authorization_token: data.variables?.authorization_token
          ? TokenMask
          : '',
      });
    }
  }, [data, form]);

  const disabled = !tools?.length || testLoading || fieldChanged;

  return (
    <Modal
      title={id ? t('mcp.editMCP') : t('mcp.addMCP')}
      open={true}
      onCancel={hideModal}
      cancelText={t('common.cancel')}
      okText={t('common.save')}
      maskClosable={false}
      footer={
        <DialogFooter>
          <Button variant="outline" type="button" onClick={hideModal}>
            {t('common.cancel')}
          </Button>
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
      }
    >
      <EditMcpForm
        onOk={handleOk}
        form={form}
        setFieldChanged={setFieldChanged}
      ></EditMcpForm>
      <Card className="bg-transparent mt-4">
        <CardContent className="p-3">
          <Collapse
            title={
              <div>
                {tools?.length || 0} {t('mcp.toolsAvailable')}
              </div>
            }
            open={collapseOpen}
            onOpenChange={setCollapseOpen}
            rightContent={
              <Button
                variant={'transparent'}
                form={FormId}
                type="submit"
                onClick={handleTest}
                className="border-none p-0 text-text-secondary hover:bg-transparent hover:text-text-primary"
              >
                <RefreshCw
                  className={cn({
                    'animate-spin': testLoading,
                  })}
                />
              </Button>
            }
          >
            <div className="overflow-auto max-h-80 divide-y-[0.5px] divide-border-button bg-bg-card rounded-md px-2.5 scrollbar-auto">
              {tools?.map((x) => (
                <McpToolCard key={x.name} data={x}></McpToolCard>
              ))}
            </div>
          </Collapse>
        </CardContent>
      </Card>
    </Modal>
  );
}
