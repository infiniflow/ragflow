import { FormContainer } from '@/components/form-container';
import { useRiskAiTask } from '@/hooks/use-risk-ai-task';
import { IRiskAITask } from '@/interfaces/knowledge/risk-ai-task';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { downloadFileFromBlob } from '@/utils/file-util';
import {
  Alert,
  Button,
  InputNumber,
  Progress,
  Space,
  Table,
  TableProps,
  Tooltip,
  Upload,
  message,
} from 'antd';
import Dragger from 'antd/es/upload/Dragger';
import { UploadCloud } from 'lucide-react';
import { useState } from 'react';
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
  } = useRiskAiTask();

  // no global testing hook; per-row testing below
  const [rows, setRows] = useState<RiskIdentifyRow[]>([]);
  const [expandedRowKeys, setExpandedRowKeys] = useState<React.Key[]>([]);

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
              similarity_threshold: 0.6,
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
        <h2 className="text-lg font-bold mb-2">
          {t('knowledgeDetails.riskIdentify')}
        </h2>
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
        />
        {error && (
          <Alert message={error} type="error" showIcon className="mb-2" />
        )}
        {task && (
          <TaskStatusCard task={task} onRetry={() => pollStatus(task.id)} />
        )}
        <RowTable
          rows={rows}
          setRows={setRows}
          kbId={id as string}
          expandedRowKeys={expandedRowKeys}
          setExpandedRowKeys={setExpandedRowKeys}
        />
      </section>
    </div>
  );
}

function RowTable({
  rows,
  setRows,
}: {
  rows: RiskIdentifyRow[];
  setRows: (r: RiskIdentifyRow[]) => void;
  kbId: string;
  expandedRowKeys: React.Key[];
  setExpandedRowKeys: (k: React.Key[]) => void;
}) {
  const updateRow = (key: number, patch: Partial<RiskIdentifyRow>) => {
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };

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
    {
      title: '相似度阈值',
      dataIndex: 'similarity_threshold',
      width: 140,
      render: (_, record) => (
        <InputNumber
          min={0}
          max={1}
          step={0.01}
          value={record.similarity_threshold}
          onChange={(v) =>
            updateRow(record.key, { similarity_threshold: Number(v ?? 0) })
          }
        />
      ),
    },
    {
      title: '向量相似度权重',
      dataIndex: 'vector_similarity_weight',
      width: 160,
      render: (_, record) => (
        <InputNumber
          min={0}
          max={1}
          step={0.01}
          value={record.vector_similarity_weight}
          onChange={(v) =>
            updateRow(record.key, { vector_similarity_weight: Number(v ?? 0) })
          }
        />
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
};

const TaskToolbar = ({
  rows,
  kbId,
  startTask,
  loading,
  disabled,
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
      similarity_threshold: r.similarity_threshold,
      vector_similarity_weight: r.vector_similarity_weight,
    }));
    await startTask(kbId, payload, { parser_type: 'structured' });
  };

  return (
    <div className="flex justify-end mb-3">
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
  );
};

type TaskStatusCardProps = {
  task: IRiskAITask;
  onRetry: () => void;
};

const TaskStatusCard = ({ task, onRetry }: TaskStatusCardProps) => {
  const [downloading, setDownloading] = useState(false);
  const progress = Math.min(100, Math.round(task.progress || 0));

  const handleDownload = async () => {
    if (!task.download_url) {
      message.error('无可用的下载地址');
      return;
    }
    try {
      setDownloading(true);
      const res = await fetch(task.download_url, {
        headers: {
          Authorization: getAuthorization(),
        },
        credentials: 'include',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const blob = await res.blob();
      downloadFileFromBlob(blob, '控制矩阵.xlsx');
    } catch (e) {
      message.error((e as Error)?.message || '下载失败');
    } finally {
      setDownloading(false);
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
        {task.error_msg && (
          <Alert message={task.error_msg} type="error" showIcon />
        )}
        {task.status === 'success' && task.download_url && (
          <Button
            loading={downloading}
            onClick={handleDownload}
            type="primary"
            size="small"
          >
            下载结果
          </Button>
        )}
      </Space>
    </FormContainer>
  );
};
