import { CardWithForm } from './card';
import { HomeHeader } from './header';

const Home = () => {
  return (
    <div>
      <HomeHeader></HomeHeader>
      <section>
        <CardWithForm></CardWithForm>
      </section>
    </div>
  );
};

export default Home;
