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
