import { Button } from '@/components/ui/button';
import { Routes } from '@/routes';
import { useLocation } from 'react-router';

const NoFoundPage = () => {
  const location = useLocation();

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh]">
      <div className="text-6xl font-bold text-text-secondary mb-4">404</div>
      <div className="text-lg text-text-secondary mb-8">
        Page not found, please enter a correct address.
      </div>
      <Button
        onClick={() => {
          window.open(
            location.pathname.startsWith(Routes.Admin) ? Routes.Admin : '/',
            '_self',
          );
        }}
      >
        Business
      </Button>
    </div>
  );
};

export default NoFoundPage;
