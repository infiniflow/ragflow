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
  const isBackfaceVisibilitySupported = () => {
    return (
      CSS.supports('backface-visibility', 'hidden') ||
      CSS.supports('-webkit-backface-visibility', 'hidden') ||
      CSS.supports('-moz-backface-visibility', 'hidden') ||
      CSS.supports('-ms-backface-visibility', 'hidden')
    );
  };
  return (
    <>
      {isBackfaceVisibilitySupported() && (
        <div className="relative w-full h-full perspective-1000">
          <div
            className={`relative w-full h-full transition-transform transform-style-3d ${isFlipped ? 'rotate-y-180' : ''}`}
          >
            {/* Front Face */}
            <div className="absolute inset-0 flex items-center justify-center backface-hidden rotate-y-0">
              {children}
            </div>

            {/* Back Face */}
            <div className="absolute inset-0 flex items-center justify-center backface-hidden rotate-y-180">
              {children}
            </div>
          </div>
        </div>
      )}
      {!isBackfaceVisibilitySupported() && <>{children}</>}
    </>
  );
};

export default FlipCard3D;
