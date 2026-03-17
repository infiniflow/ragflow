import message from '@/components/ui/message';
import { useExportAgentLog } from '@/hooks/use-agent-request';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { downloadFileFromBlob } from '@/utils/file-util';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

interface ISearchParams {
  keywords?: string;
  from_date?: Date;
  to_date?: Date;
  orderby?: string;
  desc?: boolean;
}

export const useExportAgentLogToCSV = () => {
  const { t } = useTranslation();
  const { id: canvasId } = useParams();
  const { exportLogs, loading } = useExportAgentLog();

  const convertToCSV = (data: IAgentLogResponse[]) => {
    const headers = [
      t('flow.id'),
      t('flow.userId'),
      t('flow.logTitle'),
      t('flow.state'),
      t('flow.number'),
      t('flow.latestDate'),
      t('flow.createDate'),
      t('flow.version.version'),
    ];

    const rows = data.map((item) => [
      item.id,
      item.user_id,
      item.message?.length ? item.message[0]?.content : '',
      item.errors ? t('flow.failed') : t('flow.success'),
      item.round,
      item.update_date,
      item.create_date,
      item.version_title,
    ]);

    const csvContent = [
      headers.join(','),
      ...rows.map((row) =>
        row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','),
      ),
    ].join('\n');

    return csvContent;
  };

  const handleExport = async (searchParams: ISearchParams) => {
    const allData = await exportLogs({
      keywords: searchParams.keywords,
      from_date: searchParams.from_date,
      to_date: searchParams.to_date,
      orderby: searchParams.orderby,
      desc: searchParams.desc,
    });

    if (allData.length === 0) {
      message.warning(t('flow.noDataToExport'));
      return;
    }

    const csvContent = convertToCSV(allData);
    // Add BOM for Excel to correctly display UTF-8
    const BOM = '\uFEFF';
    const blob = new Blob([BOM + csvContent], {
      type: 'text/csv;charset=utf-8;',
    });
    downloadFileFromBlob(
      blob,
      `agent-logs-${canvasId}-${new Date().toISOString().split('T')[0]}.csv`,
    );
  };

  return { handleExport, loading };
};
