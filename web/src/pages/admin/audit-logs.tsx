import { useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useDebounce } from 'ahooks';
import dayjs from 'dayjs';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  listOperationLogs,
  type AdminAuditParams,
} from '@/services/admin-service';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import AdminRecordTable from './components/record-table';

const AdminAuditKeys = {
  list: (params: AdminAuditParams) => ['admin', 'audit', params] as const,
};

const AuditActions = [
  'admin.login',
  'admin.logout',
  'user.create',
  'user.delete',
  'user.password.reset',
  'user.status.update',
  'user.admin.grant',
  'user.admin.revoke',
] as const;
const AuditResults = ['started', 'success', 'failure', 'unknown'] as const;

export default function AdminAuditLogs() {
  const { t } = useTranslation();
  const [pagination, setPagination] = useState({ page: 1, pageSize: 20 });
  const [keyword, setKeyword] = useState('');
  const [action, setAction] = useState('all');
  const [result, setResult] = useState('all');
  const debouncedKeyword = useDebounce(keyword, { wait: 300 });
  const params: AdminAuditParams = {
    page: pagination.page,
    page_size: pagination.pageSize,
    keyword: debouncedKeyword,
    action: action === 'all' ? '' : action,
    result: result === 'all' ? '' : result,
  };
  const { data, isFetching, refetch } = useQuery({
    queryKey: AdminAuditKeys.list(params),
    queryFn: async () => (await listOperationLogs(params)).data,
    placeholderData: keepPreviousData,
    retry: false,
  });
  function resetPage() {
    setPagination((previous) => ({ ...previous, page: 1 }));
  }
  function handleKeyword(event: ChangeEvent<HTMLInputElement>) {
    setKeyword(event.target.value);
    resetPage();
  }
  function handleAction(value: string) {
    setAction(value);
    resetPage();
  }
  function handleResult(value: string) {
    setResult(value);
    resetPage();
  }
  function handlePage(page: number, pageSize: number) {
    setPagination({ page, pageSize });
  }
  function handleRefresh() {
    void refetch();
  }
  function actionLabel(value: string) {
    return t('adminEnhancements.actions.' + value.replaceAll('.', '_'), {
      defaultValue: value,
    });
  }
  return (
    <section className="h-full p-8 overflow-auto">
      <Card>
        <CardHeader className="space-y-4">
          <CardTitle>{t('adminEnhancements.audit')}</CardTitle>
          <div className="flex flex-wrap gap-3">
            <Input
              className="max-w-xs"
              value={keyword}
              onChange={handleKeyword}
              placeholder={t('adminEnhancements.auditSearch')}
            />
            <Select value={action} onValueChange={handleAction}>
              <SelectTrigger className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('adminEnhancements.allActions')}
                </SelectItem>
                {AuditActions.map((value) => (
                  <SelectItem key={value} value={value}>
                    {actionLabel(value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={result} onValueChange={handleResult}>
              <SelectTrigger className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('adminEnhancements.allResults')}
                </SelectItem>
                {AuditResults.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t('adminEnhancements.results.' + value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              onClick={handleRefresh}
              disabled={isFetching}
            >
              {t('adminEnhancements.refresh')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <AdminRecordTable
            headers={[
              'time',
              'actor',
              'target',
              'action',
              'result',
              'sourceIP',
              'duration',
            ].map((name) => t('adminEnhancements.' + name))}
            rows={(data?.data ?? []).map((entry) => ({
              id: entry.id,
              cells: [
                dayjs(entry.event_time).format('YYYY-MM-DD HH:mm:ss'),
                entry.actor_email || '—',
                entry.target_email || '—',
                actionLabel(entry.action),
                t('adminEnhancements.results.' + entry.result),
                entry.ip_address || '—',
                entry.duration_ms + ' ms',
              ],
            }))}
            loading={isFetching}
            total={data?.total ?? 0}
            page={pagination.page}
            pageSize={pagination.pageSize}
            onPageChange={handlePage}
          />
        </CardContent>
      </Card>
    </section>
  );
}
