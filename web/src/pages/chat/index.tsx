import { useSelector } from 'umi';

const Chat = () => {
  const { name } = useSelector((state: any) => state.chatModel);
  return <div>chat:{name} </div>;
};

export default Chat;
