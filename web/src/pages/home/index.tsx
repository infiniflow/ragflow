import { PageContainer } from '@/layouts/components/page-container';
import { NextBanner } from './banner';
import { HomeChatEntry } from './home-chat-entry';
const Home = () => {
  return (
    <PageContainer>
      <header className="mb-8">
        <NextBanner />
      </header>

      <HomeChatEntry />
    </PageContainer>
  );
};

export default Home;
