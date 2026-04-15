import { Button } from '@/components/ui/button';
import { Routes } from '@/routes';
import { useLocation, useNavigate } from 'react-router';

const NoFoundPage = () => {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh]">
      <h1 className="text-6xl font-bold text-transparent bg-clip-text mb-4 bg-gradient-to-l from-[#9B348E] to-[#ee0000]">
        404
      </h1>
      <div className="text-lg text-text-secondary mb-8">
        Seite nicht gefunden. Bitte überprüfen Sie die URL oder kehren Sie zur
        Startseite zurück.
      </div>
      <Button
        onClick={() => {
          navigate(
            location.pathname.startsWith(Routes.Admin) ? Routes.Admin : '/',
          );
        }}
      >
        Zurück zur Startseite
      </Button>
    </div>
  );
};

export default NoFoundPage;
