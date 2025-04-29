import { Applications } from './applications';
import { NextBanner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  return (
    <div className="mx-8">
      <section>
        <NextBanner></NextBanner>
        <Datasets></Datasets>
        <Applications></Applications>
      </section>
    </div>
  );
};

export default Home;
