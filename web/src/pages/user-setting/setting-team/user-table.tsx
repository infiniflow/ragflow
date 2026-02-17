import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useListTenantUser } from '@/hooks/use-user-setting-request';
import { formatDate } from '@/utils/date';
import { upperFirst } from 'lodash';
import { ArrowDown, ArrowUp, ArrowUpDown, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TenantRole } from '../constants';
import { useHandleDeleteUser } from './hooks';

const ColorMap: Record<string, string> = {
  [TenantRole.Normal]: 'bg-transparent text-white',
  [TenantRole.Invite]: 'bg-accent-primary-5 bg-accent-primary rounded-sm',
  [TenantRole.Owner]: 'bg-red-100 text-red-800',
};

const UserTable = ({ searchUser }: { searchUser: string }) => {
  const { data, loading } = useListTenantUser();
  const { deleteTenantUser } = useHandleDeleteUser();
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc' | null>(null);
  const { t } = useTranslation();
  const sortedData = useMemo(() => {
    console.log('sortedData', data, searchUser);
    if (!data || data.length === 0) return data;
    let filtered = data;
    if (searchUser) {
      filtered = filtered.filter(
        (tenant) =>
          tenant.nickname.toLowerCase().includes(searchUser.toLowerCase()) ||
          tenant.email.toLowerCase().includes(searchUser.toLowerCase()),
      );
    }
    if (sortOrder) {
      filtered = [...filtered].sort((a, b) => {
        const dateA = new Date(a.update_date).getTime();
        const dateB = new Date(b.update_date).getTime();

        if (sortOrder === 'asc') {
          return dateA - dateB;
        } else {
          return dateB - dateA;
        }
      });
    }

    return filtered;
  }, [data, sortOrder, searchUser]);
  const toggleSortOrder = () => {
    if (sortOrder === 'asc') {
      setSortOrder('desc');
    } else if (sortOrder === 'desc') {
      setSortOrder(null);
    } else {
      setSortOrder('asc');
    }
  };

  const renderSortIcon = () => {
    if (sortOrder === 'asc') {
      return <ArrowUp className="ml-1 h-4 w-4 " />;
    } else if (sortOrder === 'desc') {
      return <ArrowDown className="ml-1 h-4 w-4" />;
    } else {
      return <ArrowUpDown className="ml-1 h-4 w-4" />;
    }
  };
  return (
    <div className="rounded-lg bg-bg-input scrollbar-auto overflow-hidden border border-border-default">
      <Table rootClassName="rounded-lg">
        <TableHeader className="bg-bg-title">
          <TableRow className="hover:bg-bg-title">
            <TableHead className="h-12 px-4">{t('common.name')}</TableHead>
            <TableHead
              className="h-12 px-4 cursor-pointer"
              onClick={toggleSortOrder}
            >
              <div className="flex items-center">
                {t('setting.updateDate')}
                {renderSortIcon()}
              </div>
            </TableHead>
            <TableHead className="h-12 px-4">{t('setting.email')}</TableHead>
            <TableHead className="h-12 px-4">{t('setting.role')}</TableHead>
            <TableHead className="h-12 px-4">{t('common.action')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className="bg-bg-base">
          {loading ? (
            <TableRow>
              <TableCell colSpan={5} className="h-24 text-center">
                <div className="flex items-center justify-center">
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-solid border-current border-r-transparent align-[-0.125em] motion-reduce:animate-[spin_1.5s_linear_infinite]"></div>
                </div>
              </TableCell>
            </TableRow>
          ) : sortedData && sortedData.length > 0 ? (
            sortedData.map((record) => (
              <TableRow key={record.user_id} className="hover:bg-bg-card">
                <TableCell className="p-4 ">
                  <div className="flex gap-1 items-center">
                    <RAGFlowAvatar
                      isPerson
                      className="size-4"
                      avatar={record.avatar}
                      name={record.nickname}
                    />
                    {record.nickname}
                  </div>
                </TableCell>
                <TableCell className="p-4">
                  {formatDate(record.update_date)}
                </TableCell>
                <TableCell className="p-4">{record.email}</TableCell>
                <TableCell className="p-4">
                  {record.role === TenantRole.Normal && (
                    <Badge className={ColorMap[record.role]}>
                      {upperFirst('Member')}
                    </Badge>
                  )}
                  {record.role !== TenantRole.Normal && (
                    <Badge className={ColorMap[record.role]}>
                      {upperFirst(record.role)}
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="p-4">
                  <ConfirmDeleteDialog
                    title={t('deleteModal.delMember')}
                    onOk={async () => {
                      await deleteTenantUser({
                        userId: record.user_id,
                      });
                      return;
                    }}
                    content={{
                      node: (
                        <ConfirmDeleteDialogNode
                          avatar={{
                            avatar: record.avatar,
                            name: record.nickname,
                            isPerson: true,
                          }}
                          name={record.email}
                        ></ConfirmDeleteDialogNode>
                      ),
                    }}
                  >
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 p-0 hover:bg-state-error-5 hover:text-state-error"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </ConfirmDeleteDialog>
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={5} className="h-24 text-center">
                {t('common.noData')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
};

export default UserTable;
