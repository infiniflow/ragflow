import { useSize } from 'ahooks';

export function useCalculateSheetRight() {
  const size = useSize(document.querySelector('body'));
  const bodyWidth = size?.width ?? 0;

  return bodyWidth > 1800 ? 'right-[620px]' : `right-1/3`;
}
