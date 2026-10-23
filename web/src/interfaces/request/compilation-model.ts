import {
  AssertionReviewAction,
  AssertionReviewStatus,
} from '@/interfaces/database/compilation-model';

/**
 * How much of the review queue to read.
 *
 * `dataset_ids` is a comma-joined list rather than an array because the queue is
 * read with a GET, and it is optional: a model is one template over many
 * datasets, so "the queue" means all of them unless the reader says otherwise.
 */
export interface IAssertionReviewRequestParams {
  status?: AssertionReviewStatus;
  class?: string;
  property?: string;
  source_type?: string;
  target_type?: string;
  dataset_ids?: string;
  limit?: number;
  offset?: number;
}

/**
 * One verdict in a batch.
 *
 * `dataset_id` travels with the id because an assertion id is only unique
 * inside its dataset — the same row name can be compiled into several.
 */
export interface IAssertionReviewRequestBody {
  items: {
    id: string;
    dataset_id: string;
    action: AssertionReviewAction;
    note?: string;
  }[];
}
