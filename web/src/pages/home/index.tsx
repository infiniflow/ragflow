import { PageContainer, PageContent } from '@/layouts/components/page-container';
import { Applications } from './applications';
import { NextBanner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  return (
    <PageContainer>
      <PageContent>
        <article className="pb-16">
          <header>
            <NextBanner />
          </header>

          <Datasets />
          <Applications />
        </article>
      </PageContent>
    </PageContainer>
  );
};

export default Home;
