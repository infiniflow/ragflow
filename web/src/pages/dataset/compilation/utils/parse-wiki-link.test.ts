import { parseWikiLinkHref } from './parse-wiki-link';

describe('parseWikiLinkHref', () => {
  it('preserves nested typed slugs in artifact links', () => {
    expect(
      parseWikiLinkHref(
        'artifact/fb9bfae2a00b43e59ecaea5e86d90c91/entity/person/张角',
      ),
    ).toEqual({ pageType: 'entity', slug: 'person/张角' });
  });

  it('preserves nested typed slugs in simple links', () => {
    expect(parseWikiLinkHref('entity/location/长社')).toEqual({
      pageType: 'entity',
      slug: 'location/长社',
    });
  });
});
