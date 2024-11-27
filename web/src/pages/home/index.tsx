import { Applications } from './applications';
import { Banner } from './banner';
import { Datasets } from './datasets';
import { HomeHeader } from './header';

const Home = () => {
  return (
    <div className="text-white mx-8">
      <HomeHeader></HomeHeader>
      <section>
        <Banner></Banner>
        <Datasets></Datasets>
        <Applications></Applications>
      </section>
    </div>
  );
};

export default Home;
