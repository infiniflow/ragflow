import {
  UseDynamicSVGImportOptions,
  useDynamicSVGImport,
} from '@/hooks/commonHooks';

interface IconProps extends React.SVGProps<SVGSVGElement> {
  name: string;
  onCompleted?: UseDynamicSVGImportOptions['onCompleted'];
  onError?: UseDynamicSVGImportOptions['onError'];
}

const SvgIcon: React.FC<IconProps> = ({
  name,
  onCompleted,
  onError,
  ...rest
}): React.ReactNode | null => {
  const { error, loading, SvgIcon } = useDynamicSVGImport(name, {
    onCompleted,
    onError,
  });
  if (error) {
    return error.message;
  }
  if (loading) {
    return 'Loading...';
  }
  if (SvgIcon) {
    return <SvgIcon {...rest} />;
  }
  return null;
};

export default SvgIcon;
