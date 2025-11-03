import { FormContainer } from '@/components/form-container';
import { useRiskAiTask } from '@/hooks/use-risk-ai-task';
import { IRiskAITask } from '@/interfaces/knowledge/risk-ai-task';
import kbService, {
  downloadRiskIdentifyTemplate,
  getRiskAITaskList,
} from '@/services/knowledge-service';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { formatDate } from '@/utils/date';
import { downloadFileFromBlob } from '@/utils/file-util';
import {
  Alert,
  Button,
  Drawer,
  InputNumber,
  Progress,
  Space,
  Table,
  TableProps,
  Tag,
  Tooltip,
  Upload,
  message,
} from 'antd';
import Dragger from 'antd/es/upload/Dragger';
import { Download, History, UploadCloud } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';

type RiskIdentifyRow = {
  key: number;
  cycle: string;
  risk: string;
  control: string;
  similarity_threshold: number;
  vector_similarity_weight: number;
  question: string;
  testing?: boolean;
  result?: any;
  aiLoading?: boolean;
  aiAnswer?: string;
};

export default function RiskIdentifyPage() {
  const { t } = useTranslation();
  const { id } = useParams();
  const [fileList, setFileList] = useState<any[]>([]);
  const [lastUploadInfo, setLastUploadInfo] = useState<any>(null);
  const {
    task,
    error,
    loading: taskLoading,
    startTask,
    pollStatus,
    downloadResult,
    retryFailedRows,
  } = useRiskAiTask();

  // no global testing hook; per-row testing below
  const [rows, setRows] = useState<RiskIdentifyRow[]>([]);
  const [expandedRowKeys, setExpandedRowKeys] = useState<React.Key[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  // Global settings for all rows
  const [similarityThreshold, setSimilarityThreshold] = useState(0.4);
  const vectorSimilarityWeight = 0.95; // Fixed value, not displayed in UI
  const [downloadingTemplate, setDownloadingTemplate] = useState(false);
  const [kbName, setKbName] = useState<string>('');

  useEffect(() => {
    const fetchKbName = async () => {
      try {
        if (!id) return;
        const { data } = await (kbService as any).get_kb_detail({ kb_id: id });
        const info = data?.data || data;
        if (info?.name) setKbName(info.name);
      } catch (_) {
        // ignore
      }
    };
    fetchKbName();
  }, [id]);

  const handleDownloadTemplate = async () => {
    try {
      setDownloadingTemplate(true);
      const response = await downloadRiskIdentifyTemplate();

      // Handle blob response similar to downloadResult
      let blob: Blob;
      if (response instanceof Blob) {
        blob = response;
      } else if ((response as any)?.data instanceof Blob) {
        blob = (response as any).data;
      } else if (response instanceof Response) {
        blob = await response.blob();
      } else {
        throw new Error('Invalid response type for file download');
      }

      downloadFileFromBlob(blob, '内控矩阵模板.xlsx');
      message.success(t('模板下载成功'));
    } catch (err) {
      message.error((err as Error).message || t('模板下载失败'));
    } finally {
      setDownloadingTemplate(false);
    }
  };

  const props = {
    name: 'data',
    multiple: false,
    accept: '.xlsx',
    maxCount: 1,
    action: api.get_risk_identify,
    headers: { Authorization: getAuthorization() },
    data: () => ({ knowledge_base_id: `${id}` }),
    beforeUpload: (file: File) => {
      if (fileList.length >= 1) {
        message.warning(t('仅支持上传一个文件'));
        return Upload.LIST_IGNORE as any;
      }
      const isXlsx =
        file.type ===
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
      if (!isXlsx) {
        message.error(t('只支持上传xlsx文件'));
        return Upload.LIST_IGNORE as any;
      }
      // 允许 Dragger 自动上传以展示内置进度条
      setFileList([file]);
      return true;
    },
    onRemove: () => {
      setFileList([]);
    },
    onChange: (info: any) => {
      // keep fileList in sync to render built-in progress
      const list = Array.isArray(info.fileList) ? info.fileList.slice(-1) : [];
      setFileList(list);
      const { status, response } = info.file || {};
      if (status === 'uploading') return;
      if (status === 'done') {
        message.success(t('文件上传成功'));
        const storage = response?.data?.storage;
        setLastUploadInfo(storage ?? null);
        try {
          const excelRows = (response?.data?.rows ||
            response?.data?.sheets?.[0]?.rows ||
            []) as any[];

          const pickControl = (obj: Record<string, any>): string => {
            const candidates = [
              '相应的内部控制',
              '对应的内部控制',
              '相关的内部控制',
              '内部控制',
            ];
            for (const k of candidates) {
              const v = obj?.[k];
              if (typeof v === 'string' && v.trim()) return v.trim();
            }
            // fallback: fuzzy match any key that contains "内部控制"
            const hit = Object.keys(obj || {}).find(
              (k) => k && k.includes('内部控制'),
            );
            if (hit && typeof obj[hit] === 'string')
              return (obj[hit] as string).trim();
            return '';
          };

          const mapped: RiskIdentifyRow[] = excelRows.map(
            (r: any, idx: number) => ({
              key: idx,
              cycle: r['循环'] ?? '',
              risk: r['主要风险点'] ?? '',
              control: pickControl(r),
              similarity_threshold: 0.4,
              vector_similarity_weight: 0.95,
              question: pickControl(r),
              aiAnswer: '',
            }),
          );
          setRows(mapped);
        } catch (e) {
          // ignore
        }
      } else if (status === 'error') {
        message.error(t('文件上传失败'));
      }
    },
    fileList,
    showUploadList: true,
  };

  return (
    <div className="p-5">
      <section className="mb-4">
        <div className="flex justify-between items-center mb-2">
          <h2 className="text-lg font-bold">
            {t('knowledgeDetails.riskIdentify')}
          </h2>
          <Button
            icon={<Download className="size-4" />}
            onClick={handleDownloadTemplate}
            loading={downloadingTemplate}
          >
            {t('下载模板')}
          </Button>
        </div>
        <FormContainer className="p-4">
          <Dragger
            {...props}
            className="rounded-lg border border-dashed border-input bg-bg-input/40 hover:bg-bg-input/60 transition-colors"
          >
            <div className="py-6 flex flex-col items-center justify-center gap-2">
              <UploadCloud className="size-10 text-primary" />
              <div className="text-base font-medium">
                {t('拖拽文件到此，或点击选择')}
              </div>
              <div className="text-xs text-text-secondary">
                {t('仅支持 .xlsx，合并单元格将自动处理')}
              </div>
            </div>
          </Dragger>

          {lastUploadInfo && (
            <div className="mt-3 text-xs text-text-secondary">
              {t('已保存到对象存储')}: {lastUploadInfo.bucket}/
              {lastUploadInfo.location}
            </div>
          )}
        </FormContainer>
      </section>

      <section className="h-full">
        <TaskToolbar
          rows={rows}
          kbId={id as string}
          startTask={startTask}
          loading={taskLoading}
          disabled={!rows.length}
          onShowHistory={() => setShowHistory(true)}
          similarityThreshold={similarityThreshold}
          setSimilarityThreshold={setSimilarityThreshold}
          vectorSimilarityWeight={vectorSimilarityWeight}
        />
        {error && (
          <Alert message={error} type="error" showIcon className="mb-2" />
        )}
        {task && (
          <TaskStatusCard
            task={task}
            onRetry={() => pollStatus(task.id)}
            onDownload={() =>
              downloadResult(task.id, `${kbName || id}-生成内控矩阵.xlsx`)
            }
            onRetryFailed={() => retryFailedRows(task.id)}
          />
        )}
        <RowTable
          rows={rows}
          kbId={id as string}
          expandedRowKeys={expandedRowKeys}
          setExpandedRowKeys={setExpandedRowKeys}
        />
      </section>

      <TaskHistoryDrawer
        open={showHistory}
        onClose={() => setShowHistory(false)}
        kbId={id as string}
        onSelectTask={(taskId) => {
          setShowHistory(false);
          pollStatus(taskId);
        }}
        downloadResult={downloadResult}
      />
    </div>
  );
}

function RowTable({
  rows,
}: {
  rows: RiskIdentifyRow[];
  kbId: string;
  expandedRowKeys: React.Key[];
  setExpandedRowKeys: (k: React.Key[]) => void;
}) {
  const columns: TableProps<any>['columns'] = [
    { title: '序号', dataIndex: 'key', width: 70, render: (_, r) => r.key + 1 },
    { title: '循环', dataIndex: 'cycle', width: 140, ellipsis: true },
    {
      title: '主要风险点',
      dataIndex: 'risk',
      width: 300,
      render: (text: string) => (
        <Tooltip title={text} placement="topLeft">
          <div className="max-w-[280px] whitespace-nowrap overflow-hidden text-ellipsis">
            {text || '-'}
          </div>
        </Tooltip>
      ),
    },
    {
      title: '相应的内部控制（Question）',
      dataIndex: 'question',
      width: 360,
      render: (text: string) => (
        <Tooltip title={text} placement="topLeft">
          <div className="max-w-[340px] whitespace-nowrap overflow-hidden text-ellipsis">
            {text || '-'}
          </div>
        </Tooltip>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={rows}
      rowKey="key"
      pagination={false}
    />
  );
}

type TaskToolbarProps = {
  rows: RiskIdentifyRow[];
  kbId: string;
  startTask: (
    kbId: string,
    rows: any[],
    options?: Record<string, any>,
  ) => Promise<void>;
  loading: boolean;
  disabled?: boolean;
  onShowHistory: () => void;
  similarityThreshold: number;
  setSimilarityThreshold: (v: number) => void;
  vectorSimilarityWeight: number;
};

const TaskToolbar = ({
  rows,
  kbId,
  startTask,
  loading,
  disabled,
  onShowHistory,
  similarityThreshold,
  setSimilarityThreshold,
  vectorSimilarityWeight,
}: TaskToolbarProps) => {
  const { t } = useTranslation();
  const handleCreateTask = async () => {
    if (!rows.length) {
      message.warning(t('请先上传并解析数据'));
      return;
    }
    const payload = rows.map((r) => ({
      循环: r.cycle,
      主要风险点: r.risk,
      相应的内部控制: r.question,
      similarity_threshold: similarityThreshold,
      vector_similarity_weight: vectorSimilarityWeight,
    }));
    await startTask(kbId, payload, { parser_type: 'structured' });
  };

  return (
    <div className="mb-3">
      <div className="flex justify-between mb-3">
        <Button icon={<History className="size-4" />} onClick={onShowHistory}>
          {t('历史记录')}
        </Button>
        <Space>
          <Button
            type="primary"
            onClick={handleCreateTask}
            loading={loading}
            disabled={disabled}
          >
            {t('一键AI识别')}
          </Button>
        </Space>
      </div>

      <FormContainer className="p-4">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{t('相似度阈值')}:</span>
            <InputNumber
              min={0}
              max={1}
              step={0.01}
              value={similarityThreshold}
              onChange={(v) => setSimilarityThreshold(Number(v ?? 0.4))}
              style={{ width: 120 }}
            />
          </div>
          <div className="text-xs text-text-secondary">
            {t('该设置将应用于所有数据行')}
          </div>
        </div>
      </FormContainer>
    </div>
  );
};

type TaskStatusCardProps = {
  task: IRiskAITask;
  onRetry: () => void;
  onDownload: () => void;
  onRetryFailed: () => void;
};

const TaskStatusCard = ({
  task,
  onRetry,
  onDownload,
  onRetryFailed,
}: TaskStatusCardProps) => {
  const [downloading, setDownloading] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const progress = Math.min(100, Math.round(task.progress || 0));

  const handleDownload = async () => {
    try {
      setDownloading(true);
      await onDownload();
    } catch (e) {
      message.error((e as Error)?.message || '下载失败');
    } finally {
      setDownloading(false);
    }
  };

  const handleRetryFailed = async () => {
    try {
      setRetrying(true);
      await onRetryFailed();
      message.success('已重新提交失败的行任务');
    } catch (e) {
      message.error((e as Error)?.message || '重试失败');
    } finally {
      setRetrying(false);
    }
  };

  return (
    <FormContainer className="p-3 mb-3">
      <Space direction="vertical" className="w-full">
        <div className="flex justify-between items-center">
          <span className="font-medium text-base">任务状态</span>
          <Space>
            <span>{task.status}</span>
            {task.status !== 'success' && (
              <Button size="small" onClick={onRetry}>
                刷新状态
              </Button>
            )}
          </Space>
        </div>
        <Progress
          percent={progress}
          status={task.status === 'failed' ? 'exception' : 'active'}
        />
        <div className="text-xs text-text-secondary">
          {`进度: ${task.processed_rows || 0}/${task.total_rows || 0}, 失败: ${
            task.failed_rows || 0
          }`}
        </div>

        {/* Enhanced row status information */}
        {task.row_status_counts && (
          <div className="text-xs text-text-secondary">
            <div>行状态统计:</div>
            <div className="ml-2">
              待处理: {task.row_status_counts.pending}, 处理中:{' '}
              {task.row_status_counts.running}, 成功:{' '}
              {task.row_status_counts.success}, 失败:{' '}
              {task.row_status_counts.failed}
            </div>
          </div>
        )}

        {/* Failed rows details */}
        {task.failed_rows_detail && task.failed_rows_detail.length > 0 && (
          <div className="text-xs">
            <div className="text-red-600 mb-1">失败行详情:</div>
            {task.failed_rows_detail.slice(0, 3).map((row, idx) => (
              <div key={idx} className="ml-2 text-red-500">
                行 {row.row_index + 1}: {row.error_msg}
              </div>
            ))}
            {task.failed_rows_detail.length > 3 && (
              <div className="ml-2 text-red-500">
                ... 还有 {task.failed_rows_detail.length - 3} 行失败
              </div>
            )}
          </div>
        )}

        {task.error_msg && (
          <Alert message={task.error_msg} type="error" showIcon />
        )}

        <div className="flex gap-2">
          {task.status === 'success' && (
            <Button
              loading={downloading}
              onClick={handleDownload}
              type="primary"
              size="small"
            >
              下载结果
            </Button>
          )}
          {task.failed_rows_detail && task.failed_rows_detail.length > 0 && (
            <Button loading={retrying} onClick={handleRetryFailed} size="small">
              重试失败行
            </Button>
          )}
        </div>
      </Space>
    </FormContainer>
  );
};

type TaskHistoryDrawerProps = {
  open: boolean;
  onClose: () => void;
  kbId: string;
  onSelectTask: (taskId: string) => void;
  downloadResult: (taskId: string, filename?: string) => void;
};

const TaskHistoryDrawer = ({
  open,
  onClose,
  kbId,
  onSelectTask,
  downloadResult,
}: TaskHistoryDrawerProps) => {
  const { t } = useTranslation();
  const [tasks, setTasks] = useState<IRiskAITask[]>([]);
  const [loading, setLoading] = useState(false);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0,
  });
  const [kbName, setKbName] = useState<string>('');

  const fetchTasks = async (page: number = 1) => {
    if (!kbId) return;
    setLoading(true);
    try {
      const res = await getRiskAITaskList(kbId, page, pagination.pageSize);
      const payload = res?.data || res;
      if (payload?.code === 0 || payload?.tasks) {
        const data = payload.data || payload;
        setTasks(data.tasks || []);
        setPagination({
          current: data.page || page,
          pageSize: data.page_size || pagination.pageSize,
          total: data.total || 0,
        });
      }
    } catch (err) {
      message.error('加载历史记录失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open && kbId) {
      fetchTasks(1);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, kbId]);

  useEffect(() => {
    const fetchKbName = async () => {
      try {
        if (!kbId) return;
        const { data } = await (kbService as any).get_kb_detail({
          kb_id: kbId,
        });
        const info = data?.data || data;
        if (info?.name) setKbName(info.name);
      } catch (_) {
        // ignore
      }
    };
    fetchKbName();
  }, [kbId]);

  const getStatusTag = (status: string) => {
    const statusMap: Record<string, { color: string; text: string }> = {
      pending: { color: 'blue', text: '待处理' },
      running: { color: 'processing', text: '处理中' },
      success: { color: 'success', text: '成功' },
      failed: { color: 'error', text: '失败' },
    };
    const config = statusMap[status] || { color: 'default', text: status };
    return <Tag color={config.color}>{config.text}</Tag>;
  };

  const columns = [
    {
      title: '名称',
      key: 'display_name',
      width: 260,
      render: (_: any, __: IRiskAITask, index: number) => {
        // Newer records should have larger sequence numbers
        const pos = (pagination.current - 1) * pagination.pageSize + index + 1;
        const total = pagination.total || 0;
        const seq = total > 0 ? total - pos + 1 : pos;
        const prefix = kbName || kbId || '';
        return (
          <span className="text-text-primary">
            {`${prefix}-生成内控矩阵-${seq}`}
          </span>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '进度',
      key: 'progress',
      width: 150,
      render: (_: any, record: IRiskAITask) => (
        <div className="flex items-center gap-2">
          <Progress
            percent={Math.min(100, Math.round(record.progress || 0))}
            size="small"
            status={record.status === 'failed' ? 'exception' : 'normal'}
            style={{ width: 80 }}
          />
          <span className="text-xs">
            {record.processed_rows || 0}/{record.total_rows || 0}
          </span>
        </div>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'create_time',
      key: 'create_time',
      width: 180,
      render: (time: number) => (time ? formatDate(time) : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_: any, record: IRiskAITask) => (
        <Space size="small">
          <Button
            size="small"
            onClick={() => onSelectTask(record.id)}
            disabled={!['running', 'pending'].includes(record.status)}
          >
            查看
          </Button>
          {record.status === 'success' && record.result_location && (
            <Button
              size="small"
              type="primary"
              onClick={() => {
                const index = tasks.findIndex((t) => t.id === record.id);
                const pos =
                  (pagination.current - 1) * pagination.pageSize +
                  (index >= 0 ? index : 0) +
                  1;
                const total = pagination.total || tasks.length || 0;
                const seq = total > 0 ? total - pos + 1 : pos;
                const prefix = kbName || kbId || '';
                const filename = `${prefix}-生成内控矩阵-${seq}.xlsx`;
                downloadResult(record.id, filename);
              }}
            >
              下载
            </Button>
          )}
        </Space>
      ),
    },
  ];

  return (
    <Drawer
      title={t('任务历史记录')}
      placement="right"
      width={800}
      onClose={onClose}
      open={open}
    >
      <Table
        columns={columns}
        dataSource={tasks}
        rowKey="id"
        loading={loading}
        pagination={{
          ...pagination,
          onChange: (page) => fetchTasks(page),
          showSizeChanger: false,
          showTotal: (total) => `共 ${total} 条`,
        }}
      />
    </Drawer>
  );
};
