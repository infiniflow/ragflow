import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useFetchUserInfo, useListTenant } from '@/hooks/user-setting-hooks';
import { formatDate } from '@/utils/date';
import { ArrowDown, ArrowUp, ArrowUpDown, LogOut } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TenantRole } from '../constants';
import { useHandleAgreeTenant, useHandleQuitUser } from './hooks';
const TenantTable = ({ searchTerm }: { searchTerm: string }) => {
  const { t } = useTranslation();
  const { data, loading } = useListTenant();
  const { handleAgree } = useHandleAgreeTenant();
  const { data: user } = useFetchUserInfo();
  const { handleQuitTenantUser } = useHandleQuitUser();
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc' | null>(null);
  const sortedData = useMemo(() => {
    if (!data || data.length === 0) return data;
    let filtered = data;
    if (searchTerm) {
      filtered = data.filter(
        (tenant) =>
          tenant.nickname.toLowerCase().includes(searchTerm.toLowerCase()) ||
          tenant.email.toLowerCase().includes(searchTerm.toLowerCase()),
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
  }, [data, sortOrder, searchTerm]);

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
      return <ArrowUp className="ml-1 h-4 w-4" />;
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
            <TableHead className="h-12 px-4">{t('common.action')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className="bg-bg-base">
          {loading ? (
            <TableRow>
              <TableCell colSpan={4} className="h-24 text-center">
                <div className="flex items-center justify-center">
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-solid border-current border-r-transparent align-[-0.125em] motion-reduce:animate-[spin_1.5s_linear_infinite]"></div>
                </div>
              </TableCell>
            </TableRow>
          ) : sortedData && sortedData.length > 0 ? (
            sortedData.map((tenant) => (
              <TableRow key={tenant.tenant_id} className="hover:bg-bg-card">
                <TableCell className="p-4 flex gap-1 items-center">
                  <RAGFlowAvatar
                    isPerson
                    className="size-4"
                    avatar={tenant.avatar}
                    name={tenant.nickname}
                  />
                  {tenant.nickname}
                </TableCell>
                <TableCell className="p-4">
                  {formatDate(tenant.update_date)}
                </TableCell>
                <TableCell className="p-4">{tenant.email}</TableCell>
                <TableCell className="p-4">
                  {tenant.role === TenantRole.Invite ? (
                    <div className="flex gap-2">
                      <Button
                        variant="link"
                        className="p-0 h-auto"
                        onClick={handleAgree(tenant.tenant_id, true)}
                      >
                        {t(`setting.agree`)}
                      </Button>
                      <Button
                        variant="link"
                        className="p-0 h-auto"
                        onClick={handleAgree(tenant.tenant_id, false)}
                      >
                        {t(`setting.refuse`)}
                      </Button>
                    </div>
                  ) : tenant.role === TenantRole.Normal &&
                    user.id !== tenant.tenant_id ? (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 p-0"
                      onClick={handleQuitTenantUser(user.id, tenant.tenant_id)}
                    >
                      {/* {t('setting.quit')} */}
                      <LogOut />
                    </Button>
                  ) : null}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={4} className="h-24 text-center">
                {t('common.noData')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
};

export default TenantTable;
