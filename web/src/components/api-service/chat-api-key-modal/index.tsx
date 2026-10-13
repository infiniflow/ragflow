import { useTranslation } from 'react-i18next';
/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
  const { t: translateUi } = useTranslation();

  const { createToken, removeToken, tokenList, listLoading, creatingLoading } =
    useOperateApiKey(idKey, dialogId);
  const { t } = useTranslate('chat');

  return (
    <>
      <Dialog open onOpenChange={hideModal}>
        <DialogContent className="max-w-[50vw] max-h-[calc(100vh-8rem)] flex flex-col">
          <DialogHeader>
            <DialogTitle>{t('apiKey')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 flex flex-col min-h-0">
            {listLoading ? (
              <div className="flex justify-center py-8">
                {translateUi('ui.loadingInProgress')}
              </div>
            ) : (
              <Table rootClassName="min-h-0 shrink">
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
              className="w-fit shrink-0"
              onClick={createToken}
              loading={creatingLoading}
              disabled={tokenList?.length >= 16}
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
