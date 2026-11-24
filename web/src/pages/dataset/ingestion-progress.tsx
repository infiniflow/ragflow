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

import IngestionProgressDetailButton from '@/ee/components/ingestion-progress-detail-button';
import { IngestionProgressResponse } from '@/interfaces/database/ingestion';
import { getIngestionProgress } from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import React from 'react';

import {
  shouldKeepPolling,
  TerminalStatuses,
} from './ingestion-progress-policy';

const PollIntervalMs = 5000;

export function useIngestionProgress(
  datasetId: string | undefined,
  logId: string | undefined,
  enabled: boolean,
) {
  const query = useQuery({
    queryKey: ['ingestionProgress', datasetId, logId],
    enabled: enabled && !!datasetId && !!logId,
    queryFn: async () => {
      const response = await getIngestionProgress(datasetId!, logId!);
      if (response.data?.code !== 0) {
        throw new Error(response.data?.message ?? 'failed to load progress');
      }
      return response.data.data as IngestionProgressResponse;
    },
    refetchInterval: (query) => {
      const data = query.state.data;
      return shouldKeepPolling(data?.status) ? PollIntervalMs : false;
    },
    retry: false,
  });

  return {
    data: query.data ?? null,
    error: query.isError,
    reload: () => query.refetch(),
  };
}

const UnitLabels: Record<string, string> = {
  'page:parse': '页面解析',
  'chunk:final': '文本切分',
  'chunk:keywords': '关键词提取',
  'chunk:tags': '标签提取',
  'chunk:resume': '断点恢复',
  'chunk:remaining': '信息补全',
  'chunk:embed': '向量化',
  'chunk:claim': '观点抽取',
  'entity:generate': '实体生成',
  'batch:map': '批量映射',
  'job:plan': '规划任务',
  'page:refine': '页面精炼',
  'row:extract': '行抽取',
  'row:dedup': '行去重',
};

function formatDuration(seconds: number): string {
  if (!seconds || seconds < 0) return '—';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  if (minutes < 60) return `${minutes}m${rest}s`;
  return `${Math.floor(minutes / 60)}h${minutes % 60}m`;
}

function formatUpdated(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString();
}

interface IngestionProgressSummaryProps {
  datasetId?: string;
  logId?: string;
  data: IngestionProgressResponse | null;
  error: boolean;
}

export const IngestionProgressSummary: React.FC<
  IngestionProgressSummaryProps
> = ({ datasetId, logId, data, error }) => {
  if (error || !data) {
    return (
      <div className="text-text-secondary text-sm">
        进度信息暂不可用，请查看完整日志。
      </div>
    );
  }

  const terminal = TerminalStatuses.has(data.status);
  const percent = Math.max(0, Math.min(100, data.percent || 0));

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-text-secondary text-sm">状态</span>
        <span className="text-text-primary text-sm font-medium">
          {terminal ? '已完成' : '解析中'}
        </span>
      </div>

      {terminal ? (
        <>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">总耗时</span>
            <span className="text-text-primary text-sm">
              {formatDuration(data.duration_s)}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">生成 chunk</span>
            <span className="text-text-primary text-sm">{data.chunk_num}</span>
          </div>
          <IngestionProgressDetailButton datasetId={datasetId} logId={logId} />
        </>
      ) : data.fallback ? (
        <div className="text-text-secondary text-sm">
          详细进度暂不可用
          <span className="ml-2">· 耗时 {formatDuration(data.duration_s)}</span>
        </div>
      ) : (
        <>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">当前阶段</span>
            <span className="text-text-primary text-sm font-medium">
              {data.stage}
              {data.unit ? ` · ${UnitLabels[data.unit] ?? data.unit}` : ''}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center justify-between text-sm">
              <span className="text-text-secondary">处理进度</span>
              <span className="text-text-primary">
                {data.current} / {data.total} ({percent.toFixed(0)}%)
              </span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded bg-bg-card">
              <div
                className="h-full rounded bg-primary"
                style={{ width: `${percent}%` }}
              />
            </div>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">耗时</span>
            <span className="text-text-primary text-sm">
              {formatDuration(data.duration_s)}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">更新于</span>
            <span className="text-text-primary text-sm">
              {formatUpdated(data.updated_at)}
            </span>
          </div>
          <IngestionProgressDetailButton datasetId={datasetId} logId={logId} />
        </>
      )}
    </div>
  );
};
