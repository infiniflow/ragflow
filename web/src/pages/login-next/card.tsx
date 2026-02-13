import React, { useEffect, useState } from 'react';
import './index.less';

type AuthFace = 'front' | 'back';
type AuthFaceContextValue = {
  active: boolean;
  face: AuthFace;
};

export const AuthFaceContext = React.createContext<AuthFaceContextValue | null>(
  null,
);

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
  const frontInertProps = (isFlipped ? { inert: '' } : {}) as React.HTMLAttributes<HTMLDivElement>;
  const backInertProps = (!isFlipped ? { inert: '' } : {}) as React.HTMLAttributes<HTMLDivElement>;
  return (
    <>
      {isBackfaceVisibilitySupported() && (
        <div className="relative w-full h-full perspective-1000">
          <div
            className={`relative w-full h-full transition-transform transform-style-3d ${isFlipped ? 'rotate-y-180' : ''}`}
          >
            {/* Front Face */}
            <div
              className={`absolute inset-0 flex items-center justify-center backface-hidden rotate-y-0 ${
                isFlipped ? 'pointer-events-none' : 'pointer-events-auto'
              }`}
              data-testid={!isFlipped ? 'auth-card-active' : undefined}
              data-face="front"
              aria-hidden={isFlipped}
              {...frontInertProps}
            >
              <AuthFaceContext.Provider value={{ active: !isFlipped, face: 'front' }}>
                {children}
              </AuthFaceContext.Provider>
            </div>

            {/* Back Face */}
            <div
              className={`absolute inset-0 flex items-center justify-center backface-hidden rotate-y-180 ${
                isFlipped ? 'pointer-events-auto' : 'pointer-events-none'
              }`}
              data-testid={isFlipped ? 'auth-card-active' : undefined}
              data-face="back"
              aria-hidden={!isFlipped}
              {...backInertProps}
            >
              <AuthFaceContext.Provider value={{ active: isFlipped, face: 'back' }}>
                {children}
              </AuthFaceContext.Provider>
            </div>
          </div>
        </div>
      )}
      {!isBackfaceVisibilitySupported() && (
        <AuthFaceContext.Provider value={{ active: true, face: 'front' }}>
          {children}
        </AuthFaceContext.Provider>
      )}
    </>
  );
};

export default FlipCard3D;
