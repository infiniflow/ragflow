import { EntityNodeColor } from '@/components/artifact-force-graph/node-style';
import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { useMemo } from 'react';

const KnowledgeGraphColorCount = 10;

/**
 * Returns plain tooltip text with the original entity type and description.
 * Blank types display as `other`; the graph handles HTML escaping.
 */
export const getKnowledgeGraphNodeLabel = (node: IArtifactGraphEntity) => {
  const type =
    typeof node.type === 'string' && node.type.trim() ? node.type : 'other';
  return node.description ? `${type} · ${node.description}` : type;
};

/**
 * Returns a stable node-color callback backed by the root CSS palette.
 * Trimmed, lowercase entity types select a color; blank and `other` types use
 * the neutral token. Different types can share one of the ten palette colors.
 */
export function useKnowledgeGraphNodeColor() {
  return useMemo(() => {
    // These root tokens use the same colors in both themes. Resolve them once,
    // outside node callbacks, so Canvas and withAlpha receive real hex colors.
    const style = getComputedStyle(document.documentElement);
    const otherColor =
      style.getPropertyValue('--graph-node-other').trim() || EntityNodeColor;
    const colors = Array.from(
      { length: KnowledgeGraphColorCount },
      (_, index) =>
        style.getPropertyValue(`--graph-node-${index + 1}`).trim() ||
        otherColor,
    );

    return (node: IArtifactGraphEntity): string => {
      // Normalize the color key only. Do not merge synonyms or mutate data.
      const type =
        typeof node.type === 'string' ? node.type.trim().toLowerCase() : '';
      if (!type || type === 'other') return otherColor;

      // FNV-1a over UTF-16 code units: stable across ordering and graph subsets.
      // Ten slots intentionally allow collisions; the tooltip identifies type.
      let hash = 2166136261;
      for (let index = 0; index < type.length; index++) {
        hash = Math.imul(hash ^ type.charCodeAt(index), 16777619);
      }
      return colors[(hash >>> 0) % KnowledgeGraphColorCount];
    };
  }, []);
}
