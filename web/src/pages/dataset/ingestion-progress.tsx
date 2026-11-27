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
import { useTranslation } from 'react-i18next';
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
  // No run identity (or a hidden modal) means progress does not apply to this
  // consumer. Track it explicitly so the summary can render nothing instead
  // of a misleading "unavailable" for memory logs and the first fetch frame.
  const idle = !enabled || !datasetId || !logId;
  const query = useQuery({
    queryKey: ['ingestionProgress', datasetId, logId],
    enabled: !idle,
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
    idle,
    loading: !idle && query.isPending,
    reload: () => query.refetch(),
  };
}

// Authoritative run statuses use both legacy numeric values and the current
// named values. Map each terminal status to the label the user should see;
// polling still keys off TerminalStatuses, not this display mapping.
const TerminalStatusLabels: Record<string, string> = {
  '2': 'stopped',
  '3': 'completed',
  '4': 'failed',
  COMPLETED: 'completed',
  FAILED: 'failed',
  STOPPED: 'stopped',
  DONE: 'completed',
  FAIL: 'failed',
  CANCEL: 'stopped',
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
  idle?: boolean;
  loading?: boolean;
}

export const IngestionProgressSummary: React.FC<
  IngestionProgressSummaryProps
> = ({ datasetId, logId, data, error, idle, loading }) => {
  const { t } = useTranslation();

  // No progress source, or the first fetch has not resolved: render nothing.
  // Only a real fetch failure is presented as "unavailable".
  if (idle || loading) {
    return null;
  }
  if (error || !data) {
    return (
      <div className="text-text-secondary text-sm">
        {t('knowledgeDetails.ingestionProgress.unavailable')}
      </div>
    );
  }

  const terminal = TerminalStatuses.has(data.status);
  const percent = Math.max(0, Math.min(100, data.percent || 0));
  const statusLabel = terminal
    ? t(
        `knowledgeDetails.ingestionProgress.${TerminalStatusLabels[data.status] ?? 'completed'}`,
      )
    : t('knowledgeDetails.ingestionProgress.parsing');
  const unitLabel = data.unit
    ? t(`knowledgeDetails.ingestionProgress.units.${data.unit}`, {
        // Unit names contain ':' (e.g. page:parse); the default nsSeparator
        // would split them into an unknown namespace and show the raw key.
        nsSeparator: false,
        defaultValue: data.unit,
      })
    : '';

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-text-secondary text-sm">
          {t('knowledgeDetails.ingestionProgress.status')}
        </span>
        <span className="text-text-primary text-sm font-medium">
          {statusLabel}
        </span>
      </div>

      {terminal ? (
        <>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">
              {t('knowledgeDetails.ingestionProgress.totalDuration')}
            </span>
            <span className="text-text-primary text-sm">
              {formatDuration(data.duration_s)}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">
              {t('knowledgeDetails.ingestionProgress.chunks')}
            </span>
            <span className="text-text-primary text-sm">{data.chunk_num}</span>
          </div>
          <IngestionProgressDetailButton datasetId={datasetId} logId={logId} />
        </>
      ) : data.fallback ? (
        <div className="flex flex-col gap-1 text-sm">
          {data.stage ? (
            <div className="flex items-center justify-between">
              <span className="text-text-secondary">
                {t('knowledgeDetails.ingestionProgress.currentStage')}
              </span>
              <span className="text-text-primary font-medium min-w-0 break-words">
                {data.stage}
              </span>
            </div>
          ) : null}
          <div className="text-text-secondary">
            {t('knowledgeDetails.ingestionProgress.detailUnavailable')}
            <span className="ml-2">
              · {t('knowledgeDetails.ingestionProgress.elapsed')}{' '}
              {formatDuration(data.duration_s)}
            </span>
          </div>
        </div>
      ) : (
        <>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">
              {t('knowledgeDetails.ingestionProgress.currentStage')}
            </span>
            <span className="text-text-primary text-sm font-medium min-w-0 break-words">
              {data.stage}
              {unitLabel ? ` · ${unitLabel}` : ''}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center justify-between text-sm">
              <span className="text-text-secondary">
                {t('knowledgeDetails.ingestionProgress.progress')}
              </span>
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
            <span className="text-text-secondary text-sm">
              {t('knowledgeDetails.ingestionProgress.elapsed')}
            </span>
            <span className="text-text-primary text-sm">
              {formatDuration(data.duration_s)}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-text-secondary text-sm">
              {t('knowledgeDetails.ingestionProgress.updatedAt')}
            </span>
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
