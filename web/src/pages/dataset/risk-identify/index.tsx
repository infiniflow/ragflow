import { FormContainer } from '@/components/form-container';
import kbService, {
  exportRiskAIIdentifyBatch,
} from '@/services/knowledge-service';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { downloadFileFromBlob } from '@/utils/file-util';
import {
  Button,
  InputNumber,
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

export default function RiskIdentifyPage() {
  const { t } = useTranslation();
  const { id } = useParams();
  const [fileList, setFileList] = useState<any[]>([]);
  const [lastUploadInfo, setLastUploadInfo] = useState<any>(null);
  const [exporting, setExporting] = useState(false);

  // no global testing hook; per-row testing below

  type RowType = {
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
  const [rows, setRows] = useState<RowType[]>([]);
  const [expandedRowKeys, setExpandedRowKeys] = useState<React.Key[]>([]);
  const updateRow = (key: number, patch: Partial<RowType>) => {
    setRows((prev) =>
      prev.map((r) => (r.key === key ? { ...r, ...patch } : r)),
    );
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

          const mapped: RowType[] = excelRows.map((r: any, idx: number) => ({
            key: idx,
            cycle: r['循环'] ?? '',
            risk: r['主要风险点'] ?? '',
            control: pickControl(r),
            similarity_threshold: 0.6,
            vector_similarity_weight: 0.95,
            question: pickControl(r),
            aiAnswer: '',
          }));
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
        <div className="flex justify-end mb-2">
          <Button
            disabled={!rows.length}
            loading={exporting}
            onClick={async () => {
              setExporting(true);
              try {
                const payload = rows.map((r) => ({
                  循环: r.cycle,
                  主要风险点: r.risk,
                  相应的内部控制: r.question,
                  similarity_threshold: r.similarity_threshold,
                  vector_similarity_weight: r.vector_similarity_weight,
                }));
                const res = await exportRiskAIIdentifyBatch(
                  id as string,
                  payload,
                  'structured',
                );
                const blob = res?.data;
                if (!(blob instanceof Blob)) {
                  throw new Error('Invalid blob response');
                }
                downloadFileFromBlob(blob, `控制矩阵.xlsx`);
              } catch (e) {
                message.error(t('message.500'));
              } finally {
                setExporting(false);
              }
            }}
          >
            一键AI识别并导出
          </Button>
        </div>
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
  kbId,
  expandedRowKeys,
  setExpandedRowKeys,
}: {
  rows: any[];
  setRows: (r: any[]) => void;
  kbId: string;
  expandedRowKeys: React.Key[];
  setExpandedRowKeys: (k: React.Key[]) => void;
}) {
  const { t } = useTranslation();

  const updateRow = (key: number, patch: Partial<any>) => {
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };

  const handleTest = async (record: any) => {
    if (!record?.question) {
      message.warning(t('knowledgeDetails.testTextPlaceholder'));
      return;
    }
    updateRow(record.key, { testing: true });
    try {
      const { data } = await kbService.risk_retrieval({
        kb_id: kbId,
        question: record.question,
        similarity_threshold: record.similarity_threshold,
        vector_similarity_weight: record.vector_similarity_weight,
        page: 1,
        size: 10,
      });
      const res = data?.data;
      updateRow(record.key, { result: res, testing: false });
      setExpandedRowKeys(Array.from(new Set([record.key, ...expandedRowKeys])));
    } catch (e) {
      updateRow(record.key, { testing: false });
      message.error(t('message.500'));
    }
  };

  const handleAI = async (record: any) => {
    try {
      // Ensure we have retrieval results
      const ensured = await ensureRetrieval(record, kbId as string, updateRow);
      const prompt = buildAIPrompt(ensured);
      updateRow(record.key, { aiLoading: true });
      const { data } = await kbService.risk_ai_identify({
        kb_id: kbId,
        prompt,
      });
      const answer = data?.data?.answer || '';
      updateRow(record.key, { aiAnswer: answer, aiLoading: false });
      setExpandedRowKeys((prev) => Array.from(new Set([record.key, ...prev])));
    } catch (e) {
      updateRow(record.key, { aiLoading: false });
      message.error(t('message.500'));
    }
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
    {
      title: '操作',
      key: 'action',
      width: 240,
      render: (_, record) => (
        <div className="flex gap-2">
          <Button
            loading={record.testing}
            type="primary"
            onClick={() => handleTest(record)}
          >
            执行检索
          </Button>
          <Button loading={record.aiLoading} onClick={() => handleAI(record)}>
            AI识别
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Table
      columns={columns}
      dataSource={rows}
      rowKey="key"
      pagination={false}
      expandable={{
        expandedRowKeys,
        onExpandedRowsChange: (keys) => setExpandedRowKeys(keys as React.Key[]),
        expandedRowRender: (record) => (
          <div className="space-y-3">
            {record?.result?.chunks?.length ? (
              record.result.chunks.slice(0, 5).map((c: any) => (
                <FormContainer key={c.chunk_id} className="p-3">
                  <div className="text-xs text-text-secondary mb-1">
                    {c.docnm_kwd}
                  </div>
                  <div className="whitespace-pre-wrap">
                    {c.content_with_weight}
                  </div>
                </FormContainer>
              ))
            ) : (
              <span className="text-text-secondary">暂无结果</span>
            )}
            {record?.aiAnswer && (
              <FormContainer className="p-3">
                <div className="text-xs text-text-secondary mb-1">
                  AI识别结果
                </div>
                <div className="whitespace-pre-wrap">{record.aiAnswer}</div>
              </FormContainer>
            )}
          </div>
        ),
      }}
    />
  );
}

function buildAIPrompt(record: any): string {
  const cycle = record.cycle || '';
  const risk = record.risk || '';
  const control = record.question || '';
  const chunks: string[] = (record?.result?.chunks || [])
    .slice(0, 10)
    .map((c: any) => c?.content_with_weight || '')
    .filter(Boolean);
  const text = chunks.join('\n\n');
  return [
    '## 角色',
    '你是一名经验丰富的内控审计专家',
    '## 任务',
    `针对${cycle},为了防范${risk}，对“${control}”关键控制点进行审计。`,
    '### 任务1',
    '根据RAG检索出的被审计单位相关内控制度:',
    '```' + text + '```',
    '筛选出与该循环的关键控制点相关的内控制度,包含制度名称和对应原文，输出“相关制度”，不相关的制度需要排除。',
    '### 任务2',
    '根据任务1识别的相关制度，整理输出“控制活动描述”，即用一句话进行专业描述，表达要客观清晰、具备可测试性，并包含控制的目的、执行人、控制内容、频率、控制方式和留痕依据等要素，但不需要逐项分点列出，只输出一条完整规范的审计用语。',
    '### 任务3',
    '输出“控制频率”，即每年一次、每季一次、每月一次、每周一次、每日一次、每日多次。如果制度中未明确则写“待填写”。',
    '### 任务4',
    '输出“相关单据”，列出控制活动中的依据或记录，如盘点表，发货单，对账单等，单据名称用书名号包裹，如：《客户信息表》。',
    '### 任务5',
    '输出“相关人员”，列出控制活动中涉及发起、执行、审核、审批等相关人员。',
    '### 任务6',
    '输出“设计缺陷”，判断是否存在内控制度设计缺陷。如果存在设计缺陷，则输出存在设计缺陷，并简要列示缺陷名称和原因。如果不存在，则输出不存在设计缺陷。',
    '## 输出',
    '将任务中需要输出的字段以json格式输出，包含：“相关制度”，“控制活动描述”，“控制频率”，“相关单据”，“相关人员”，“设计缺陷”,均为文本格式，不要是数组。',
  ].join('\n');
}

async function ensureRetrieval(
  record: any,
  kbId: string,
  updateRow: (key: number, patch: any) => void,
) {
  if (record?.result?.chunks?.length) return record;
  updateRow(record.key, { testing: true });
  try {
    const { data } = await kbService.risk_retrieval({
      kb_id: kbId,
      question: record.question,
      similarity_threshold: record.similarity_threshold,
      vector_similarity_weight: record.vector_similarity_weight,
      page: 1,
      size: 10,
    });
    const res = data?.data;
    updateRow(record.key, { result: res, testing: false });
    return { ...record, result: res };
  } catch (e) {
    updateRow(record.key, { testing: false });
    throw e;
  }
}
