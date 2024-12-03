import { Applications } from './applications';
import { Banner } from './banner';
import { Datasets } from './datasets';

const Home = () => {
  return (
    <div className="mx-8">
      <section>
        <Banner></Banner>
        <Datasets></Datasets>
        <Applications></Applications>
      </section>
    </div>
  );
};

export default Home;
