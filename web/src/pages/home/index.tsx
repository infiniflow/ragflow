import { PageContainer } from '@/layouts/page-container';
import { Applications } from './applications';
import { NextBanner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  return (
    <PageContainer>
      <header className="mb-8">
        <NextBanner />
      </header>

      <Datasets />
      <Applications />
    </PageContainer>
  );
};

export default Home;
