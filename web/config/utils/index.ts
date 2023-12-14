/**
 * Whether to generate package preview
 * 是否生成打包报告
 */
export default {};

export function isReportMode(): boolean {
  return process.env.REPORT === 'true';
}
