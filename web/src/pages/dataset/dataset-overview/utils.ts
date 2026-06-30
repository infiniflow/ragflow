import { IOverviewTotal } from './interface';

export interface IIngestionStatus {
  cancel_count?: number;
  done_count?: number;
  fail_count?: number;
  running_count?: number;
  unstart_count?: number;
}

export interface IIngestionSummary {
  doc_num?: number;
  chunk_num?: number;
  token_num?: number;
  status?: IIngestionStatus;
}

// The `ingestions/summary` API nests the counts under `status` with
// `*_count` keys. The overview UI reads flat fields, so translate here
// to keep the two sides decoupled.
export function mapOverviewTotal(data?: IIngestionSummary): IOverviewTotal {
  const status = data?.status ?? {};
  return {
    cancelled: status.cancel_count ?? 0,
    failed: status.fail_count ?? 0,
    finished: status.done_count ?? 0,
    processing: status.running_count ?? 0,
    downloaded: status.unstart_count ?? 0,
  };
}
