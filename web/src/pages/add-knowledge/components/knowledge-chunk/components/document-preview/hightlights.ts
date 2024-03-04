export const testHighlights = [
  {
    content: {
      text: '实验证明，由氧氯化锆锂和高镍三元正极组成的全固态锂电池展示了极为优异的性能：在12 分钟快速充电的条件下，该电池仍然成功地在室温稳定循环2000 圈以上。',
    },
    position: {
      boundingRect: {
        x1: 219.7,
        // x1: 419.7,
        y1: 204.3,
        // y1: 304.3,
        x2: 547.0,
        // x2: 747.0,
        y2: 264.0,
        // y2: 364.0,
      },
      rects: [
        // {
        //   x1: 219.7,
        //   // x1: 419.7,
        //   y1: 204.3,
        //   // y1: 304.3,
        //   x2: 547.0,
        //   // x2: 747.0,
        //   y2: 264.0,
        //   // y2: 364.0,
        //   width: 849,
        //   height: 1200,
        // },
      ],
      pageNumber: 9,
    },
    comment: {
      text: 'Flow or TypeScript?',
      emoji: '🔥',
    },
    id: 'jsdlihdkghergjl',
  },
  {
    content: {
      text: '图2：乘联会预计6 月新能源乘用车厂商批发销量74 万辆，环比增长10%，同比增长30%。',
    },
    position: {
      boundingRect: {
        x1: 219.0,
        x2: 546.0,
        y1: 616.0,
        y2: 674.7,
      },
      rects: [],
      pageNumber: 6,
    },
    comment: {
      text: 'Flow or TypeScript?',
      emoji: '🔥',
    },
    id: 'bfdbtymkhjildbfghserrgrt',
  },
  {
    content: {
      text: '图2：乘联会预计6 月新能源乘用车厂商批发销量74 万辆，环比增长10%，同比增长30%。',
    },
    position: {
      boundingRect: {
        x1: 73.7,
        x2: 391.7,
        y1: 570.3,
        y2: 676.3,
      },
      rects: [],
      pageNumber: 1,
    },
    comment: {
      text: '',
      emoji: '',
    },
    id: 'fgnhxdvsesgmghyu',
  },
].map((x) => {
  const boundingRect = x.position.boundingRect;
  const ret: any = {
    width: 849,
    height: 1200,
  };
  Object.entries(boundingRect).forEach(([key, value]) => {
    ret[key] = value / 0.7;
  });
  return { ...x, position: { ...x.position, boundingRect: ret, rects: [ret] } };
});
