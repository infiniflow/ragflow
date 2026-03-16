import { Collapse } from '@/components/collapse';
import { Button, ButtonLoading } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { DialogClose, DialogFooter } from '@/components/ui/dialog';
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
    const nextValues = {
      ...omit(values, 'authorization_token'),
      variables: { authorization_token: values.authorization_token },
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
    // <Dialog open onOpenChange={hideModal}>
    //   <DialogContent>
    //     <DialogHeader>
    //       <DialogTitle>{id ? t('mcp.editMCP') : t('mcp.addMCP')}</DialogTitle>
    //     </DialogHeader>
    //     <EditMcpForm
    //       onOk={handleOk}
    //       form={form}
    //       setFieldChanged={setFieldChanged}
    //     ></EditMcpForm>
    //     <Card className="bg-transparent">
    //       <CardContent className="p-3">
    //         <Collapse
    //           title={
    //             <div>
    //               {nextTools?.length || 0} {t('mcp.toolsAvailable')}
    //             </div>
    //           }
    //           open={collapseOpen}
    //           onOpenChange={setCollapseOpen}
    //           rightContent={
    //             <Button
    //               variant={'transparent'}
    //               form={FormId}
    //               type="submit"
    //               onClick={handleTest}
    //               className="border-none p-0 hover:bg-transparent"
    //             >
    //               <RefreshCw
    //                 className={cn('text-text-secondary', {
    //                   'animate-spin': testLoading,
    //                 })}
    //               />
    //             </Button>
    //           }
    //         >
    //           <div className="overflow-auto max-h-80 divide-y-[0.5px] divide-border-button bg-bg-card rounded-md px-2.5 scrollbar-auto">
    //             {nextTools?.map((x) => (
    //               <McpToolCard key={x.name} data={x}></McpToolCard>
    //             ))}
    //           </div>
    //         </Collapse>
    //       </CardContent>
    //     </Card>
    //     <DialogFooter>
    //       <DialogClose asChild>
    //         <Button variant="outline">{t('common.cancel')}</Button>
    //       </DialogClose>
    //       <ButtonLoading
    //         type="submit"
    //         form={FormId}
    //         loading={loading}
    //         onClick={handleSave}
    //         disabled={disabled}
    //       >
    //         {t('common.save')}
    //       </ButtonLoading>
    //     </DialogFooter>
    //   </DialogContent>
    // </Dialog>
    <Modal
      title={id ? t('mcp.editMCP') : t('mcp.addMCP')}
      open={true}
      onCancel={hideModal}
      cancelText={t('common.cancel')}
      okText={t('common.save')}
      footer={
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">{t('common.cancel')}</Button>
          </DialogClose>
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
      <Card className="bg-transparent">
        <CardContent className="p-3">
          <Collapse
            title={
              <div>
                {nextTools?.length || 0} {t('mcp.toolsAvailable')}
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
              {nextTools?.map((x) => (
                <McpToolCard key={x.name} data={x}></McpToolCard>
              ))}
            </div>
          </Collapse>
        </CardContent>
      </Card>
    </Modal>
  );
}
