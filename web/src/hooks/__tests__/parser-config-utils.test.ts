import { extractRaptorConfigExt } from '../parser-config-utils';

describe('extractRaptorConfigExt', () => {
  it('defaults legacy RAPTOR configs to GMM clustering', () => {
    expect(
      extractRaptorConfigExt({
        use_raptor: true,
        prompt: 'summarize',
      }),
    ).toMatchObject({
      use_raptor: true,
      prompt: 'summarize',
      clustering_method: 'gmm',
    });
  });

  it('preserves explicit AHC clustering', () => {
    expect(
      extractRaptorConfigExt({
        use_raptor: true,
        clustering_method: 'ahc',
      }),
    ).toMatchObject({
      use_raptor: true,
      clustering_method: 'ahc',
    });
  });
});
