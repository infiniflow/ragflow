import { Banner } from './banner';
import { HomeHeader } from './header';
import NextBanner from './next-banner';

const Home = () => {
  return (
    <div>
      <HomeHeader></HomeHeader>
      <section>
        <Banner></Banner>
        <NextBanner></NextBanner>
      </section>
    </div>
  );
};

export default Home;
