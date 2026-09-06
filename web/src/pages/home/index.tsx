import { getFirstVisibleNavigationPath } from '@/constants/navigation';
import { useSystemConfig } from '@/hooks/use-system-request';
import { PageContainer } from '@/layouts/components/page-container';
import { Navigate } from 'react-router';
import { Applications } from './applications';
import { NextBanner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  const { config, loading } = useSystemConfig();

  if (loading) {
    return null;
  }

  if (config && !config.visibleSections.includes('home')) {
    const firstVisiblePath = getFirstVisibleNavigationPath(
      config.visibleSections,
    );
    if (firstVisiblePath) {
      return <Navigate to={firstVisiblePath} replace />;
    }
  }

  return (
    <PageContainer>
      <article>
        <header className="mb-8">
          <NextBanner />
        </header>

        <Datasets />
        <Applications />
      </article>
    </PageContainer>
  );
};

export default Home;
