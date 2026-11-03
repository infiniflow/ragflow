import nextRequest from '@/utils/next-request';
import { applyDatasetParserConfig } from './knowledge-service';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));
jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: {},
}));
jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: {},
}));

it('applies saved dataset parser settings with a bodyless POST', async () => {
  await applyDatasetParserConfig('dataset-1');

  expect(nextRequest).toHaveBeenCalledWith({
    url: '/api/v1/datasets/dataset-1/documents/apply-parser-config',
    method: 'post',
    datasetId: 'dataset-1',
    skipGlobalErrorNotification: true,
  });
  expect(jest.mocked(nextRequest).mock.calls[0][0]).not.toHaveProperty('data');
});
