import { Applications } from './applications';
import { NextBanner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  return (
    <div className="mx-8">
      <section>
        <NextBanner></NextBanner>
        <section className="h-[calc(100dvh-260px)] overflow-auto scrollbar-thin">
          <Datasets></Datasets>
          <Applications></Applications>
        </section>
      </section>
    </div>
  );
};

export default Home;
