import { parseWikiLinkHref } from './wiki-link-util';

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

  it.each([
    ['artifact/dataset/concept/人工智能', 'concept', '人工智能'],
    ['/artifact/dataset/entity/person/张角', 'entity', 'person/张角'],
    ['topic/智能体', 'topic', '智能体'],
    ['/entity/location/长社', 'entity', 'location/长社'],
  ])('preserves internal link %s', (href, pageType, slug) => {
    expect(parseWikiLinkHref(href)).toEqual({ pageType, slug });
  });

  it.each([
    'https://example.com/topic/agents',
    'http://example.com/entity/person/张角',
    '//example.com/concept/agents',
    '\\\\evil.example/topic/page',
    '/\\evil.example/topic/page',
    '\\/evil.example/topic/page',
    'https://example.com/artifact/dataset/entity/alice',
    'HTTPS://example.com/topic/agents',
    '  //example.com/topic/agents  ',
    'web+wiki.1://example.com/topic/agents',
    'mailto:user@example.com/topic/agents',
  ])('does not treat external URL %s as an internal wiki link', (href) => {
    expect(parseWikiLinkHref(href)).toBeNull();
  });
});
