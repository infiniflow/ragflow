import CopyToClipboard from '@/components/copy-to-clipboard';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { formatDate } from '@/utils/date';
import { Trash2 } from 'lucide-react';
import { useOperateApiKey } from '../hooks';

const ChatApiKeyModal = ({
  dialogId,
  hideModal,
  idKey,
}: IModalProps<any> & { dialogId?: string; idKey: string }) => {
  const { createToken, removeToken, tokenList, listLoading, creatingLoading } =
    useOperateApiKey(idKey, dialogId);
  const { t } = useTranslate('chat');

  return (
    <>
      <Dialog open onOpenChange={hideModal}>
        <DialogContent className="max-w-[50vw]">
          <DialogHeader>
            <DialogTitle>{t('apiKey')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {listLoading ? (
              <div className="flex justify-center py-8">Loading...</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Token</TableHead>
                    <TableHead>{t('created')}</TableHead>
                    <TableHead>{t('action')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tokenList?.map((tokenItem) => (
                    <TableRow key={tokenItem.token}>
                      <TableCell className="font-medium break-all">
                        {tokenItem.token}
                      </TableCell>
                      <TableCell>{formatDate(tokenItem.create_date)}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <CopyToClipboard text={tokenItem.token} />
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => removeToken(tokenItem.token)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
            <Button
              onClick={createToken}
              loading={creatingLoading}
              disabled={tokenList?.length > 0}
            >
              {t('createNewKey')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default ChatApiKeyModal;
