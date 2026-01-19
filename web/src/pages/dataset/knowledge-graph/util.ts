import { isEmpty } from 'lodash';
import { v4 as uuid } from 'uuid';

class KeyGenerator {
  idx = 0;
  chars: string[] = [];
  constructor() {
    const chars = Array(26)
      .fill(1)
      .map((x, idx) => String.fromCharCode(97 + idx)); // 26 char
    this.chars = chars;
  }
  generateKey() {
    const key = this.chars[this.idx];
    this.idx++;
    return key;
  }
}

// Classify nodes based on edge relationships
export class Converter {
  keyGenerator;
  dict: Record<string, string> = {}; // key is node id, value is combo
  constructor() {
    this.keyGenerator = new KeyGenerator();
  }
  buildDict(edges: { source: string; target: string }[]) {
    edges.forEach((x) => {
      if (this.dict[x.source] && !this.dict[x.target]) {
        this.dict[x.target] = this.dict[x.source];
      } else if (!this.dict[x.source] && this.dict[x.target]) {
        this.dict[x.source] = this.dict[x.target];
      } else if (!this.dict[x.source] && !this.dict[x.target]) {
        this.dict[x.source] = this.dict[x.target] =
          this.keyGenerator.generateKey();
      }
    });
    return this.dict;
  }
  buildNodesAndCombos(nodes: any[], edges: any[]) {
    this.buildDict(edges);
    const nextNodes = nodes.map((x) => ({ ...x, combo: this.dict[x.id] }));

    const combos = Object.values(this.dict).reduce<any[]>((pre, cur) => {
      if (pre.every((x) => x.id !== cur)) {
        pre.push({
          id: cur,
          data: {
            label: `Combo ${cur}`,
          },
        });
      }
      return pre;
    }, []);

    return { nodes: nextNodes, combos };
  }
}

export const isDataExist = (data: any) => {
  return (
    data?.data && typeof data?.data !== 'boolean' && !isEmpty(data?.data?.graph)
  );
};

const findCombo = (communities: string[]) => {
  const combo = Array.isArray(communities) ? communities[0] : undefined;
  return combo;
};

export const buildNodesAndCombos = (nodes: any[]) => {
  const combos: any[] = [];
  nodes.forEach((x) => {
    const combo = findCombo(x?.communities);
    if (combo && combos.every((y) => y.data.label !== combo)) {
      combos.push({
        isCombo: true,
        id: uuid(),
        data: {
          label: combo,
        },
      });
    }
  });

  const nextNodes = nodes.map((x) => {
    return {
      ...x,
      combo: combos.find((y) => y.data.label === findCombo(x?.communities))?.id,
    };
  });

  return { nodes: nextNodes, combos };
};
