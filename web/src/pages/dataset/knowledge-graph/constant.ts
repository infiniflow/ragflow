const nodes = [
  {
    type: '"ORGANIZATION"',
    description:
      '"厦门象屿是一家公司，其营业收入和市场占有率在2018年至2022年间有所变化。"',
    source_id: '0',
    id: '"厦门象屿"',
  },
  {
    type: '"EVENT"',
    description:
      '"2018年是一个时间点，标志着厦门象屿营业收入和市场占有率的记录开始。"',
    source_id: '0',
    entity_type: '"EVENT"',
    id: '"2018"',
  },
  {
    type: '"EVENT"',
    description:
      '"2019年是一个时间点，厦门象屿的营业收入和市场占有率在此期间有所变化。"',
    source_id: '0',
    entity_type: '"EVENT"',
    id: '"2019"',
  },
  {
    type: '"EVENT"',
    description:
      '"2020年是一个时间点，厦门象屿的营业收入和市场占有率在此期间有所变化。"',
    source_id: '0',
    entity_type: '"EVENT"',
    id: '"2020"',
  },
  {
    type: '"EVENT"',
    description:
      '"2021年是一个时间点，厦门象屿的营业收入和市场占有率在此期间有所变化。"',
    source_id: '0',
    entity_type: '"EVENT"',
    id: '"2021"',
  },
  {
    type: '"EVENT"',
    description:
      '"2022年是一个时间点，厦门象屿的营业收入和市场占有率在此期间有所变化。"',
    source_id: '0',
    entity_type: '"EVENT"',
    id: '"2022"',
  },
  {
    type: '"ORGANIZATION"',
    description:
      '"厦门象屿股份有限公司是一家公司，中文简称为厦门象屿，外文名称为Xiamen Xiangyu Co.,Ltd.，外文名称缩写为Xiangyu，法定代表人为邓启东。"',
    source_id: '1',
    id: '"厦门象屿股份有限公司"',
  },
  {
    type: '"PERSON"',
    description: '"邓启东是厦门象屿股份有限公司的法定代表人。"',
    source_id: '1',
    entity_type: '"PERSON"',
    id: '"邓启东"',
  },
  {
    type: '"GEO"',
    description: '"厦门是一个地理位置，与厦门象屿股份有限公司相关。"',
    source_id: '1',
    entity_type: '"GEO"',
    id: '"厦门"',
  },
  {
    type: '"PERSON"',
    description:
      '"廖杰 is the Board Secretary, responsible for handling board-related matters and communications."',
    source_id: '2',
    id: '"廖杰"',
  },
  {
    type: '"PERSON"',
    description:
      '"史经洋 is the Securities Affairs Representative, responsible for handling securities-related matters and communications."',
    source_id: '2',
    entity_type: '"PERSON"',
    id: '"史经洋"',
  },
  {
    type: '"GEO"',
    description:
      '"A geographic location in Xiamen, specifically in the Free Trade Zone, where the company\'s office is situated."',
    source_id: '2',
    entity_type: '"GEO"',
    id: '"厦门市湖里区自由贸易试验区厦门片区"',
  },
  {
    type: '"GEO"',
    description:
      '"The building where the company\'s office is located, situated at Xiangyu Road, Xiamen."',
    source_id: '2',
    entity_type: '"GEO"',
    id: '"象屿集团大厦"',
  },
  {
    type: '"EVENT"',
    description:
      '"Refers to the year 2021, used for comparing financial metrics with the year 2022."',
    source_id: '3',
    id: '"2021年"',
  },
  {
    type: '"EVENT"',
    description:
      '"Refers to the year 2022, used for presenting current financial metrics and comparing them with the year 2021."',
    source_id: '3',
    entity_type: '"EVENT"',
    id: '"2022年"',
  },
  {
    type: '"EVENT"',
    description:
      '"Indicates the focus on key financial metrics in the table, such as weighted averages and percentages."',
    source_id: '3',
    entity_type: '"EVENT"',
    id: '"主要财务指标"',
  },
].map(({ type, ...x }) => ({ ...x }));

const edges = [
  {
    weight: 2.0,
    description: '"厦门象屿在2018年的营业收入和市场占有率被记录。"',
    source_id: '0',
    source: '"厦门象屿"',
    target: '"2018"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿在2019年的营业收入和市场占有率有所变化。"',
    source_id: '0',
    source: '"厦门象屿"',
    target: '"2019"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿在2020年的营业收入和市场占有率有所变化。"',
    source_id: '0',
    source: '"厦门象屿"',
    target: '"2020"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿在2021年的营业收入和市场占有率有所变化。"',
    source_id: '0',
    source: '"厦门象屿"',
    target: '"2021"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿在2022年的营业收入和市场占有率有所变化。"',
    source_id: '0',
    source: '"厦门象屿"',
    target: '"2022"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿股份有限公司的法定代表人是邓启东。"',
    source_id: '1',
    source: '"厦门象屿股份有限公司"',
    target: '"邓启东"',
  },
  {
    weight: 2.0,
    description: '"厦门象屿股份有限公司位于厦门。"',
    source_id: '1',
    source: '"厦门象屿股份有限公司"',
    target: '"厦门"',
  },
  {
    weight: 2.0,
    description:
      '"廖杰\'s office is located in the Xiangyu Group Building, indicating his workplace."',
    source_id: '2',
    source: '"廖杰"',
    target: '"象屿集团大厦"',
  },
  {
    weight: 2.0,
    description:
      '"廖杰 works in the Xiamen Free Trade Zone, a specific area within Xiamen."',
    source_id: '2',
    source: '"廖杰"',
    target: '"厦门市湖里区自由贸易试验区厦门片区"',
  },
  {
    weight: 2.0,
    description:
      '"史经洋\'s office is also located in the Xiangyu Group Building, indicating his workplace."',
    source_id: '2',
    source: '"史经洋"',
    target: '"象屿集团大厦"',
  },
  {
    weight: 2.0,
    description:
      '"史经洋 works in the Xiamen Free Trade Zone, a specific area within Xiamen."',
    source_id: '2',
    source: '"史经洋"',
    target: '"厦门市湖里区自由贸易试验区厦门片区"',
  },
  {
    weight: 2.0,
    description:
      '"The years 2021 and 2022 are related as they are used for comparing financial metrics, showing changes and adjustments over time."',
    source_id: '3',
    source: '"2021年"',
    target: '"2022年"',
  },
  {
    weight: 2.0,
    description:
      '"The \'主要财务指标\' is related to the year 2021 as it provides the basis for financial comparisons and adjustments."',
    source_id: '3',
    source: '"2021年"',
    target: '"主要财务指标"',
  },
  {
    weight: 2.0,
    description:
      '"The \'主要财务指标\' is related to the year 2022 as it presents the current financial metrics and their changes compared to 2021."',
    source_id: '3',
    source: '"2022年"',
    target: '"主要财务指标"',
  },
];

export const graphData = {
  directed: false,
  multigraph: false,
  graph: {},
  nodes,
  edges,
  combos: [],
};
