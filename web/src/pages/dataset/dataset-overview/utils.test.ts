import { mapOverviewTotal } from './utils';

describe('mapOverviewTotal', () => {
  it('maps nested status counts to the flat overview fields', () => {
    const result = mapOverviewTotal({
      doc_num: 2576,
      status: {
        cancel_count: 0,
        done_count: 937,
        fail_count: 18,
        running_count: 767,
        unstart_count: 854,
      },
    });

    expect(result).toEqual({
      cancelled: 0,
      failed: 18,
      finished: 937,
      processing: 767,
      downloaded: 854,
    });
  });

  it('defaults every field to 0 when status is missing', () => {
    expect(mapOverviewTotal({ doc_num: 0 })).toEqual({
      cancelled: 0,
      failed: 0,
      finished: 0,
      processing: 0,
      downloaded: 0,
    });
  });

  it('defaults every field to 0 when data is undefined', () => {
    expect(mapOverviewTotal(undefined)).toEqual({
      cancelled: 0,
      failed: 0,
      finished: 0,
      processing: 0,
      downloaded: 0,
    });
  });

  it('fills only the counts present in a partial status', () => {
    expect(mapOverviewTotal({ status: { done_count: 5 } })).toEqual({
      cancelled: 0,
      failed: 0,
      finished: 5,
      processing: 0,
      downloaded: 0,
    });
  });
});
