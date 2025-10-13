import React, { useEffect, useState } from 'react';
import './index.less';

type IProps = {
  children: React.ReactNode;
  isLoginPage: boolean;
};
const FlipCard3D = (props: IProps) => {
  const { children, isLoginPage } = props;
  const [isFlipped, setIsFlipped] = useState(false);
  useEffect(() => {
    console.log('title', isLoginPage);
    if (isLoginPage) {
      setIsFlipped(false);
    } else {
      setIsFlipped(true);
    }
  }, [isLoginPage]);

  return (
    <div className="relative w-full h-full perspective-1000">
      <div
        className={`relative w-full h-full transition-transform transform-style-3d ${isFlipped ? 'rotate-y-180' : ''}`}
      >
        {/* Front Face */}
        <div className="absolute inset-0 flex items-center justify-center bg-blue-500 text-white rounded-xl backface-hidden">
          {children}
        </div>

        {/* Back Face */}
        <div className="absolute inset-0 flex items-center justify-center bg-green-500 text-white rounded-xl backface-hidden rotate-y-180">
          {children}
        </div>
      </div>
    </div>
  );
};

export default FlipCard3D;
