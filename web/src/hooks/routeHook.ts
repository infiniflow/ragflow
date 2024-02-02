import { useLocation } from 'umi';

export enum SegmentIndex {
  Second = '2',
  Third = '3',
}

export const useSegmentedPathName = (index: SegmentIndex) => {
  const { pathname } = useLocation();

  const pathArray = pathname.split('/');
  return pathArray[index] || '';
};

export const useSecondPathName = () => {
  return useSegmentedPathName(SegmentIndex.Second);
};

export const useThirdPathName = () => {
  return useSegmentedPathName(SegmentIndex.Third);
};
