import { renderHook } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { withAlpha } from '@/components/artifact-force-graph/utils';
import {
  getKnowledgeGraphNodeLabel,
  useKnowledgeGraphNodeColor,
} from '../use-knowledge-graph-node-style';

const Css = readFileSync(
  resolve(__dirname, '../../../../tailwind.css'),
  'utf8',
);
const Tokens = [
  ...Css.matchAll(/(--graph-node-(?:\d+|other)):\s*(#[\da-f]{6});/g),
];
const entity = (type: unknown, description = ''): IArtifactGraphEntity => ({
  slug: 'test',
  name: 'test',
  aliases: [],
  type: type as string,
  description,
  weight: 1,
});

beforeAll(() => {
  Tokens.forEach(([, name, color]) =>
    document.documentElement.style.setProperty(name, color),
  );
});
afterAll(() => {
  Tokens.forEach(([, name]) =>
    document.documentElement.style.removeProperty(name),
  );
});

describe('knowledge graph type colors', () => {
  it('uses concrete CSS palette colors compatible with existing dimming', () => {
    expect(Tokens).toHaveLength(11);
    const { result } = renderHook(useKnowledgeGraphNodeColor);
    const colors = Tokens.map(([, , color]) => color);
    const color = result.current(entity('person'));
    expect(colors).toContain(color);
    expect(withAlpha(color, 0.2)).toBe(`${color}33`);
  });

  it('stays stable for shuffled, filtered, expanded and newly mounted graphs', () => {
    const types = [
      'person',
      'organization',
      '地点',
      'time',
      'location',
      '任意类型',
      '🧭',
    ];
    const first = renderHook(useKnowledgeGraphNodeColor);
    const initial = new Map(
      types.map((type) => [type, first.result.current(entity(type))]),
    );
    for (const type of [...types].reverse()) {
      expect(first.result.current(entity(type))).toBe(initial.get(type));
    }
    first.result.current(entity('new type'));
    const second = renderHook(useKnowledgeGraphNodeColor);
    for (const type of types.slice(2, 5)) {
      expect(second.result.current(entity(type))).toBe(initial.get(type));
    }
  });

  it('normalizes only whitespace and case for colors, preserving original label text', () => {
    const { result } = renderHook(useKnowledgeGraphNodeColor);
    expect(result.current(entity(' Person '))).toBe(
      result.current(entity('person')),
    );
    const node = entity(' Person ', 'Original description');
    expect(getKnowledgeGraphNodeLabel(node)).toBe(
      ' Person  · Original description',
    );
    expect(node.type).toBe(' Person ');
    expect(node.description).toBe('Original description');
  });

  it.each([undefined, null, '', '   ', 'other', ' OTHER ', {}, 42])(
    'uses the default color for missing/other type %p',
    (type) => {
      const { result } = renderHook(useKnowledgeGraphNodeColor);
      const defaultColor = Tokens.find(
        ([, name]) => name === '--graph-node-other',
      )![2];
      expect(result.current(entity(type))).toBe(defaultColor);
    },
  );

  it('keeps arbitrary named types, including unknown, in the categorical palette', () => {
    const { result } = renderHook(useKnowledgeGraphNodeColor);
    const defaultColor = result.current(entity('other'));
    for (const type of ['unknown', 'custom type', '中文类型', '__proto__']) {
      expect(result.current(entity(type))).not.toBe(defaultColor);
      expect(getKnowledgeGraphNodeLabel(entity(type))).toBe(type);
    }
  });

  it('documents real hash collisions without changing or merging their labels', () => {
    const { result } = renderHook(useKnowledgeGraphNodeColor);
    // FNV-1a UTF-16 modulo 10 is deliberately bounded, including ordinary types.
    for (const [first, second] of [
      ['organization', '地点'],
      ['time', 'location'],
    ]) {
      expect(result.current(entity(first))).toBe(
        result.current(entity(second)),
      );
      expect(getKnowledgeGraphNodeLabel(entity(first))).not.toBe(
        getKnowledgeGraphNodeLabel(entity(second)),
      );
    }
    expect(result.current(entity('organization'))).toBe(
      Tokens.find(([, name]) => name === '--graph-node-3')![2],
    );
    expect(result.current(entity('time'))).toBe(
      Tokens.find(([, name]) => name === '--graph-node-7')![2],
    );
    // Supplementary characters hash both UTF-16 code units, not one code point.
    expect(result.current(entity('🧭'))).toBe(
      Tokens.find(([, name]) => name === '--graph-node-5')![2],
    );
  });

  it('reads the palette once and keeps the color callback stable on rerender', () => {
    const getStyle = jest.spyOn(window, 'getComputedStyle');
    const { result, rerender } = renderHook(useKnowledgeGraphNodeColor);
    const callback = result.current;
    const calls = getStyle.mock.calls.length;
    for (let index = 0; index < 100; index++) callback(entity(`type-${index}`));
    rerender();
    expect(result.current).toBe(callback);
    expect(getStyle.mock.calls).toHaveLength(calls);
    expect(calls).toBe(1);
    getStyle.mockRestore();
  });

  it('shows other for blank types and keeps descriptions without rewriting them', () => {
    expect(getKnowledgeGraphNodeLabel(entity(''))).toBe('other');
    expect(getKnowledgeGraphNodeLabel(entity(undefined, 'A & B'))).toBe(
      'other · A & B',
    );
    expect(getKnowledgeGraphNodeLabel(entity('person'))).toBe('person');
  });
});
