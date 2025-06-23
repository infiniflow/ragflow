import { ReactNode } from 'react';

type TopTitleProps = {
  title: ReactNode;
  description: ReactNode;
};

export function TopTitle({ title, description }: TopTitleProps) {
  return (
    <div className="pb-5">
      <div className="text-2xl font-semibold">{title}</div>
      <p className="text-text-sub-title pt-2">{description}</p>
    </div>
  );
}
