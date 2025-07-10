import classNames from 'classnames';
import {
  GitPullRequestArrow,
  Layers,
  LayoutGrid,
  LucideProps,
  Users,
} from 'lucide-react';
import React from 'react';
import '../index.less';
interface IFeatureProps {
  apps: string;
  teamMembers: string;
  datasetStorage: string;
  apiRequests: string;
}
interface IPricingCardProps {
  title: string;
  description: string;
  price: string;
  feature: IFeatureProps;
  buttonLabel: string;
  isUse?: boolean;
  icon: (
    props?: JSX.IntrinsicAttributes &
      Omit<LucideProps, 'ref'> &
      React.RefAttributes<SVGSVGElement>,
  ) => JSX.Element;
}

interface ISuffixProps {
  id: number;
  icon: JSX.Element;
  text: 'apps' | 'team members' | 'GB dataset storage' | 'min API requests';
  key: keyof IFeatureProps;
}
const PricingCard: React.FC<IPricingCardProps> = ({
  title,
  description,
  price,
  feature,
  buttonLabel,
  isUse = false,
  icon,
}) => {
  const suffix = [
    {
      id: 1,
      icon: <LayoutGrid size={12} className="text-gray-500 font-normal mr-2" />,
      text: 'Apps',
      key: 'apps',
    },
    {
      id: 2,
      icon: <Users size={12} className="text-gray-500 font-normal mr-2" />,
      text: 'team members',
      key: 'teamMembers',
    },
    {
      id: 3,
      icon: <Layers size={12} className="text-gray-500 font-normal mr-2" />,
      text: 'GB dataset storage',
      key: 'datasetStorage',
    },
    {
      id: 4,
      icon: (
        <GitPullRequestArrow
          size={12}
          className="text-gray-500 font-normal mr-2"
        />
      ),
      text: 'min API requests',
      key: 'apiRequests',
    },
  ] as ISuffixProps[];
  return (
    <div
      className={`price-card rounded-lg shadow-lg p-6 text-center transition-transform hover:scale-105 bg-black`}
    >
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold mb-4 text-left">{title}</h2>
        <div className="hover:text-sky-400">{icon()}</div>
      </div>
      <p className="mb-6 text-left h-16">{description}</p>
      <ul className="mb-6">
        {suffix.map((item) => (
          <li key={item.id} className="mb-2 text-left">
            <div className="flex items-center">
              {item.icon}
              <span className="italic text-base font-semibold">
                {feature[item.key]}
              </span>
              <span className="ml-2 text-xm text-gray-500 font-normal">
                {item.text}
              </span>
            </div>
          </li>
        ))}
      </ul>
      <h3 className="text-3xl font-bold mb-6 text-left">
        <span className="text-sm mr-1">$</span>
        {price}
        <span className="text-sm text-gray-500 font-normal ml-1">/month</span>
      </h3>
      {/* bg-gradient-to-r from-gray-900 to-gray-950 */}
      <button
        type="button"
        className={classNames(
          'w-full py-2 rounded-full font-bold  text-black  hover:bg-sky-500',
          { 'bg-gray-900': isUse, 'text-white': isUse, 'bg-white': !isUse },
        )}
      >
        {buttonLabel}
      </button>
    </div>
  );
};

export default PricingCard;
